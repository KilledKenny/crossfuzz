# cross_fuzz Implementation Plan

## Summary

Five phases, from a working MVP with 2 targets and random mutation to a production-hardened tool supporting all 5 languages with coverage-guided differential fuzzing.

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

## Phase 3: Full Language Support

**Goal:** Add Java, JavaScript, and C++ harnesses. All 5 languages fully operational.

### Deliverables

| # | File | Description |
|---|------|-------------|
| 1 | `harness/java/src/crossfuzz/Harness.java` | Java harness: persistent loop, pipe protocol via `/proc/self/fd/N`, shared memory via `FileChannel.map()` |
| 2 | `harness/java/src/crossfuzz/Target.java` | `Target` interface: `byte[] fuzz(byte[] input)` |
| 3 | Java coverage integration | JaCoCo agent in `premain` mode; custom `IExecutionDataVisitor` copies probe data to shared memory bitmap after each execution |
| 4 | `harness/js/crossfuzz.js` | JS harness: persistent loop, pipe protocol via `fs.createReadStream/WriteStream(fd)`, shared memory via native N-API addon or file I/O fallback |
| 5 | JS coverage integration | Istanbul-based instrumentation of target source; `__coverage__` counters copied to bitmap after each execution |
| 6 | `harness/cpp/crossfuzz.hpp` | C++ harness: thin wrapper over C harness with `std::function`, `std::span`, RAII |
| 7 | `pkg/runner/pool.go` | Process pool: run multiple instances of the same target for intra-language parallelism |
| 8 | `examples/json_parse/` | JSON parser differential fuzz across all 5 languages |

### Language-specific challenges

**Java:**
- JVM startup is slow (~200ms) -- persistent mode is critical
- Shared memory: `FileChannel.map()` on `/proc/self/fd/N` works on Linux. For portability, consider passing the shm file path via environment variable.
- Coverage: JaCoCo's `Instrumenter` + `ExecutionDataStore` provide per-probe boolean arrays. Map probe IDs to bitmap indices via hash.

**JavaScript:**
- Node.js cannot natively `mmap` -- options:
  - **Native addon** (N-API): `mmap()` wrapper, ~50 lines of C. Best performance.
  - **File I/O fallback**: `fs.readFileSync`/`fs.writeFileSync` on the shm file. Simple but slower (~10x).
  - Start with file I/O, upgrade to native addon.
- Coverage: Instrument target source with Istanbul at load time. The `istanbul-lib-instrument` package provides an API for this.

**C++:**
- Trivial: wrap the C harness. Provide `crossfuzz::run(std::function<std::vector<uint8_t>(std::span<const uint8_t>)>)`.
- Same SanitizerCoverage mechanism as C.

### Definition of done
- `examples/json_parse/` runs with all 5 languages simultaneously
- Coverage collected from all 5 targets
- A JSON parsing discrepancy (e.g., number precision, unicode handling) is found

---

## Phase 4: Advanced Comparators and Minimization

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

## Phase 5: Production Hardening

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
| Java shared memory complexity | Medium | High | Use `/proc/self/fd/N` with `FileChannel.map()` on Linux; document Linux-only for Java targets initially |
| JS mmap native addon maintenance | Medium | Medium | Ship with file-I/O fallback that works everywhere; native addon is optional optimization |
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
- Phase 3 adds breadth (all 5 languages)
- Phase 4 adds precision (better comparison, minimization)
- Phase 5 adds reliability (production-grade robustness)
