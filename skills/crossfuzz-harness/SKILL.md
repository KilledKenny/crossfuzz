---
name: crossfuzz-harness
description: Use this skill when the user wants to add a new target to a cross_fuzz campaign, write a crossfuzz.toml config, understand how harnesses work, choose which language to write a target in, learn about the IPC protocol between the coordinator and a target process, or understand the shared-memory layout, coverage bitmap, or pipe messages. Trigger for questions like "how do I add a new target?", "how do I write the config?", "what does a harness do?", "how does cross_fuzz communicate with my process?", "what does fd 3 do?", "what is the coverage bitmap format?", "how does the bitmap merge work?", "what is `instrument` for?", "how do I write an input filter?", "how do I write a comparator harness?", or "how do I set up a server target?".
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

| Language | Reference | Notes |
|----------|-----------|-------|
| C | `<skill-dir>/references/c.md` | Fastest; SanitizerCoverage via clang |
| C++ | `<skill-dir>/references/c.md` | Thin wrapper over C; same build flags |
| Go | `<skill-dir>/references/go.md` | Easy to use; needs `-cover` build flag and coverpkg list |
| Java | `<skill-dir>/references/java.md` | JVM startup hidden by persistent mode; needs `-javaagent` |
| JS/TS | `<skill-dir>/references/js.md` | Runs under Bun; Istanbul-based coverage |
| Python | `<skill-dir>/references/python.md` | Branch-arc coverage via the `coverage` library |
| Rust | `<skill-dir>/references/rust.md` | SanitizerCoverage via rustc LLVM flags; needs RUSTFLAGS at build time |

Pick the language your target implementation is already in, then read the matching reference file. Each per-language reference covers the full harness API: target signature, build command, Settings, Filter, Compare, and server-mode usage.

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
args     = ["run", "--preload", "../../harness/js/instrument.ts", "./client.ts"]
```

When coverage should come entirely from the server and the harness is a thin HTTP trigger, set `instrument: false` in the harness's Settings.

## Settings

All harnesses accept a Settings struct/object with these fields:

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | `true` | Collect and report coverage. Set `false` when the harness is a thin client and coverage comes from the server. |
| `transform` | `false` | Filter mode only: when `true`, the filter's returned bytes replace the original input for targets. |

Field naming varies by language: Go uses `Instrument`/`Transform`; C/C++/Java/JS/Python/Rust use `instrument`/`transform`. See each language reference for the exact form.

## TOML config

For the full annotated schema (campaign, corpus, target, comparator, input_filter) and real examples, read `<skill-dir>/references/config.md`.

## IPC protocol

For the wire protocol, shared memory layout, pipe message types, and coverage bitmap format, read `<skill-dir>/references/protocol.md`.

## Reference index

All references live in `<skill-dir>/references/`:

- `config.md` — TOML config schema and examples
- `protocol.md` — wire protocol and shared memory layout
- `c.md` — C and C++ targets
- `go.md` — Go targets
- `java.md` — Java targets
- `js.md` — JavaScript and TypeScript targets
- `python.md` — Python targets
- `rust.md` — Rust targets
