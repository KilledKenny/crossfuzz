# cross_fuzz Architecture

## Overview

cross_fuzz is a **coverage-guided differential fuzzing** tool for finding behavioral discrepancies across equivalent implementations in C, C++, Go, Java, and JavaScript.

**Core idea:** Generate inputs, feed them to all implementations simultaneously, collect coverage feedback from every target, use the combined coverage to guide mutation, and flag any case where outputs diverge.

### Goals

1. **Find bugs that tests miss** -- differential fuzzing explores edge cases that hand-written tests rarely cover
2. **Coverage-guided across all languages** -- coverage from *any* target drives input generation, so a new code path in the Java implementation can lead to discovering a bug in the C implementation
3. **Minimal harness effort** -- thin per-language libraries handle IPC and coverage; the user only writes the target function
4. **Extensible comparison** -- built-in comparators for common cases, pluggable interface for custom logic

---

## High-Level Architecture

```
                          ┌──────────────────────────┐
                          │       CLI / Config        │
                          │  crossfuzz run config.toml│
                          └────────────┬─────────────┘
                                       │
                          ┌────────────▼─────────────┐
                          │       Coordinator         │
                          │                           │
                          │  ┌─────────┐ ┌────────┐  │
                          │  │ Mutator │ │ Corpus │  │
                          │  └────┬────┘ └───┬────┘  │
                          │       │          │        │
                          │  ┌────▼──────────▼────┐  │
                          │  │  Coverage Merger    │  │
                          │  └────────┬───────────┘  │
                          │           │               │
                          │  ┌────────▼───────────┐  │
                          │  │   Comparator(s)     │  │
                          │  └────────┬───────────┘  │
                          └───────────┼───────────────┘
                                      │
              ┌───────────┬───────────┼───────────┬───────────┐
              │           │           │           │           │
        ┌─────▼────┐┌─────▼────┐┌─────▼────┐┌─────▼────┐┌─────▼────┐
        │  C/C++   ││   Go     ││  Java    ││   JS     ││  ...     │
        │  Target  ││  Target  ││  Target  ││  Target  ││ (future) │
        │ Process  ││ Process  ││ Process  ││ Process  ││          │
        └──────────┘└──────────┘└──────────┘└──────────┘└──────────┘
              │           │           │           │
         [shared mem] [shared mem] [shared mem] [shared mem]
         input/output input/output input/output input/output
         + coverage   + coverage   + coverage   + coverage
```

**Flow for each fuzzing iteration:**

1. Coordinator picks an input from the corpus (or mutates one)
2. Input is written to each target's shared memory region
3. A `fuzz` command is sent to all targets in parallel over pipes
4. Each target reads the input, executes the user's function, writes the output, updates its coverage bitmap
5. Each target responds with execution metadata over the pipe
6. Coordinator reads all outputs and coverage bitmaps
7. Coverage bitmaps are merged -- if any new edges are hit, the input is added to the corpus
8. Outputs are passed to the comparator -- any discrepancy is saved as a finding

---

## Components

### 1. Coordinator (`pkg/engine/`)

The heart of the system. Manages the fuzzing campaign loop.

**Responsibilities:**
- Maintain the global corpus (seed inputs + discovered interesting inputs)
- Drive the mutation engine to produce new inputs
- Dispatch inputs to all target processes in parallel
- Merge coverage bitmaps from all targets
- Decide which inputs are "interesting" (expand combined coverage)
- Run comparators on outputs from each iteration
- Save findings (discrepancies) to disk
- Report live statistics (execs/sec, corpus size, coverage, findings count)

**Design pattern:** Fan-out/fan-in via Go channels. One goroutine per target handles the RPC loop. The coordinator goroutine multiplexes results.

```
            Coordinator goroutine
                    │
       ┌────────────┼────────────┐
       ▼            ▼            ▼
   target-A     target-B     target-C
   goroutine    goroutine    goroutine
       │            │            │
    [pipe+shm]   [pipe+shm]   [pipe+shm]
       │            │            │
   C process    Go process   Java process
```

