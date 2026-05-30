---
name: crossfuzz-cli
description: Use this skill whenever the user asks about running crossfuzz, understanding CLI commands (build/run/reduce/analyze), CLI flags, reading findings, or troubleshooting a fuzzing campaign. Trigger for questions like "how do I run crossfuzz?", "what does --workers do?", "what does `analyze` do?", "what comparator should I use?", or "why are there no findings?". For writing the crossfuzz.toml config itself, the **crossfuzz-harness** skill owns the schema.
---

# crossfuzz CLI

crossfuzz is a coverage-guided differential fuzzer. It feeds the same input to multiple language implementations of the same function and flags any output divergence.

## Workflow

```bash
# 1. Build coordinator (once)
make bin/crossfuzz

# 2. Build targets defined in your config (path defaults to ./crossfuzz.toml)
./bin/crossfuzz build [crossfuzz.toml]

# 3. Run a campaign (--build rebuilds first)
./bin/crossfuzz run [crossfuzz.toml] --build

# 4. Inspect a finding
./bin/crossfuzz analyze [crossfuzz.toml] --payload-path findings/

# 5. (Optional) Deduplicate corpus
./bin/crossfuzz reduce [crossfuzz.toml]
```

## Commands at a glance

| Command | Purpose |
|---------|---------|
| `build` | Run each target's `build_cmd` |
| `run` | Start the fuzzing campaign loop |
| `reduce` | Deduplicate corpus by coverage profile |
| `analyze` | Run one or more payloads and print hex-diff output |

For full flag documentation read `<skill-dir>/references/commands.md`.

## Config file

Five sections: `[campaign]`, `[corpus]`, `[[target]]` (one per implementation), `[comparator]`, and optionally `[input_filter]`.

For the complete annotated schema and real examples, see the **crossfuzz-harness** skill's `references/config.md` — TOML config ownership lives there alongside the harness API references.

### Minimal working config

```toml
[campaign]
name = "my_diff"
timeout = "30m"
exec_timeout = "500ms"
warmup_rounds = 10

[corpus]
seed_dir = "./seeds"
corpus_dir = "./corpus"
findings_dir = "./findings"

[[target]]
name = "go_impl"
language = "go"
binary = "./go_target_bin"
build_cmd = "cd go_target && go build -cover -covermode=atomic -o ../go_target_bin ."

[[target]]
name = "java_impl"
language = "java"
binary = "java"
args = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
        "-cp", "../../harness/java/build/libs/crossfuzz.jar:.", "MyTarget"]
build_cmd = "cd ../../harness/java && gradle jar && cd - && javac -cp ../../harness/java/build/libs/crossfuzz.jar MyTarget.java"

[comparator]
type = "byte_equal"
```

## Comparator quick-pick

| Type | When to use |
|------|-------------|
| `byte_equal` | Outputs must be byte-identical (base64, parsers returning canonical output) |
| `json_structural` | Both return JSON but may differ in key order / whitespace |
| `numeric` | Outputs are numbers; compare with absolute epsilon tolerance |
| `numeric_relative` | Same as `numeric` but uses relative tolerance |
| `none` | Server mode — one harness drives everything, no comparison needed |
| `custom` | Provide a script that reads JSON on stdin; exit 0 = match, non-zero = mismatch (stdout used as description if non-empty) |
| `harness` | Dedicated comparator process using the pipe protocol |

## Input filter

Use `[input_filter]` to reject structurally invalid inputs before they reach your targets:

```toml
[input_filter]
binary = "./url_filter"
build_cmd = "cd filter && go build -o ../url_filter ."
```

To have the filter rewrite accepted inputs (not just accept/reject them), `transform` must be enabled in **two** places: set `transform = true` in this `[input_filter]` block (so the coordinator reads the rewritten bytes back) **and** pass `Settings{Transform: true}` in the filter harness (so the filter writes them). Set only one and transform silently does nothing — the original input is forwarded unchanged.

## Findings

Each finding is saved as a raw binary file in `findings_dir/`. Run `analyze` to see per-target output side-by-side with diff highlighting:

```bash
./bin/crossfuzz analyze --payload-path ./findings/
```

## Flags vs. config

When a CLI flag mirrors a config field, an **explicitly passed flag takes precedence** over the config value; if the flag is omitted, the config value is used. This applies to `--timeout` (`[campaign] exec_timeout`), `--warmup` (`warmup_rounds`), `--corpus` (`corpus_dir`), and `--findings` (`findings_dir`). So `exec_timeout` set in the TOML is respected unless you override it on the command line.

## Most-used flags

```
--build               Build before running
--workers=N           Parallel workers, each with their own target processes (default 1).
                      Don't exceed `nproc`: each worker spawns a full copy of every target,
                      so memory scales linearly and oversubscribing CPUs hurts throughput.
--stop-after=VALUE    Stop after N executions per worker (integer; total = N×workers) or
                      after a duration, e.g. 30s, 2m
--max-findings=N      Stop after N findings (default 10)
--timeout=DURATION    Per-execution timeout override, e.g. 500ms, 5s; if unset uses config
                      exec_timeout (default 1s)
```

These are the flags you reach for most. For the full set — `--validate`, `--warmup`,
`--seed`, `--max-memory`, `--log-file`, `--corpus`, `--findings`, `--debug-edge`, and
the per-command flags for `reduce`/`analyze` — read `<skill-dir>/references/commands.md`.
