# cross_fuzz Implementation Plan

## Summary

Seven phases, from a working MVP with 2 targets and random mutation to a production-hardened tool supporting all 5 languages with coverage-guided differential fuzzing.

---

## Phase 1: Foundation (MVP)

**Goal:** Orchestrator runs 2 targets (C + Go), feeds them identical inputs via shared memory, compares outputs with byte equality, reports discrepancies. Random mutation only -- no coverage guidance yet.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `go.mod` | Go module initialization |
| 2 | `pkg/protocol/protocol.go` | Wire protocol types (message structs) and length-prefixed JSON codec |
| 3 | `pkg/coverage/shmem.go` | Shared memory management: create temp file, `mmap`, FD inheritance helpers |
| 4 | `pkg/runner/process.go` | Start a target process with inherited FDs (pipe + shm), send/receive protocol messages |
| 5 | `pkg/runner/runner.go` | Target runner interface: `Start()`, `Execute(input) -> output`, `Stop()` |
| 6 | `pkg/compare/compare.go` | `Comparator` interface and `Discrepancy` type |
| 7 | `pkg/compare/byte_equal.go` | Byte-equality comparator |
| 8 | `pkg/config/config.go` | TOML config parsing (campaign, targets, comparator) |
| 9 | `pkg/engine/coordinator.go` | Main loop: pick random input, dispatch to all targets, compare outputs, report |
| 10 | `harness/c/crossfuzz.h` + `crossfuzz.c` | C harness: persistent-mode loop, pipe protocol, shared memory (no coverage yet) |
| 11 | `harness/go/crossfuzz.go` | Go harness: same as C but in Go |
| 12 | `cmd/crossfuzz/main.go` | CLI entry point: parse config, start coordinator |
| 13 | `examples/base64/` | Base64 encode in C and Go with seed corpus and config |

### Key decisions in this phase
- Establish the shared memory layout (header + input + output + coverage regions)
- Establish the pipe protocol message format
- Establish the FD inheritance convention (fd 3 = coordinator->worker, fd 4 = worker->coordinator)
- The coordinator loop is single-threaded: iterate over targets sequentially

### Definition of done
- `crossfuzz run examples/base64/crossfuzz.toml` starts both targets, feeds random byte inputs, and logs any output discrepancies
- Introduce a deliberate bug in one target (e.g., wrong padding for certain input lengths) and confirm it is found

---

## Phase 2: Coverage-Guided Mutation

**Goal:** Add coverage collection from C and Go targets. Implement mutation engine. Inputs that expand combined coverage are saved to the corpus.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `pkg/coverage/bitmap.go` | 64KB bitmap operations: `HasNewBits(global, current)`, `Merge(dst, src)`, `CountBits()`, `ResetCounters()`, power-of-two bucketing |
| 2 | `pkg/engine/mutator.go` | Mutator core: strategy selection, PRNG state, dictionary support |
| 3 | `pkg/engine/mutator_bytes.go` | All 10 byte-level mutation strategies |
| 4 | `pkg/engine/corpus.go` | Corpus manager: load seeds, add interesting inputs, SHA-256 dedup, save to disk |
| 5 | `pkg/engine/stats.go` | Live statistics: execs/sec, corpus size, total coverage bits, findings count |
| 6 | Update `harness/c/` | Add SanitizerCoverage callbacks: `__sanitizer_cov_trace_pc_guard_init`, `__sanitizer_cov_trace_pc_guard` writing to bitmap in shared memory |
| 7 | Update `harness/go/` | After each execution, copy Go coverage counters into the shared memory bitmap |
| 8 | Update `pkg/engine/coordinator.go` | Coverage-guided loop: mutate -> execute all -> merge bitmaps -> check for new bits -> compare outputs |

### Coverage feedback loop (pseudocode)

