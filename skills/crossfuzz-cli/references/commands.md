# CLI Commands Reference

## Usage

```
crossfuzz <command> <config.toml> [flags]
```

All commands take the config file as the second argument. Flags follow after.

---

## `build`

Runs each target's `build_cmd`, then the filter's and comparator's `build_cmd` if present.

```bash
crossfuzz build crossfuzz.toml [--name=t1,t2]
```

| Flag | Description |
|------|-------------|
| `--name=t1,t2` | Only build these targets (comma-separated names) |

Exits non-zero if any build fails.

---

## `run`

Starts the fuzzing campaign. Targets are started as persistent processes and kept alive for the duration.

```bash
crossfuzz run crossfuzz.toml [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--build` | false | Build all targets before starting |
| `--name=t1,t2` | all | Restrict to these targets |
| `--workers=N` | 1 | Parallel workers; each worker starts its own copy of all target processes. Do not exceed `nproc` — each worker is a full set of target processes, so memory scales linearly and oversubscribing CPUs hurts throughput. |
| `--max-findings=N` | 10 | Stop after this many unique findings |
| `--timeout=DURATION` | 5s | Per-execution timeout; target is killed and restarted on expiry (e.g. `500ms`, `5s`) |
| `--max-memory=SIZE` | 0 (none) | Virtual memory limit per target process (e.g. `512M`, `1G`) |
| `--validate=N` | 0 | Re-execute each new corpus input N times; log inputs whose output differs across runs |
| `--warmup=N` | 0 | Run the full corpus N times before the main fuzzing loop starts |
| `--corpus=DIR` | corpus | Directory for storing/loading corpus entries (overrides config `corpus_dir` if set) |
| `--findings=DIR` | findings | Directory for saving findings (overrides config `findings_dir` if set) |
| `--debug-edge` | false | Print per-target edge counts in the live status ticker |

**Status ticker** (printed every second):
```
[00:01:32] execs=94321 exec/s=1047 corpus=87 findings=0 edges=1203
```

**Coverage from all workers is merged** into a single global bitmap. More workers = more throughput, same coverage signal quality.

---

## `reduce`

Deduplicates the corpus by running all entries and keeping only those that cover at least one edge not covered by entries already kept. Useful after a long campaign to shrink a large corpus.

```bash
crossfuzz reduce crossfuzz.toml [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--build` | false | Build all targets before running |
| `--corpus=DIR` | corpus | Input corpus directory |
| `--corpus-reduced=DIR` | corpus-reduced | Output directory for the reduced corpus |
| `--validate=N` | 0 | Re-run each input N times to confirm stable coverage before keeping |

Output:
```
Reduced 1043 → 87 entries (saved to "corpus-reduced")
```

---

## `analyze`

Runs one or more raw payloads against all targets and prints per-target output as a hex dump with diff highlighting (differing bytes are colored).

```bash
crossfuzz analyze crossfuzz.toml --payload="hello"
crossfuzz analyze crossfuzz.toml --payload-path=./findings/
crossfuzz analyze crossfuzz.toml --payload-path=./findings/abc123 --build
```

| Flag | Description |
|------|-------------|
| `--payload=STRING` | Literal string bytes to send as input |
| `--payload-path=PATH` | File or directory of raw payload files; if a directory, all files are run |
| `--build` | Build targets before running |
| `--name=t1,t2` | Only run these targets |

Output per payload:
```
=== Payload: abc123 (47 bytes) ===
Input:
00000000  7b 22 6b 65 79 22 3a 31  ...

--- Target: go_impl ---
00000000  6f 62 6a 65 63 74 0a     object.

--- Target: java_impl ---
00000000  4f 62 6a 65 63 74 0a     Object.   ← differing bytes highlighted
```
