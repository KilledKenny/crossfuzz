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

# 2. Build targets defined in your config
./bin/crossfuzz build crossfuzz.toml

# 3. Run a campaign (--build rebuilds first)
./bin/crossfuzz run crossfuzz.toml --build

# 4. Inspect a finding
./bin/crossfuzz analyze crossfuzz.toml --payload-path findings/

# 5. (Optional) Deduplicate corpus
./bin/crossfuzz reduce crossfuzz.toml
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

With `transform = true` in the filter's Settings, the returned bytes replace the original input for targets.

## Findings

Each finding is saved as a raw binary file in `findings_dir/`. Run `analyze` to see per-target output side-by-side with diff highlighting:

```bash
./bin/crossfuzz analyze crossfuzz.toml --payload-path ./findings/
```

## Most-used flags

```
--build               Build before running
--workers=N           Parallel workers, each with their own target processes (default 1)
--max-findings=N      Stop after N findings (default 10)
--timeout=DURATION    Per-execution timeout, e.g. 500ms, 5s (default 5s)
--max-memory=SIZE     Virtual memory limit per target, e.g. 512M, 1G
--validate=N          Re-run each new input N times to confirm output stability
--warmup=N            Run corpus N times before main loop
--name=t1,t2          Only run/build these named targets
--corpus=DIR          Override corpus directory (default: corpus)
--findings=DIR        Override findings directory (default: findings)
```
