# TOML Config Reference

---

## `[campaign]`

```toml
[campaign]
name = "my_diff"          # Campaign name shown in output (required)
timeout = "30m"           # Total campaign duration; e.g. "30m", "1h", "2h30m"
exec_timeout = "500ms"    # Per-execution timeout; target killed & restarted on expiry
max_input_size = 4096     # Maximum input size in bytes. Optional; defaults to 4096 if omitted. Hard cap is 1 MB (the input region size).
```

`timeout` and `exec_timeout` accept Go duration strings: `ms`, `s`, `m`, `h`.

---

## `[corpus]`

```toml
[corpus]
seed_dir     = "./seeds"    # User-provided seed inputs (files with initial test cases)
corpus_dir   = "./corpus"   # Auto-discovered interesting inputs (written by coordinator)
findings_dir = "./findings" # Saved discrepancies and crashes
```

All three can be overridden at runtime with `--corpus=DIR` and `--findings=DIR`.

---

## `[[target]]`

Repeat for each implementation. Order matters only for display.

```toml
[[target]]
name      = "go_impl"        # Unique name (used in output and --name flag)
language  = "go"             # "c" | "cpp" | "go" | "java" | "js" | "python" | "rust"
binary    = "./go_target_bin"
args      = []               # Optional CLI arguments to binary
type      = "harness"        # "harness" (default) | "server"
build_cmd = "..."            # Shell command run by `crossfuzz build`
env       = ["FOO=bar"]      # Extra environment variables
```

### Language-specific patterns

**C**
```toml
binary = "./c_target"
build_cmd = "clang -fsanitize-coverage=trace-pc-guard -O2 -I ../../harness/c -o c_target c_target.c ../../harness/c/crossfuzz.c"
```

**C++**
```toml
binary = "./cpp_target"
build_cmd = "clang -fsanitize-coverage=trace-pc-guard -O2 -c ../../harness/c/crossfuzz.c -o crossfuzz_c.o && clang++ -std=c++23 -fsanitize-coverage=trace-pc-guard -O2 -I ../../harness/c -o cpp_target cpp_target.cpp ../../harness/cpp/crossfuzz.cpp crossfuzz_c.o && rm crossfuzz_c.o"
```

**Go**
```toml
binary = "./go_target_bin"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_target_bin ."
```

**Java**
```toml
binary = "java"
args   = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
          "-cp", "../../harness/java/build/libs/crossfuzz.jar:.", "MyTarget"]
build_cmd = "cd ../../harness/java && gradle jar && cd - && javac -cp ../../harness/java/build/libs/crossfuzz.jar MyTarget.java"
```

**JavaScript / TypeScript (Bun)**
```toml
binary = "bun"
args   = ["run", "--preload", "../../harness/js/instrument.ts", "./target.ts"]
build_cmd = "cd ../../harness/js && bun install"
```

### `type = "server"`

Server targets are long-running processes. A separate `type = "harness"` target acts as the client.

```toml
[[target]]
name     = "my_api"
type     = "server"
language = "go"
binary   = "./api_server"
build_cmd = "..."

[[target]]
name     = "js_harness"
type     = "harness"
language = "js"
binary   = "bun"
args     = ["run", "--preload", "../../harness/js/instrument.ts", "./harness.ts"]
```

See `examples/server_fuzz/` for a working multi-server example.

---

## `[comparator]`

```toml
[comparator]
type = "byte_equal"
```

| `type` | Description |
|--------|-------------|
| `byte_equal` | All outputs must be byte-identical |
| `json_structural` | Deep JSON comparison, ignores key order and whitespace |
| `numeric` | Parse as float, compare with absolute epsilon |
| `numeric_relative` | Parse as float, compare with relative tolerance |
| `none` | Never reports a discrepancy (server mode) |
| `custom` | External subprocess; reads JSON on stdin. Exit 0 = match, non-zero = mismatch. If the script writes a non-empty line to stdout, it is used as the discrepancy description |
| `harness` | Dedicated comparator process using the pipe protocol |

**`type = "custom"`**
```toml
[comparator]
type   = "custom"
script = "./compare.py"
```
Input JSON: `{"input": "<base64>", "outputs": {"target_a": "<base64>", "target_b": "<base64>"}}`

**`type = "harness"`**
```toml
[comparator]
type      = "harness"
binary    = "./compare_bin"
args      = []
build_cmd = "go build -o compare_bin ./compare/"
env       = []
```

---

## `[input_filter]`

Optional. Every generated input is sent here first; rejected inputs are discarded.

```toml
[input_filter]
binary    = "./url_filter"
args      = []
build_cmd = "cd filter && go build -o ../url_filter ."
env       = []
transform = false    # true = filter may rewrite the input; returned bytes replace original
```

---

## Real example configs

### `examples/base64/crossfuzz.toml` — 5 targets, byte_equal comparator

```toml
[campaign]
name = "base64_diff"
timeout = "30m"
exec_timeout = "500ms"
max_input_size = 1024

[corpus]
seed_dir = "./seeds"
corpus_dir = "./corpus"
findings_dir = "./findings"

[[target]]
name = "c_base64"
language = "c"
binary = "./c_target"
build_cmd = "clang -fsanitize-coverage=trace-pc-guard -O2 -I ../../harness/c -o c_target c_target.c ../../harness/c/crossfuzz.c"

[[target]]
name = "go_base64"
language = "go"
binary = "./go_target_bin"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_target_bin ."

[[target]]
name = "ts_base64"
language = "js"
binary = "bun"
args = ["run", "--preload", "../../harness/js/instrument.ts", "./target_ts.ts"]
build_cmd = "cd ../../harness/js && bun install"

[comparator]
type = "byte_equal"
```

### `examples/url_parse/crossfuzz.toml` — with input_filter

```toml
[campaign]
name = "url_parse_diff"
timeout = "30m"
exec_timeout = "1000ms"
max_input_size = 1024

[corpus]
seed_dir = "./seeds"
corpus_dir = "./corpus"
findings_dir = "./findings"

[[target]]
name = "go_url"
language = "go"
binary = "./go_url_target"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_url_target ."

[[target]]
name = "java_url"
language = "java"
binary = "java"
args = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
        "-cp", "../../harness/java/build/libs/crossfuzz.jar:.", "JavaTarget"]
build_cmd = "cd ../../harness/java && gradle jar && cd - && javac -cp ../../harness/java/build/libs/crossfuzz.jar JavaTarget.java"

[comparator]
type = "byte_equal"

[input_filter]
binary = "./url_filter"
build_cmd = "cd filter && go build -o ../url_filter ."
```
