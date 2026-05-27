# TOML Config Reference

---

## `[campaign]`

```toml
[campaign]
name = "my_diff"          # Campaign name shown in output (required)
timeout = "30m"           # Total campaign duration; e.g. "30m", "1h", "2h30m"
exec_timeout = "500ms"    # Per-execution timeout; target killed & restarted on expiry
max_input_size = 4096     # Maximum input size in bytes. Optional; defaults to 4096 if omitted. Hard cap is 1 MB (the input region size).
warmup_rounds = 10        # Run the corpus N times through worker 0 before the main loop to stabilise flaky coverage edges. Optional; defaults to 0.
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

**C** (CMake — recommended; requires the crossfuzz harness installed system-wide, see `c-install.md`)
```toml
binary = "./build/c_target"
build_cmd = "cmake -B build -DCMAKE_C_COMPILER=clang -DCMAKE_BUILD_TYPE=Release && cmake --build build"
```

The target's `CMakeLists.txt` calls `find_package(crossfuzz REQUIRED)` and links `crossfuzz::c`. See `c.md` for the full pattern (pkg-config also supported).

**C++** (CMake)
```toml
binary = "./build/cpp_target"
build_cmd = "cmake -B build -DCMAKE_CXX_COMPILER=clang++ -DCMAKE_BUILD_TYPE=Release && cmake --build build"
```

Link `crossfuzz::cpp` in the target's `CMakeLists.txt`. See `c.md`.

**Go**
```toml
binary = "./go_target_bin"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_target_bin ."
```

**Java** (Maven — harness pulled from Maven Central as `io.killedkenny.crossfuzz:crossfuzz`)
```toml
binary = "java"
args   = ["-javaagent:crossfuzz.jar", "-cp", "crossfuzz.jar:target/classes", "MyTarget"]
build_cmd = "mvn compile"
```

**Java** (Gradle)
```toml
binary = "java"
args   = ["-javaagent:crossfuzz.jar", "-cp", "crossfuzz.jar:build/classes/java/main", "MyTarget"]
build_cmd = "gradle downloadAgent compileJava"
```

See `java.md` for the `pom.xml` / `build.gradle` snippets that copy `crossfuzz.jar` into the project root.

**JavaScript / TypeScript (Bun)**
```toml
binary = "bun"
args   = ["run", "--preload", "@crossfuzz/crossfuzz/instrument.ts", "./target.ts"]
build_cmd = "bun add @crossfuzz/crossfuzz"
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
args     = ["run", "--preload", "@crossfuzz/crossfuzz/instrument.ts", "./harness.ts"]
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
binary = "./c/build/c_target"
build_cmd = "cmake -B c/build -S c -DCMAKE_C_COMPILER=clang -DCMAKE_BUILD_TYPE=Release && cmake --build c/build"

[[target]]
name = "go_base64"
language = "go"
binary = "./go_target_bin"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_target_bin ."

[[target]]
name = "ts_base64"
language = "js"
binary = "bun"
args = ["run", "--preload", "@crossfuzz/crossfuzz/instrument.ts", "./target_ts.ts"]
build_cmd = "bun add @crossfuzz/crossfuzz"

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
args = ["-javaagent:crossfuzz.jar", "-cp", "crossfuzz.jar:target/classes", "JavaTarget"]
build_cmd = "mvn compile"

[comparator]
type = "byte_equal"

[input_filter]
binary = "./url_filter"
build_cmd = "cd filter && go build -o ../url_filter ."
```