```
global_bitmap = zeros(64KB)
corpus = load_seeds(config.seed_dir)

loop:
    input = mutator.mutate(corpus.pick_random())
    
    for each target in targets:
        target.shmem.write_input(input)
        target.shmem.reset_coverage()
        target.send("fuzz")
        target.recv()   // blocks until execution complete
    
    combined = zeros(64KB)
    for each target in targets:
        combined |= target.shmem.read_coverage()
    
    if has_new_bits(global_bitmap, combined):
        corpus.add(input)
        global_bitmap |= combined
    
    outputs = {t.name: t.shmem.read_output() for t in targets}
    if discrepancy := comparator.compare(input, outputs); discrepancy != nil:
        save_finding(discrepancy)
    
    stats.update(len(input), time_elapsed)
```

### Definition of done
- Corpus grows over time as coverage expands
- Stats show increasing coverage percentage
- The fuzzer finds discrepancies that random testing would take much longer to find
- C targets compiled with `-fsanitize-coverage=trace-pc-guard` produce meaningful coverage data

---

## Phase 3: Java Harness

**Goal:** Add Java harness with coverage via `java.lang.instrument`. Java targets fully operational alongside C and Go.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `harness/java/src/crossfuzz/Harness.java` | Java harness: persistent loop, pipe protocol via `/proc/self/fd/N`, shared memory via `FileChannel.map()` |
| 2 | `harness/java/src/crossfuzz/Target.java` | `Target` interface: `byte[] fuzz(byte[] input)` |
| 3 | `harness/java/src/crossfuzz/CoverageAgent.java` | `premain` agent using `java.lang.instrument.Instrumentation`; registers a `ClassFileTransformer` that injects coverage callbacks at basic block entries |
| 4 | `harness/java/src/crossfuzz/CoverageTransformer.java` | ASM-based bytecode transformer: assigns each basic block a stable hash-derived bitmap index, injects `CoverageRuntime.hit(index)` calls |
| 5 | `harness/java/src/crossfuzz/CoverageRuntime.java` | `hit(int index)`: writes a 1 to the shared memory bitmap at the given index (mapped via `FileChannel.map()` over the inherited shm FD) |
| 6 | `harness/java/META-INF/MANIFEST.MF` | Declares `Premain-Class`, `Can-Retransform-Classes: true` |
| 7 | `examples/json_parse/JavaTarget.java` | JSON parser target implemented in Java |

### Language-specific notes

- JVM startup is slow (~200ms) -- persistent mode is critical; the harness loop must stay resident between executions.
- Shared memory: `FileChannel.map()` on `/proc/self/fd/N` works on Linux. The shm file path is also passed via `CROSSFUZZ_SHM_PATH` for portability.
- Coverage: `java.lang.instrument` gives access to raw bytecode at class-load time. The ASM library transforms each class to inject `CoverageRuntime.hit(index)` at every basic block. Map block identities to bitmap indices via `(className + blockId).hashCode() & 0xFFFF`.
- The agent JAR is passed with `-javaagent:crossfuzz-agent.jar` in the target's `launch_cmd`.

### Definition of done
- Java target runs persistently and responds to pipe protocol messages
- Coverage bitmap fills as code paths are exercised
- `examples/json_parse/` runs with C, Go, and Java simultaneously

---

## Phase 4: C++ Harness

**Goal:** Add C++ harness. Thin wrapper over the C harness -- straightforward since it shares the same SanitizerCoverage mechanism.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `harness/cpp/crossfuzz.hpp` | C++ harness: RAII wrapper over C harness with `std::function<std::vector<uint8_t>(std::span<const uint8_t>)>` entry point |
| 2 | `examples/json_parse/CppTarget.cpp` | JSON parser target implemented in C++ |

### Language-specific notes

- Trivial: `crossfuzz.hpp` `#include`s `crossfuzz.h` and wraps the C entry point with a type-safe C++ lambda interface.
- Same `-fsanitize-coverage=trace-pc-guard` compilation flags as C targets; no new coverage infrastructure needed.
- Provide `crossfuzz::run(std::function<...>)` that internally calls the C `crossfuzz_run` with a trampoline.