### 2. Mutation Engine (`pkg/engine/mutator.go`)

Generates new inputs by mutating existing corpus entries.

**Strategies** (adapted from Go's `internal/fuzz` mutator and AFL++):

| # | Strategy | Description |
|---|----------|-------------|
| 1 | Bit flip | Flip 1, 2, or 4 consecutive bits |
| 2 | Byte flip | Flip 1, 2, or 4 consecutive bytes |
| 3 | Arithmetic | Add/subtract small values to 8/16/32-bit integers (both endiannesses) |
| 4 | Interesting values | Replace with known-interesting constants (0, 1, -1, 0x7f, 0x80, MAX_INT, etc.) |
| 5 | Random overwrite | Replace a random byte with a random value |
| 6 | Insert | Insert 1-N random bytes at a random position |
| 7 | Delete | Remove 1-N bytes from a random position |
| 8 | Duplicate | Copy a chunk of the input to another position |
| 9 | Splice | Combine two corpus entries (crossover) |
| 10 | Dictionary | Insert or replace with user-provided tokens |

The mutator selects strategies randomly with uniform probability. A PCG-based PRNG provides reproducible sequences from a seed.

### 3. Corpus Manager (`pkg/engine/corpus.go`)

Manages the set of inputs that drive fuzzing.

- **Seed corpus:** User-provided initial inputs loaded from a directory
- **Generated corpus:** Inputs discovered during fuzzing that expand coverage
- **Deduplication:** SHA-256 hash of input bytes prevents duplicates
- **Persistence:** Interesting inputs saved to `cache/` directory; findings saved to `findings/`
- **Selection:** Inputs selected uniformly at random for mutation (future: favor inputs with higher coverage scores)

### 4. Coverage System (`pkg/coverage/`)

Manages coverage collection, storage, and merging across all targets.

#### Coverage Bitmap

A **64KB (65,536-byte) array** of 8-bit saturating counters, following the AFL model:

- Each counter represents an edge (control-flow transition between basic blocks)
- Counter values are bucketed to powers of two: `{1, 2, 4, 8, 16, 32, 64, 128}` -- this groups "hit once" vs "hit a few times" vs "hit many times"
- An input is **interesting** if it produces a non-zero counter in any position where the global coverage bitmap has zero, or if it moves an existing counter to a higher bucket

#### Coverage Merging

```
Per-input execution:
  bitmap_combined = zeros(64KB)
  for each target:
    bitmap_combined |= target.bitmap    // bitwise OR

  if has_new_bits(global_bitmap, bitmap_combined):
    corpus.add(input)                   // interesting!
    global_bitmap |= bitmap_combined    // update global state
```

`has_new_bits` checks whether `bitmap_combined` has any byte `b` where `b & ^global[i] != 0` -- meaning a new edge or a new hit-count bucket.

#### Per-Language Coverage Collection

| Language | Instrumentation | How It Writes to Bitmap |
|----------|----------------|------------------------|
| C/C++ | `clang -fsanitize-coverage=trace-pc-guard` | Harness implements `__sanitizer_cov_trace_pc_guard_init` and `__sanitizer_cov_trace_pc_guard` callbacks; each guard ID is hashed to a bitmap index, counter incremented |
| Go | `go build -cover` (Go 1.20+ `runtime/coverage`) | Harness reads coverage counters after each execution and writes them into the shared memory bitmap |
| Java | JaCoCo agent in `premain` mode | Custom `IExecutionDataVisitor` reads probe data after each execution and maps it to bitmap indices |
| JavaScript | Istanbul-based AST instrumentation (Jazzer.js approach) | Instrumented source maintains a `__coverage__` object; harness copies counter values to the bitmap buffer after each execution |

### 5. Target Runner (`pkg/runner/`)

Manages the lifecycle of target processes.

**Process management:**
- Start target process with inherited file descriptors (pipes for RPC, shared memory FD)
- Monitor for crashes (non-zero exit, signals like SIGSEGV)
- Restart crashed processes automatically
- Enforce execution timeouts (kill after configurable deadline)
- Clean shutdown on SIGINT/SIGTERM

**Persistent mode** (default and recommended):
- Target process starts once and stays alive
- Processes inputs in a loop: read input from shared memory -> execute -> write output -> respond
- Avoids fork/exec overhead (~1ms per invocation), enabling thousands of executions per second

**One-shot mode** (fallback for targets that can't run persistently):
- New process spawned per input
- Coverage communicated via shared memory file that persists across invocations
- Much slower, but simpler and works with any executable

### 6. Wire Protocol (`pkg/protocol/`)

Defines the RPC contract between the coordinator and target harnesses.

**Transport:** OS pipes (inherited file descriptors 3 and 4 by convention -- fd 3 for coordinator->worker, fd 4 for worker->coordinator). stdin/stdout are left free for target function I/O or logging.

**Encoding:** Length-prefixed JSON messages.

```
Message format:
  [4 bytes: payload length, big-endian uint32]
  [N bytes: JSON payload]
```

**Message types:**

```
Coordinator -> Worker:
  {"type": "ping"}
  {"type": "fuzz", "timeout_ms": 1000}
  {"type": "minimize", "timeout_ms": 5000, "limit": 100}
  {"type": "shutdown"}

Worker -> Coordinator:
  {"type": "pong"}
  {"type": "fuzz_result", "ok": true, "error": "", "exec_time_ns": 1234}
  {"type": "minimize_result", "ok": true, "error": ""}
  {"type": "ready"}
```

**Shared Memory Layout** (per target, single mmap'd region):

```
Offset        Size        Field
────────────────────────────────────────────
0x000000      8 bytes     exec_count (uint64)
0x000008      4 bytes     input_len (uint32)
0x00000C      4 bytes     output_len (uint32)
0x000010      4 bytes     status (uint32: 0=ok, 1=error, 2=crash)
0x000014      44 bytes    reserved (padding to 64-byte header)
0x000040      1 MB        input region
0x100040      1 MB        output region
0x200040      64 KB       coverage bitmap
────────────────────────────────────────────
Total: ~2 MB + 64 KB per target
```

The coordinator writes to the input region; the worker writes to the output region and coverage bitmap. The header fields are updated atomically where necessary.

### 7. Comparison Framework (`pkg/compare/`)

The comparator is what makes cross_fuzz a *differential* fuzzer.

**Interface:**

```go
type Comparator interface {
    Name() string
    Compare(input []byte, outputs map[string][]byte) *Discrepancy
}

type Discrepancy struct {
    Input       []byte
    Outputs     map[string][]byte  // target name -> output bytes
    Description string
    Comparator  string
    Timestamp   time.Time
}
```

**Built-in comparators:**

| Comparator | Description |
|-----------|-------------|
| `ByteEqual` | All outputs must be byte-identical |
| `JSONStructural` | Parse as JSON, deep-compare ignoring key order and whitespace |
| `NumericTolerance` | Parse as number, compare with configurable epsilon |
| `ExitCode` | Compare execution status (success/error) across targets |
| `Regex` | All outputs must match (or not match) a given pattern |

**Custom comparators:**

Users can provide comparison logic in two ways:

1. **Go plugin:** Implement the `Comparator` interface, compile as a Go plugin (`.so`), loaded via `plugin.Open`
2. **Subprocess:** Any executable that reads a JSON payload on stdin and exits 0 (match) or 1 (discrepancy):

```json
{
  "input": "<base64>",
  "outputs": {
    "c_target": "<base64>",
    "go_target": "<base64>",
    "java_target": "<base64>"
  }
}
```

### 8. Harness Libraries (`harness/`)

Each target language has a thin library that handles the protocol, shared memory, and coverage plumbing. The user only writes the target function.

#### C/C++ Harness (`harness/c/`)

```c
#include "crossfuzz.h"

// User implements this function:
int crossfuzz_target(const uint8_t *data, size_t size,
                     uint8_t *out, size_t *out_size) {
    // process data, write result to out, set *out_size
    return 0; // 0 = success
}
```

The harness provides:
- `main()` that enters the persistent-mode loop
- Pipe protocol implementation (read commands from fd 3, write responses to fd 4)
- Shared memory mapping (mmap the file passed via environment variable `CROSSFUZZ_SHM_PATH`)
- SanitizerCoverage callbacks (`__sanitizer_cov_trace_pc_guard_init`, `__sanitizer_cov_trace_pc_guard`) that write to the coverage bitmap region

Build command:
```bash
clang -fsanitize-coverage=trace-pc-guard -o target target.c harness/c/crossfuzz.c
```

#### Go Harness (`harness/go/`)

```go
package main

import "github.com/user/cross_fuzz/harness/gofuzz"

func target(data []byte) ([]byte, error) {
    // process data, return result
}

func main() {
    gofuzz.Run(target)
}
```

The harness provides:
- `Run()` function that enters the persistent-mode loop
- Pipe protocol implementation via `os.NewFile(fd, ...)`
- Shared memory via `syscall.Mmap` on the FD from `CROSSFUZZ_SHM_FD`
- Coverage collection using `go build -cover` instrumentation and `runtime/coverage` APIs

#### Java Harness (`harness/java/`)

```java
import crossfuzz.Harness;

public class MyTarget extends Harness {
    @Override
    public byte[] target(byte[] data) throws Exception {
        // process data, return result
    }

    public static void main(String[] args) {
        new MyTarget().run();
    }
}
```

The harness provides:
- `run()` method with the persistent-mode loop
- Pipe protocol via `FileInputStream(fd=3)` / `FileOutputStream(fd=4)` (using `/proc/self/fd/N` on Linux)
- Shared memory via `FileChannel.map()` (MappedByteBuffer)
- Coverage via JaCoCo agent (`-javaagent:jacoco.jar=output=none`) with a custom execution data hook

#### JavaScript Harness (`harness/js/`)

```javascript
const crossfuzz = require('@crossfuzz/crossfuzz');

crossfuzz.run(function target(data) {
    // data is a Buffer, return a Buffer
    return result;
});
```

The harness provides:
- `run()` function with the persistent-mode loop
- Pipe protocol via `fs.createReadStream(null, { fd: 3 })` / `fs.createWriteStream(null, { fd: 4 })`
- Shared memory via a native N-API addon for `mmap` (with a fallback to file-based I/O)
- Coverage via Istanbul instrumentation of the target source (the harness instruments on load)

---

## Configuration

TOML-based configuration file:

```toml
[campaign]
name = "json_parser_diff"
timeout = "1h"                  # Total campaign duration
exec_timeout = "100ms"          # Per-execution timeout
max_input_size = 4096           # Maximum input size in bytes

[corpus]
seed_dir = "./seeds"            # User-provided seed inputs
corpus_dir = "./corpus"         # Auto-discovered interesting inputs
findings_dir = "./findings"     # Discrepancies and crashes

[[target]]
name = "c_json"
language = "c"
binary = "./targets/c_json_harness"
build_cmd = "clang -fsanitize-coverage=trace-pc-guard -o targets/c_json_harness targets/c_json.c harness/c/crossfuzz.c"
sanitizers = ["address", "undefined"]  # Optional: enable ASan/UBSan

[[target]]
name = "go_json"
language = "go"
binary = "./targets/go_json_harness"
build_cmd = "go build -cover -o targets/go_json_harness ./targets/go_json/"

[[target]]
name = "java_json"
language = "java"
binary = "java"
args = ["-javaagent:jacoco.jar=output=none", "-cp", "targets/", "JsonTarget"]
build_cmd = "javac -cp harness/java/crossfuzz.jar -d targets/ targets/JsonTarget.java"

[[target]]
name = "js_json"
language = "js"
binary = "node"
args = ["./targets/js_json_harness.js"]

[comparator]
type = "json_structural"        # Built-in comparator
# type = "custom"
# script = "./compare.py"       # Custom comparator subprocess
```

---

## Project Structure

```
cross_fuzz/
├── go.mod
├── go.sum
├── ARCHITECTURE.md
├── IMPLEMENTATION_PLAN.md
├── cmd/
│   └── crossfuzz/
│       └── main.go                 # CLI entry point
├── pkg/
│   ├── engine/
│   │   ├── coordinator.go          # Main fuzzing loop
│   │   ├── mutator.go              # Mutation strategies
│   │   ├── mutator_bytes.go        # Byte-level mutations
│   │   ├── corpus.go               # Corpus management
│   │   ├── minimize.go             # Input minimization
│   │   └── stats.go                # Live statistics
│   ├── runner/
│   │   ├── runner.go               # Target runner interface
│   │   ├── process.go              # Single process lifecycle
│   │   └── pool.go                 # Process pool (parallel targets)
│   ├── coverage/
│   │   ├── bitmap.go               # 64KB bitmap operations
│   │   └── shmem.go                # Shared memory management
│   ├── compare/
│   │   ├── compare.go              # Comparator interface
│   │   ├── byte_equal.go           # Byte equality
│   │   ├── json_structural.go      # JSON deep comparison
│   │   ├── numeric.go              # Numeric tolerance
│   │   └── custom.go               # Subprocess-based custom comparator
│   ├── protocol/
│   │   └── protocol.go             # Wire protocol types + codec
│   └── config/
│       └── config.go               # TOML config parsing
├── harness/
│   ├── c/
│   │   ├── crossfuzz.h             # C harness header
│   │   └── crossfuzz.c             # C harness implementation
│   ├── cpp/
│   │   └── crossfuzz.hpp           # C++ wrapper over C harness
│   ├── go/
│   │   └── crossfuzz.go            # Go harness package
│   ├── java/
│   │   └── src/crossfuzz/
│   │       ├── Harness.java        # Java harness base class
│   │       └── Target.java         # Target interface
│   └── js/
│       ├── crossfuzz.js            # JS harness module
│       └── package.json
└── examples/
    ├── base64/                     # Base64 encode/decode across languages
    │   ├── c_target.c
    │   ├── go_target.go
    │   ├── js_target.js
    │   ├── seeds/
    │   └── crossfuzz.toml
    └── json_parse/                 # JSON parser differential fuzz
        ├── c_target.c
        ├── go_target.go
        ├── java_target.java
        ├── js_target.js
        ├── seeds/
        └── crossfuzz.toml
```

---

## Design Trade-offs

| Decision | Alternative Considered | Why We Chose This |
|----------|----------------------|-------------------|
| Go orchestrator | Python, Rust | Go balances dev speed + concurrency + single binary. Goroutine-per-target is the natural model. |
| 64KB bitmap | Per-language separate coverage tracking | Unified bitmap is proven (AFL), enables cross-language coverage influence, fits in L1 cache |
| Pipes + shared memory | gRPC, Unix sockets, TCP | Pipes are the simplest reliable IPC; shared memory avoids copying hot-path data. Same pattern as Go's `internal/fuzz`. |
| Persistent mode default | One-shot (new process per input) | 100-1000x faster. The per-target process stays warm (JVM startup alone is ~200ms). |
| JSON protocol messages | Protobuf, msgpack, custom binary | JSON is human-debuggable, adequate for the control plane (messages are small and infrequent). Data plane uses shared memory. |
| TOML config | YAML, JSON, flags-only | TOML is readable, supports inline tables for target configs, has good Go libraries (BurntSushi/toml). |
