---
name: crossfuzz-harness
description: Use this skill when the user wants to add a new target to a crossfuzz campaign, write a crossfuzz.toml config, understand how harnesses work, choose which language to write a target in, learn about the IPC protocol between the coordinator and a target process, or understand the shared-memory layout, coverage bitmap, or pipe messages. Trigger for questions like "how do I add a new target?", "how do I write the config?", "what does a harness do?", "how does crossfuzz communicate with my process?", "what does fd 3 do?", "what is the coverage bitmap format?", "how does the bitmap merge work?", "what is `instrument` for?", "how do I write an input filter?", "how do I write a comparator harness?", or "how do I set up a server target?".
---

# crossfuzz Harness Overview

A **harness** is a thin library that handles the IPC between the crossfuzz coordinator and your target process. You write the target function; the harness handles everything else.

## What a harness does

1. Opens the shared memory region pointed to by `CROSSFUZZ_SHM` env var
2. Opens the command pipe (fd 3 in, fd 4 out)
3. Sends `{"type":"ready"}` to signal it is ready
4. Loops: read input from shared memory → call your function → write output → collect coverage → send `{"type":"fuzz_result"}`

You never touch pipes or shared memory directly.

Pick the language your target implementation is already in and read the matching reference file from the index at the bottom. Each per-language reference covers the full harness API: target signature, build command, Settings, Filter, Compare, and server-mode usage.

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
type     = "harness"          # Sends HTTP requests and reports results
language = "js"
binary   = "bun"
args     = ["run", "--preload", "@crossfuzz/crossfuzz/instrument.ts", "./client.ts"]
```

When coverage should come entirely from the server and the harness is a thin HTTP trigger, set `instrument: false` in the harness's Settings.

## Settings

All harnesses accept a Settings struct/object with these fields:

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | `true` | Collect and report coverage. Set `false` when the harness is a thin client and coverage comes from the server. |
| `transform` | `false` | Filter mode only: when `true`, the filter's returned bytes replace the original input for targets. |

Field naming varies by language: Go uses `Instrument`/`Transform`; C/C++/Java/JS/Python/Rust use `instrument`/`transform`. See each language reference for the exact form.

## Reference index

All references live in `<skill-dir>/references/`:

- `config.md` — TOML config schema (campaign, corpus, target, comparator, input_filter) and real examples
- `protocol.md` — wire protocol, pipe message types, shared memory layout, coverage bitmap format
- `c.md` — C and C++ targets. Fastest; SanitizerCoverage via clang
- `go.md` — Go targets. Easy to use; needs `-cover` and a coverpkg list
- `java.md` — Java targets. Persistent JVM hides startup cost; needs `-javaagent`
- `js.md` — JavaScript / TypeScript targets. Runs under Bun; Istanbul-based coverage
- `python.md` — Python targets. Branch-arc coverage via the `coverage` library
- `rust.md` — Rust targets. SanitizerCoverage via rustc LLVM flags; needs RUSTFLAGS at build time