### Definition of done
- C++ target compiles with `clang++ -fsanitize-coverage=trace-pc-guard` and runs via the existing runner
- `examples/json_parse/` runs with C, Go, Java, and C++ simultaneously

---

## Phase 5: JavaScript Harness

**Goal:** Add JavaScript harness using Bun. Bun's native `mmap` support simplifies shared memory; Istanbul provides coverage instrumentation.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `harness/js/crossfuzz.ts` | Bun harness: persistent loop, pipe protocol via `Bun.file(fd)` streams, shared memory via `Bun.mmap()` over the inherited shm FD |
| 2 | `harness/js/instrument.ts` | Load-time Istanbul instrumentation: wraps `require`/`import` to instrument target source; copies `__coverage__` counters into the bitmap after each execution |
| 3 | `examples/json_parse/target.ts` | JSON parser target implemented in TypeScript/Bun |
| 4 | `pkg/runner/pool.go` | Process pool: run multiple instances of the same target for intra-language parallelism |
| 5 | `examples/json_parse/` | Complete JSON parser differential fuzz example across all 5 languages |

### Language-specific notes

- Bun exposes `Bun.mmap(fd, size)` natively -- no native addon or file-I/O fallback needed. Map the inherited shm FD directly.
- Pipe I/O: use `Bun.stdin`/`Bun.stdout` or `new ReadableStream` over the inherited pipe FDs.
- Coverage: instrument target source with `istanbul-lib-instrument` at load time. After each execution, iterate `__coverage__[file][statementMap]` counters and OR them into the bitmap.
- Bun startup is fast (<5ms) but persistent mode is still preferred to amortize instrumentation overhead.

### Definition of done
- `examples/json_parse/` runs with all 5 languages simultaneously
- Coverage collected from all 5 targets
- A JSON parsing discrepancy (e.g., number precision, unicode handling) is found

---

## Phase 6: Advanced Comparators and Minimization

**Goal:** Rich comparison framework, input minimization, structured findings.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `pkg/compare/json_structural.go` | JSON comparator: parse both sides, deep-compare ignoring key order. Report structural diff. |
| 2 | `pkg/compare/numeric.go` | Numeric comparator: parse as float64, compare with configurable epsilon. Support `NaN`, `Inf` handling. |
| 3 | `pkg/compare/custom.go` | Subprocess comparator: spawn user's script, pass JSON payload on stdin, read exit code. |
| 4 | `pkg/engine/minimize.go` | Input minimizer: given a discrepancy-triggering input, iteratively shrink it while preserving the discrepancy. Strategies: binary reduction, byte deletion, chunk removal. |
| 5 | Findings directory structure | Each finding saved as a directory: `findings/<hash>/input.bin`, `output_<target>.bin`, `metadata.json` |
| 6 | Text/JSON report generation | Summary report: total findings, grouped by comparator, with minimized reproduction cases |

### Minimization algorithm

```
minimize(input, targets, comparator):
    // Binary reduction
    for chunk_size = len(input)/2; chunk_size >= 1; chunk_size /= 2:
        for offset = 0; offset < len(input); offset += chunk_size:
            candidate = input[:offset] + input[offset+chunk_size:]
            if still_triggers_discrepancy(candidate, targets, comparator):
                input = candidate
    
    // Byte-by-byte deletion
    for i = 0; i < len(input); i++:
        candidate = input[:i] + input[i+1:]
        if still_triggers_discrepancy(candidate, targets, comparator):
            input = candidate
            i--  // re-check this position
    
    return input
```

### Definition of done
- JSON structural comparator correctly identifies key-order-independent differences
- Custom subprocess comparator works with a Python comparison script
- Minimizer reduces a 4KB discrepancy input to <100 bytes

---

## Phase 7: Production Hardening

**Goal:** Robustness, performance, usability for real-world campaigns.

### Deliverables

