# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build the coordinator binary
go build ./cmd/crossfuzz/

# Run all Go tests
go test ./...

# Run tests for a specific package
go test ./pkg/coverage/
go test ./harness/gofuzz/

# Build targets defined in a config
./crossfuzz build examples/base64/crossfuzz.toml

# Run a fuzzing campaign
./crossfuzz run examples/base64/crossfuzz.toml

# Build C harness target (must use clang with sanitizer coverage)
clang -fsanitize-coverage=trace-pc-guard -o target target.c harness/c/crossfuzz.c

# Build Go fuzz target (-coverpkg=all is required when target delegates into stdlib/third-party)
go build -cover -covermode=atomic -coverpkg=all -o target ./cmd/target

# Build Java harness
cd harness/java && gradle build   # produces build/libs/crossfuzz.jar
```

## Architecture

cross_fuzz is a coverage-guided **differential fuzzer**: it sends the same generated input to multiple implementations of the same function (across C, Go, Java, JS), collects coverage from all targets, merges it into a shared bitmap, and flags any divergence in outputs.

### Data flow per iteration

1. Coordinator picks/mutates a corpus entry
2. Writes input to each target's shared memory region (`pkg/coverage/shmem.go`)
3. Sends `{"type":"fuzz"}` over a pipe to each target process (fd 3)
4. Each target executes, updates its coverage bitmap in the shared memory, writes output
5. Each target responds `{"type":"fuzz_result"}` over fd 4
6. Coordinator merges all coverage bitmaps — if new edges appear, input is added to corpus
7. Comparator checks whether all outputs agree; disagreements are saved to `findings/`

### Key packages

| Package | Role |
|---------|------|
| `pkg/engine/` | Main fuzzing loop (`coordinator.go`), mutation strategies (`mutator.go`, `mutator_bytes.go`), corpus management (`corpus.go`) |
| `pkg/runner/` | Lifecycle of target processes: start, pipe setup, shared memory handoff, crash detection, timeout |
| `pkg/coverage/` | 64 KB AFL-style bitmap ops (`bitmap.go`), shared memory creation/mapping (`shmem.go`) |
| `pkg/compare/` | `Comparator` interface + built-in implementations (`ByteEqual`, `JSONStructural`, etc.) |
| `pkg/protocol/` | Wire types and length-prefixed JSON codec used on the pipes |
| `pkg/config/` | TOML config parsing |

### Harnesses (`harness/`)

Each language harness handles the pipe protocol, shared memory mapping, and coverage plumbing. Users only write the target function.

- **C** (`harness/c/`): implements SanitizerCoverage callbacks; target must be compiled with `clang -fsanitize-coverage=trace-pc-guard`
- **Go** (`harness/gofuzz/`): uses `runtime/coverage` APIs; binary must be built with `-cover -covermode=atomic`. The `covCollector` runs a warmup phase to mask flaky (non-deterministic) bitmap slots from GC/allocator instrumentation
- **Java** (`harness/java/`): Gradle project; custom `CoverageAgent`/`CoverageTransformer` via Java instrumentation API; produces `crossfuzz.jar`
- **JavaScript**: Istanbul-based AST instrumentation (not yet in repo, documented in ARCHITECTURE.md)

### IPC layout

Each target gets one shared memory region (~2 MB + 64 KB) with this layout:

```
0x000000  8 B   exec_count
0x000008  4 B   input_len
0x00000C  4 B   output_len
0x000010  4 B   status (0=ok, 1=error, 2=crash)
0x000040  1 MB  input region    (coordinator writes)
0x100040  1 MB  output region   (target writes)
0x200040  64 KB coverage bitmap (target writes)
```

The coordinator passes the shared memory path via `CROSSFUZZ_SHM` env var. Pipes use inherited fd 3 (coordinator→worker) and fd 4 (worker→coordinator).

### Coverage bitmap

A 64 KB byte array of saturating counters following the AFL model. Counters are bucketized to powers of two `{1,2,4,8,16,32,64,128}`. An input is "interesting" if it produces a non-zero counter in any slot where the global bitmap is zero, or moves an existing counter to a higher bucket. The Go harness hashes `(pkgID, funcID, counterIdx)` tuples using three multiplicative hash constants into 16-bit bitmap indices.

### Configuration

TOML files with four sections: `[campaign]`, `[corpus]`, `[comparator]`, and one or more `[[target]]` entries. The `build_cmd` field in each target is run by `crossfuzz build`; it is separate from the `binary`/`args` used at runtime.
