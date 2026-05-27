# crossfuzz

Rust harness for [crossfuzz](https://github.com/KilledKenny/crossfuzz), a coverage-guided differential fuzzer. This crate handles shared memory mapping, pipe IPC with the coordinator, and SanitizerCoverage instrumentation — you only write the target function.

> **Platform:** Linux / Unix only (`shm_open`, inherited pipe file descriptors).

## Usage

Add to your `Cargo.toml`:

```toml
[dependencies]
crossfuzz = "0.1.0"
```

Compile with SanitizerCoverage instrumentation so the coordinator gets coverage signal:

```bash
RUSTFLAGS="-C passes=sancov-module \
           -C llvm-args=-sanitizer-coverage-level=3 \
           -C llvm-args=-sanitizer-coverage-trace-pc-guard" \
  cargo build --release
```

Without these flags the binary runs correctly but produces no coverage data.

## Example

```rust
fn my_function(input: &[u8]) -> Vec<u8> {
    // ... your implementation
    input.to_vec()
}

fn main() {
    crossfuzz::fuzz(|input| Ok(my_function(input)), Default::default());
}
```

For a complete working example see [`examples/base64/rust/`](https://github.com/KilledKenny/crossfuzz/tree/main/examples/base64/rust).

## Entry points

| Function | Use when |
|----------|----------|
| `crossfuzz::fuzz` | Standard fuzz target — receives input, returns output |
| `crossfuzz::filter` | Input filter / transformer before fuzzing |
| `crossfuzz::compare` | Custom comparator for output divergence checks |

## Configuration

All entry points accept a `Settings` struct (all fields have sensible defaults):

```rust
crossfuzz::fuzz(my_fn, crossfuzz::Settings {
    instrument: true,  // enable SanitizerCoverage (default: true)
    warmup: 3,         // extra pre-heat iterations before recording (default: 0)
    transform: false,  // filter mode: replace input with output (default: false)
});
```

## License

MIT
