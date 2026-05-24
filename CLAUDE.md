# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build everyting coordinator + all buildable harness
make

# Build the coordinator binary
make bin/crossfuzz

# Run all tests (currently only Go tests)
make test

# Build Java & JS harnesses
make harness

# Build targets defined in a config
./bin/crossfuzz build crossfuzz.toml

# Run a fuzzing campaign
./bin/crossfuzz run crossfuzz.toml
```

## Architecture

cross_fuzz is a coverage-guided **differential fuzzer**: it sends the same generated input to multiple implementations of the same function (across C, C++, Go, Java, JS/TS), collects coverage from all targets, merges it into a shared bitmap, and flags any divergence in outputs.

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

- **C** (`harness/c/`): implements SanitizerCoverage callbacks; target must be compiled with `clang -fsanitize-coverage=trace-pc-guard -I ../../harness/c crossfuzz.c`
- **C++** (`harness/cpp/`): thin wrapper over the C harness; must compile both `crossfuzz.c` (from `harness/c/`) and `crossfuzz.cpp` together with `-fsanitize-coverage=trace-pc-guard`
- **Go** (`harness/gofuzz/`): uses `runtime/coverage` APIs; binary must be built with `-cover -covermode=atomic`. Use `-coverpkg` with an explicit package list (not just `./...`) to include stdlib/third-party packages the target delegates into — see examples for the `go list -deps` filter pattern. The `covCollector` runs a warmup phase to mask flaky bitmap slots from GC/allocator noise.
- **Java** (`harness/java/`): Gradle project; custom `CoverageAgent`/`CoverageTransformer` via Java instrumentation API; produces `crossfuzz.jar`. Pass `-javaagent:crossfuzz.jar` at runtime.
- **JavaScript/TypeScript** (`harness/js/`): runs under Bun. Published as the `@crossfuzz/crossfuzz` npm package. Istanbul-based AST instrumentation is applied via `bun --preload @crossfuzz/crossfuzz/instrument.ts`. Targets import `run`/`fuzz` from `@crossfuzz/crossfuzz`.

All harnesses support an `instrument` setting (default `true`) that can be set to `false` when the harness is a thin HTTP trigger and coverage should come entirely from an instrumented server process. Field names: Go `Settings{Instrument: false}`, C `{.instrument = 0}`, C++ `{.instrument = false}`, JS `{instrument: false}`, Python/Rust/Java `instrument = false`.

### Server targets

Targets can be `type = "server"` in the TOML config for long-running server processes (e.g. HTTP servers). In this mode the coordinator does not use the pipe protocol with the target; instead a separate harness process (usually Go or JS) acts as the client, sends requests, and reports results. The Go harness provides `InitServer()`, `ClearCoverage()`, and `CollectCoverage()` APIs for server-side coverage collection in this setup.

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

Target `language` values: `"c"`, `"cpp"`, `"go"`, `"java"`, `"js"`, `"python"`, `"rust"`. Target `type` values: `"harness"` (default, uses pipe protocol) or `"server"` (long-running process, no pipe).

Comparator `type` values: `"byte_equal"`, `"json_structural"`, `"numeric"`, `"custom"` (requires `script`), `"none"`.
