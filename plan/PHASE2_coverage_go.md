# Phase 2 (Go): Wire Coverage Into The Go Harness

## Context

cross_fuzz is a coverage-guided differential fuzzer. Phase 1 established the
shared-memory + pipe protocol, byte-equality comparison, and a working C+Go
differential loop on `examples/base64`. Phase 2's goal (per
`IMPLEMENTATION_PLAN.md`) is to enable **coverage-guided mutation** by having
each per-language harness populate the 64 KB coverage bitmap that lives at
offset `0x200040` inside its shared-memory region.

A thorough inventory of the repo shows Phase 2 is **already ~95% wired on the
orchestrator side**:

- `pkg/coverage/bitmap.go` — `HasNewBits`, `Merge`, `CountBits`, `Reset`,
  `Bucketize` are all implemented (`pkg/coverage/bitmap.go:6-53`).
- `pkg/coverage/shmem.go` — 64 KB bitmap region reserved at `CoverageOffset`,
  `SharedMem.Coverage()` returns a direct slice into it
  (`pkg/coverage/shmem.go:149-156`).
- `pkg/runner/process.go:109-134` already resets the bitmap, executes, and
  copies it back to the caller after every iteration.
- `pkg/engine/coordinator.go:86-115` already bucketizes combined coverage,
  checks for new bits, merges into the global bitmap, and grows the corpus
  on new coverage.
- `harness/c/crossfuzz.c` already implements the SanitizerCoverage callbacks
  and writes to the shmem bitmap (confirmed by the Phase 1 exploration).
- Mutator (`pkg/engine/mutator.go`, `mutator_bytes.go`), corpus
  (`pkg/engine/corpus.go`), and live stats (`pkg/engine/stats.go`) are
  complete.

What is **missing** is that the Go harness
(`harness/gofuzz/harness.go:52-85`) never touches the coverage region, and
`examples/base64/go_target/` is built without any coverage instrumentation
(`examples/base64/crossfuzz.toml:22`). As a result `combinedCov` from Go
targets is always zero, `HasNewBits` never fires for Go, the corpus never
grows from Go execution, and mutation is effectively blind.

This plan focuses exclusively on the Go side of Phase 2. (C already works.)

---

## Design Decision: How To Read Go Coverage Per Iteration

