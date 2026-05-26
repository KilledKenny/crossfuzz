# crossfuzz

Python harness for [cross_fuzz](https://github.com/KilledKenny/cross_fuzz), a
coverage-guided **differential fuzzer** across C, C++, Go, Java,
JavaScript/TypeScript, Python, and Rust. The coordinator sends the same
generated input to multiple implementations of the same function, collects
coverage from every target, and flags any divergence in outputs.

This package is the Python side: it handles the shared-memory IPC, pipe
protocol, and branch-arc coverage collection so you only have to write the
target function.

## Install

```bash
pip install crossfuzz
```

You also need the `crossfuzz` coordinator binary — see the
[main repo](https://github.com/KilledKenny/cross_fuzz) for build/install instructions.

## Quick start

Write a target script that calls `crossfuzz.fuzz(your_function)`:

```python
import crossfuzz

def my_target(data: bytes) -> bytes:
    # Your code under test. Raise an exception to signal an error.
    return data

crossfuzz.fuzz(my_target)
```

Point the coordinator at it from a `crossfuzz.toml` config:

```toml
[[target]]
name = "python_impl"
language = "python"
binary = "python3"
args = ["my_target.py"]
```

Then run the campaign with `crossfuzz run crossfuzz.toml`.

## API

- `crossfuzz.fuzz(target, settings=None)` — persistent fuzz loop.
  `target(data: bytes) -> bytes`.
- `crossfuzz.filter(target, settings=None)` — persistent filter loop.
  `target(data: bytes) -> tuple[bytes, bool]`.
- `crossfuzz.compare(target, settings=None)` — custom comparator loop.
  `target(input: bytes, names: list[str], outputs: list[bytes]) -> str`.
- `crossfuzz.Settings(instrument=True, warmup=0, transform=False)` —
  configuration. Set `instrument=False` for thin HTTP-trigger harnesses
  where coverage comes from an instrumented server process.

## Platform

POSIX only — the harness uses `mmap` and inherited file descriptors 3/4 to
talk to the coordinator. Windows is not supported.

## License

MIT
