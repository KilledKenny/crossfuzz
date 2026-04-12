---
name: crossfuzz-harness
description: Use this skill when the user wants to add a new target to a cross_fuzz campaign, understand how harnesses work, choose which language to write a target in, or learn about the IPC protocol between the coordinator and a target process. Trigger for questions like "how do I add a new target?", "what does a harness do?", "how does cross_fuzz communicate with my process?", "what is DisableInstrumentation for?", "how do I write an input filter?", "how do I write a comparator harness?", or "how do I set up a server target?".
---

# cross_fuzz Harness Overview

A **harness** is a thin library that handles the IPC between the cross_fuzz coordinator and your target process. You write the target function; the harness handles everything else.

## What a harness does

1. Opens the shared memory region pointed to by `CROSSFUZZ_SHM` env var
2. Opens the command pipe (fd 3 in, fd 4 out)
3. Sends `{"type":"ready"}` to signal it is ready
4. Loops: read input from shared memory → call your function → write output → collect coverage → send `{"type":"fuzz_result"}`

You never touch pipes or shared memory directly.

## Choosing a language

| Language | Skill | Notes |
|----------|-------|-------|
| C | `crossfuzz-harness-c` | Fastest; SanitizerCoverage via clang |
| C++ | `crossfuzz-harness-c` | Thin wrapper over C; same build flags |
| Go | `crossfuzz-harness-go` | Easy to use; needs `-cover` build flag and coverpkg list |
| Java | `crossfuzz-harness-java` | JVM startup hidden by persistent mode; needs `-javaagent` |
| JS/TS | `crossfuzz-harness-js` | Runs under Bun; Istanbul-based coverage |
| Rust | `crossfuzz-harness-rust` | SanitizerCoverage via rustc LLVM flags; needs RUSTFLAGS at build time |

Pick the language your target implementation is already in.

## Three entry points per harness

| Entry point | Role |
|-------------|------|
| `Fuzz` / `fuzz()` | Normal fuzzing target — receives input bytes, returns output bytes |
| `Filter` / `filter()` | Input filter — accepts or rejects (and optionally transforms) inputs before they reach targets |
| `Compare` / `compare()` | Custom comparator — reads all targets' outputs from shared memory and reports mismatches |

## Target types in the TOML config

### `type = "harness"` (default)
The coordinator communicates with the target via pipes + shared memory. Normal mode for all languages.

### `type = "server"`
For long-running server processes (e.g. HTTP servers). The coordinator does not pipe-communicate with the server process. A separate harness target acts as the client, sends requests, and reports results. The server's coverage is collected from its own shared memory.

```toml
[[target]]
name     = "my_api"
type     = "server"
language = "go"
binary   = "./api_server"

[[target]]
name     = "js_client"
type     = "harness"          # This one sends HTTP requests and reports results
language = "js"
binary   = "bun"
args     = ["run", "--preload", "../../harness/js/instrument.ts", "./client.ts"]
```

When coverage should come entirely from the server and the harness is a thin HTTP trigger, set `Instrument: false` in the harness's Settings.

## Settings

All harnesses accept a Settings struct/object:

| Field | Default | Description |
|-------|---------|-------------|
| `Instrument` / `instrument` | `true` | Collect and report coverage. Set `false` when the harness is a thin client and coverage comes from the server. |
| `Warmup` / `warmup` | `0` | Go only: run extra iterations on first input to mask flaky GC/allocator coverage slots |
| `Transform` / `transform` | `false` | Filter mode only: when `true`, the filter's returned bytes replace the original input for targets |

## For the wire protocol and shared memory layout

Read `<skill-dir>/references/protocol.md`.
