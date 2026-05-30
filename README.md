# crossfuzz

Coverage-guided **differential fuzzer** across C, C++, Go, Java, JavaScript / TypeScript, Python, and Rust.

crossfuzz feeds the same generated input to multiple implementations of the same function, collects coverage from every target into a shared bitmap, and uses that combined coverage to guide mutation. Whenever the outputs disagree, the input is saved as a finding. A new code path discovered in one language can lead to a bug being uncovered in another.

## Supported languages

| Language       | Instrumentation                                     |
|----------------|------------------------------------------------------|
| C / C++        | `clang -fsanitize-coverage=trace-pc-guard`           |
| Go             | `go build -cover -covermode=atomic`                  |
| Java           | `crossfuzz.jar` javaagent (bytecode instrumentation) |
| JS / TypeScript| Bun + Istanbul preload (`@crossfuzz/crossfuzz`)      |
| Python         | `harness/python` (sys.settrace)                      |
| Rust           | `cargo rustc` with `sancov-module`                   |

## Install

**Prerequisite:** Go is the only thing required to build the coordinator itself. Per-language toolchains (`clang`, `bun`, `openjdk` + `gradle`, `python3`, `cargo`) are only needed for the targets you actually want to fuzz.

### From source (`go install`)

```bash
go install github.com/KilledKenny/crossfuzz/cmd/crossfuzz@latest
```

### Prebuilt binaries

```bash
curl -fsSL https://raw.githubusercontent.com/KilledKenny/crossfuzz/main/install.sh | bash
```

Or download a binary directly from the [latest GitHub Release](https://github.com/KilledKenny/crossfuzz/releases/latest).

### Local clone

```bash
git clone https://github.com/KilledKenny/crossfuzz
cd crossfuzz
make bin/crossfuzz     # just the coordinator
make                   # coordinator + all bundled harnesses (Java jar, JS, Python, Rust)
```

### Shell completion

crossfuzz ships completion for bash, zsh, fish and powershell, including
completion of the `<config.toml>` argument and `--name` target names. For bash:

```bash
# try it in the current shell
source <(crossfuzz completion bash)

# install permanently (Linux)
crossfuzz completion bash | sudo tee /etc/bash_completion.d/crossfuzz >/dev/null
```

Bash completion requires the `bash-completion` package. For other shells run
`crossfuzz completion zsh|fish|powershell` and see `crossfuzz completion --help`.

## AI assistant skills

crossfuzz ships skills that teach your AI assistant the CLI and harness APIs for all supported languages.

### Claude Code

```
/plugin marketplace add KilledKenny/crossfuzz
/plugin install crossfuzz-skills@crossfuzz
/reload-plugins
```

### Other tools (agent-skills-cli)

```bash
npx agent-skills-cli add KilledKenny/crossfuzz
```

## Basic usage

crossfuzz is driven by a TOML config that lists the targets to fuzz and how to compare their outputs. The three commands you will use most:

```bash
crossfuzz build path/to/crossfuzz.toml      # run each target's build_cmd
crossfuzz run   path/to/crossfuzz.toml      # start the fuzzing campaign
crossfuzz analyze path/to/crossfuzz.toml --payload "hello"   # one-shot: send a payload, print each target's output
```

Useful flags on `run`:

| Flag           | What it does                                                        |
|----------------|---------------------------------------------------------------------|
| `--build`      | Run `build` first, then start the campaign.                         |
| `--name`       | Comma-separated list of targets to include (default: all).          |
| `--workers N`  | Run N parallel fuzzing workers, each with its own target processes. |
| `--stop-after` | Stop after N executions per worker, or after a duration (`30s`, `2m`). |

## A small example

The shortest target is base64 encoding. A Python target is just a function plus one harness call:

```python
# examples/base64/python/python_target.py
import base64
import crossfuzz

def encode_base64(data: bytes) -> bytes:
    return base64.b64encode(data)

crossfuzz.fuzz(encode_base64)
```

A minimal `crossfuzz.toml` to fuzz two implementations against each other:

```toml
[campaign]
name = "base64_diff"
timeout = "30m"
max_input_size = 1024

[corpus]
seed_dir     = "./seeds"
corpus_dir   = "./corpus"
findings_dir = "./findings"

[[target]]
name     = "python_base64"
language = "python"
binary   = "../../harness/python/.venv/bin/python3"
args     = ["./python/python_target.py"]

[[target]]
name      = "c_base64"
language  = "c"
binary    = "./c/c_target"
build_cmd = "cd c && clang -fsanitize-coverage=trace-pc-guard -O2 -I ../../../harness/c -o c_target c_target.c ../../../harness/c/crossfuzz.c"

[comparator]
type = "byte_equal"
```

Run it:

```bash
cd examples/base64
crossfuzz build crossfuzz.toml
crossfuzz run   crossfuzz.toml
```

Interesting inputs are written to `corpus/`; any input that produces disagreeing outputs is saved to `findings/`.

## Real examples

See [`examples/`](./examples/README.md) for working end-to-end campaigns (base64, JSON parsers, URL parsers, HTTP servers).

## Further reading

[`ARCHITECTURE.md`](./ARCHITECTURE.md) covers the internals: the AFL-style coverage bitmap, the pipe + shared-memory IPC layout, mutation strategies, and how each language harness plugs into the coordinator.