Go 1.20+ emits coverage for any program built with `go build -cover`. For
in-process per-iteration reads the public API is
[`runtime/coverage`](https://pkg.go.dev/runtime/coverage):

| Function                         | Use                                                       |
|----------------------------------|-----------------------------------------------------------|
| `WriteCounters(io.Writer) error` | Serialize current counter snapshot into the covcounters binary format |
| `ClearCounters() error`          | Zero all counters in-place                                |
| `WriteMeta(io.Writer) error`     | Dump package/function metadata (needed for decoding if you want names) |

Both counter functions **require `-covermode=atomic`** and return an error
otherwise. They are goroutine-safe. They do *not* require `GOCOVERDIR` to be
set — that env var only controls on-exit emission, which we do not use.

### Alternatives considered

1. **`//go:linkname` to `internal/fuzz` `_counters`/`_ecounters`** — Go's
   native fuzzer uses these symbols for 8-bit edge counters, but the linker
   only emits them for `go test -fuzz` binaries, *not* for `go build -cover`.
   Cannot be used for a freestanding `gofuzz.Run()` harness without
   restructuring the target as a `Fuzz*` test.
2. **`//go:linkname` to `runtime/coverage.getCovCounterList` (or
   `internal/coverage/rtcov.Meta.List`)** — would give direct access to the
   `[]CovCounterBlob` that `WriteCounters` walks internally, avoiding parsing.
   Faster (~10-30 LOC) but pulls in an unexported symbol that Go makes no
   stability promise about. We keep this as a future optimization if
   iteration throughput becomes a bottleneck.
3. **`go tool covdata textfmt` subprocess** — per-iteration fork/exec is a
   non-starter for fuzzing throughput.
4. **Build target as `go test -c -fuzz=Fuzz...`** — inverts our control flow
   (the test binary owns `main`) and would require reworking `gofuzz.Run`
   and `ProcessConfig` to drive a test binary. Too invasive for Phase 2.

### Chosen approach

**Use `runtime/coverage.WriteCounters(&buf)` + implement a minimal in-tree
parser for the covcounters binary format, hash each
`(pkgID, funcID, counterIdx)` tuple into the 64 KB bitmap, store the
bucketed counter value, then `ClearCounters()`.**

Rationale: public API, stable across Go 1.20+, no unsafe, no reliance on
internal symbols. Parser is ~100 LOC and only needs to understand what
`WriteCounters` emits (subset of what `go tool covdata` handles). If
throughput is insufficient later, we can swap in the linkname approach
behind the same internal function — the harness loop and bitmap layout do
not need to change.

---

## Covcounters Binary Format (Reference)

`WriteCounters` writes exactly one "counter file" that the rest of the Go
tooling would normally persist as `covcounters.<hash>.<pid>.<nanos>`. The
format is defined in Go source at `internal/coverage/defs.go` and
`internal/coverage/decodecounter/` (authoritative reference for the
implementation).

```
+-------------------------------+  offset 0
| CounterFileHeader (32 bytes)  |
|   Magic      [4]byte          |   {0x00, 'c', 'v', 'C'}  <-- NOT "covc"
|   Version    uint32           |   currently 1
|   MetaHash   [16]byte         |   MD5 of the companion covmeta
|   CFlavor    uint8            |   0=CtrRaw (LE uint32)  1=CtrULeb (ULEB128)
|   BigEndian  uint8            |   0 on all supported platforms
|   _          [6]byte          |   padding
+-------------------------------+  offset 32
| 1..N CounterSegments          |
|   SegmentHeader (16 bytes)    |
|     FcnEntries  uint64        |
|     StrTabLen   uint32        |
|     ArgsLen     uint32        |
|   ArgsPayload   [ArgsLen]byte |   OS args / GOOS / GOARCH as stringtab
|   StrTab        [StrTabLen]byte
|   FuncPayload * FcnEntries    |
|     NumCtrs   uint32|ULEB      |
|     PkgId     uint32|ULEB      |
|     FuncId    uint32|ULEB      |
|     Counters[NumCtrs]          |   each uint32|ULEB
|   (padding to 4-byte align)    |
+-------------------------------+
| CounterFileFooter (16 bytes)  |
|   Magic         [4]byte       |   same magic as header
|   NumSegments   uint32        |
|   _             [8]byte       |   padding
+-------------------------------+
```

For a single in-process snapshot there is exactly **one segment**.
`WriteCounters` uses the ULEB flavor by default. We must support both.

Our parser only needs: flavor, endianness, `FcnEntries`, args/strtab length
(to skip), and for each function `(PkgId, FuncId, NumCtrs, Counters[])`.
We do not need function or file names, so we never touch the string table.

---

## Implementation Plan

### 1. Add a covcounters parser (new file)

**New file:** `harness/gofuzz/covcounters.go`

Responsibilities:

```go
package gofuzz

// funcCounters is one function's counter payload from a WriteCounters snapshot.
type funcCounters struct {
    pkgID    uint32
    funcID   uint32
    counters []uint32 // reused across calls; parser owns the storage
}

// covReader parses the covcounters binary stream produced by
// runtime/coverage.WriteCounters. It reuses internal buffers so that
// repeated per-iteration parses do not allocate.
type covReader struct {
    scratch []funcCounters // grown as needed, truncated each call
    ctrBuf  []uint32       // flat counter arena, indices into it from scratch[].counters
}

// parse walks data and appends every function payload into r.scratch.
// Returns the populated slice (aliased to r.scratch).
func (r *covReader) parse(data []byte) ([]funcCounters, error) { ... }
```

Helpers required:

- `readHeader(data) (flavor uint8, bigEndian bool, rest []byte, err error)`
  — verifies the 4-byte magic `{0x00,'c','v','C'}`, checks version, reads
  flavor/endian, returns `data[32:]`.
- `readSegmentHeader(data, flavor) (fcnEntries uint64, payload []byte, err error)`
  — reads 16-byte header, then skips `ArgsLen + StrTabLen` bytes, returns
  the function-payload slice.
- `readULEB128(data) (value uint64, n int, err error)` — standard base-128
  decoder, ~15 LOC.
- `readU32(data, bigEndian) (uint32, int)` — little/big endian variant.
- A single per-function loop that pulls `NumCtrs, PkgId, FuncId` then
  `NumCtrs` counter words (encoding branches on `flavor`).

Edge cases to handle:

- Empty snapshot (zero segments, zero functions) — must not error; return
  empty slice. Important for the handshake/startup check.
- Unknown `CFlavor` — return a clear error (`"unsupported covcounter
  flavor %d"`) so the harness can fail fast.
- Truncated input — bounded reads; return `io.ErrUnexpectedEOF`.

**Testing this file:** a small unit test `covcounters_test.go` that feeds a
buffer produced by `runtime/coverage.WriteCounters` of the test process
itself and asserts the parser returns a non-empty list with non-zero counter
values on at least one entry. Kept minimal; real validation happens at the
integration level (see Verification section).

### 2. Add a coverage collector to the Go harness (modify existing file)

**Modify:** `harness/gofuzz/harness.go`

Add a new unexported type owned by `Run`:

```go
type covCollector struct {
    buf      bytes.Buffer     // reused each iteration
    reader   covReader        // reused each iteration
    enabled  bool             // false if binary was not built with -cover -covermode=atomic
    bitmap   []byte           // alias of shm.Coverage()
}

// init verifies coverage instrumentation is present. A single
// WriteCounters+ClearCounters probe is the documented way to detect
// "-cover -covermode=atomic". On failure we log a clear warning and
// disable coverage for this run rather than crashing — that keeps
// existing examples runnable without instrumentation while signalling
// the user should rebuild.
func (c *covCollector) init(bitmap []byte) { ... }

// snapshot captures counters, hashes them into the bitmap using
// saturating 8-bit values, then clears counters for the next iteration.
func (c *covCollector) snapshot() error { ... }
```

Hashing strategy (inside `snapshot`, per counter):

```go
// Derive a stable 16-bit bitmap index from (pkgID, funcID, ctrIdx).
// Mirrors AFL's index-into-bitmap design; collisions are acceptable.
key := uint64(pkgID)*0x9E3779B97F4A7C15 +
       uint64(funcID)*0xBF58476D1CE4E5B9 +
       uint64(ctrIdx)*0x94D049BB133111EB
idx  := uint16(key ^ (key >> 32))       // mod 65536
val  := counterValue                    // uint32 from parser

// Saturating 8-bit bucket — the byte at idx stores the *maximum*
// counter value seen across all collisions on this iteration, clamped
// to 255. Bucketization into powers of two happens later in the
// coordinator via coverage.Bucketize().
if val > 255 { val = 255 }
if byte(val) > c.bitmap[idx] {
    c.bitmap[idx] = byte(val)
}
```

Notes on the hash:

- The multipliers are splitmix64-style constants — fast, well-distributed.
- We take the max rather than overwrite because a single bitmap byte may
  receive contributions from multiple `(pkgID, funcID, idx)` tuples that
  collided to the same slot; taking the max preserves the strongest signal.
- The coordinator (`pkg/engine/coordinator.go:109`) subsequently calls
  `coverage.Bucketize` on the combined bitmap, so our job is only to
  deliver a faithful per-slot counter value, not a pre-bucketed one.

Where to hook it in `Run` (`harness/gofuzz/harness.go:22`):

```go
// existing: map shmem, open fd 3/4, send ready
var collector covCollector
collector.init(shm.Coverage())
```

Inside the `TypeFuzz` case, after `target(input)` returns
(`harness.go:64`) and before the response is encoded (`harness.go:81`):

```go
// write output, set status as before

if collector.enabled {
    if err := collector.snapshot(); err != nil {
        // coverage is best-effort; a parse/clear failure must not
        // abort the campaign. Log once, then keep going.
        collector.enabled = false
        fmt.Fprintf(os.Stderr, "crossfuzz: coverage disabled: %v\n", err)
    }
}

// encode response as before
```

Important: we do **not** need to `ResetCoverage` ourselves — the
coordinator already zeros the bitmap in `Process.Execute()`
(`pkg/runner/process.go:112`) before sending the `fuzz` command. Our
`snapshot` starts from an already-zeroed bitmap, writes into it, and by
the time the response goes back the coordinator reads the populated
bytes.

### 3. Fix the Go target build command

**Modify:** `examples/base64/crossfuzz.toml:22`

```toml
build_cmd = "cd go_target && go build -cover -covermode=atomic -o ../go_target_bin ."
```

No `GOCOVERDIR` is set; the harness uses `WriteCounters` directly and does
not rely on on-exit emission.

**Nothing to change in** `examples/base64/go_target/main.go` — the target
function does not need to import `runtime/coverage`; the harness handles
everything.

### 4. Detect non-instrumented binaries and warn clearly

Inside `covCollector.init` we call `WriteCounters` into a throwaway buffer
and interpret errors:

```go
if err := coverage.WriteCounters(io.Discard); err != nil {
    fmt.Fprintf(os.Stderr,
        "crossfuzz: coverage disabled for this Go target: %v\n"+
        "           (rebuild with `go build -cover -covermode=atomic`)\n", err)
    c.enabled = false
    return
}
coverage.ClearCounters() // discard the probe's counters
c.enabled = true
c.bitmap = bitmap
```

This keeps the harness backward-compatible with Phase 1 binaries (they run
with zero coverage, same as today) while making the fix obvious to anyone
reading stderr.

---

## Critical Files

| File | Action | Why |
|------|--------|-----|
| `harness/gofuzz/covcounters.go` | **new** | Parser for `runtime/coverage.WriteCounters` output. |
| `harness/gofuzz/covcounters_test.go` | **new** | Smoke-test the parser against self-produced data. |
| `harness/gofuzz/harness.go` | **edit** | Add `covCollector` type, init at startup, call `snapshot` after each target execution. |
| `examples/base64/crossfuzz.toml` | **edit** | Add `-cover -covermode=atomic` to the Go target's `build_cmd`. |

No changes required in: `pkg/coverage/{bitmap.go,shmem.go}`,
`pkg/runner/process.go`, `pkg/engine/coordinator.go`,
`pkg/protocol/protocol.go`, `pkg/config/config.go`,
`examples/base64/go_target/main.go`.

---

## Reuse Notes (Existing Code We Rely On)

- `pkg/coverage/shmem.go:149` — `SharedMem.Coverage()` gives us the direct
  mmap-backed byte slice the harness writes into. No copies.
- `pkg/coverage/shmem.go:154` — `ResetCoverage` is called by the
  coordinator before each iteration; we do not duplicate it.
- `pkg/coverage/bitmap.go:43` — `Bucketize` is applied by the coordinator
  to the merged bitmap, so we only need to produce raw saturating counts.
- `pkg/runner/process.go:130-131` — `Execute` already copies the Go
  target's bitmap back out and returns it to the coordinator; no runner
  changes needed.
- `pkg/engine/coordinator.go:108-115` — `HasNewBits`/`Merge` already run
  on the combined bitmap; Go coverage will automatically expand the
  corpus once we start populating `shm.Coverage()`.

---

## Verification

### Unit

- `go test ./harness/gofuzz/...` exercises `covReader.parse` against a
  real `runtime/coverage.WriteCounters` buffer produced inside the test
  process (build the test binary with `go test -cover -covermode=atomic`).
  Assert: parse returns `>0` function entries and at least one non-zero
  counter after touching some instrumented code.

### Integration — smoke test

1. Rebuild: `cd examples/base64 && go build -cover -covermode=atomic -o go_target_bin ./go_target`
2. Rebuild C target unchanged.
3. Run for 10s: `crossfuzz run examples/base64/crossfuzz.toml` with campaign
   `timeout = "10s"`.
4. **Expected signals** (baseline = today's Phase 1 output):
   - Live stats line (`pkg/engine/stats.go`) shows **coverage bits > 0** and
     growing — today it stays at 0 for the Go-only contribution.
   - Corpus size (`corpus/cache/`) grows past the seed count — today it is
     stuck at seed count because `HasNewBits` never fires.
   - `execs/sec` is lower than pure-Phase-1 but stable (no per-iteration
     panic, no leaks). A ~2-5x slowdown vs Phase 1 Go-only is acceptable;
     if it is >10x we should investigate before merging.
5. Deliberately break the Go base64 target (e.g. swap two characters in
   the `std` alphabet) and confirm a finding still lands in `findings/`.
6. Flip the build back to `go build` (no `-cover`) and confirm the
   harness prints the "coverage disabled" warning once and keeps running
   (backward compatibility).

### Cross-checks

- `go tool covdata textfmt -i <tmpdir>` on a `WriteCounters`-produced file
  gives us a ground-truth counter dump to compare against our parser's
  output during development.
- Print `coverage.CountBits(c.globalCov)` once per second from the
  coordinator during the smoke test — this number must be monotone and
  should plateau, not oscillate.

---

## Out Of Scope For This Plan

- C/C++ coverage (already works per Phase 1 inspection; separate plan if
  it breaks).
- Java and JS harnesses (Phase 3).
- Switching to the `//go:linkname` fast path (Phase 5 perf hardening, only
  if profiling says parsing is the bottleneck).
- New mutation strategies, new comparators, minimization — all already
  scheduled for later phases.
- Changing the wire protocol — coverage is entirely shared-memory data,
  so no message types need new fields.