| # | Description |
|---|-------------|
| 1 | **Timeout handling:** Per-execution timeout. If a target doesn't respond within the deadline, kill and restart it. Save the input as a timeout finding. |
| 2 | **Crash detection:** Detect target process crash (SIGSEGV, SIGABRT, non-zero exit). Save crashing input. Restart process. |
| 3 | **OOM protection:** Set memory limits per target via `setrlimit(RLIMIT_AS)`. Detect OOM kills. |
| 4 | **Parallel fuzzing:** Multiple coordinator workers, each with their own set of target processes. Shared corpus with lock-free concurrent access. |
| 5 | **CLI subcommands:** `crossfuzz run` (start campaign), `crossfuzz build` (compile targets), `crossfuzz minimize <input>` (minimize a specific input), `crossfuzz report` (generate summary report) |
| 6 | **Build integration:** `crossfuzz build` reads `build_cmd` from config for each target and compiles them with appropriate instrumentation flags |
| 7 | **Signal handling:** SIGINT triggers graceful shutdown -- save corpus, stop targets, write final stats |
| 8 | **Comprehensive tests:** Unit tests for bitmap operations, mutation strategies, protocol codec, comparators. Integration tests with actual multi-language targets. |
| 9 | **Documentation:** Getting started guide, harness writing guide, comparator guide |

### Parallel architecture

```
                  ┌─────────────────┐
                  │  Main Process   │
                  │  (corpus mgr)   │
                  └───────┬─────────┘
                          │ shared corpus
            ┌─────────────┼─────────────┐
            ▼             ▼             ▼
     ┌─────────┐   ┌─────────┐   ┌─────────┐
     │ Worker 1│   │ Worker 2│   │ Worker 3│
     │(coord.) │   │(coord.) │   │(coord.) │
     └────┬────┘   └────┬────┘   └────┬────┘
       ┌──┼──┐       ┌──┼──┐       ┌──┼──┐
       ▼  ▼  ▼       ▼  ▼  ▼       ▼  ▼  ▼
      C  Go  JS     C  Go  JS     C  Go  JS
```

Each worker has its own set of target processes. Workers share the corpus and global coverage bitmap. Coverage bitmap updates use atomic OR operations for lock-free concurrent access.

### Definition of done
- Campaign runs for 1 hour without crashes or hangs in the orchestrator
- Graceful shutdown saves all state
- `crossfuzz build && crossfuzz run` workflow works end-to-end
- Tests pass for all components

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Java instrumentation overhead | Medium | Medium | `java.lang.instrument` transforms happen at class-load time; amortized cost per execution is small. Benchmark and limit instrumentation to application classes only (exclude JDK internals). |
| JS Bun mmap API stability | Low | Medium | `Bun.mmap` is a stable Bun API; pin the Bun version in CI to avoid surprises |
| Go coverage counter access changes between Go versions | Low | Medium | Use `go build -cover` (stable API since Go 1.20); avoid `//go:linkname` hacks |
| Coverage granularity mismatch across languages | High | Low | Accept approximate coverage unification; the goal is to find discrepancies, not measure exact coverage. Even coarse coverage from one language helps guide mutation. |
| Target process instability under fuzzing | High | Medium | Robust restart with state recovery; save last N inputs for reproduction |
| Performance bottleneck on slowest target | High | Medium | Accept throughput limited by slowest target. In Phase 5, add option to run fast targets at higher frequency and compare asynchronously. |

---

## MVP Quick-Start

The absolute minimum viable product (buildable in ~3 days):

1. Pipe-based communication only (no shared memory)
2. Two targets: C + Go
3. Random byte mutation (no coverage)
4. Byte-equality comparison
5. Findings printed to stdout

This proves the core value proposition: **"same input, multiple languages, spot the difference."**

From there, each phase adds a meaningful capability:
- Phase 2 adds intelligence (coverage-guided mutation)
- Phase 3 adds Java (instrument-based coverage, persistent JVM)
- Phase 4 adds C++ (trivial wrap over C harness)
- Phase 5 adds JavaScript (Bun with native mmap, Istanbul coverage)
- Phase 6 adds precision (better comparison, minimization)
- Phase 7 adds reliability (production-grade robustness)
