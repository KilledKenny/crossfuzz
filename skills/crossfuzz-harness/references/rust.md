# Rust Harness

## Harness crate

Add to your binary's `Cargo.toml`:

```toml
[dependencies]
crossfuzz = "0.0.1"
```

## Fuzz target

```rust
fn main() {
    crossfuzz::fuzz(
        |input| {
            // Return Err(...) on error — does not crash the process.
            Ok(input.to_vec())
        },
        Default::default(),
    );
}
```

### Target closure signature

```rust
Fn(&[u8]) -> Result<Vec<u8>, Box<dyn std::error::Error>>
```

## Build command

```bash
cargo rustc --release -- \
  -C passes=sancov-module \
  -C llvm-args=-sanitizer-coverage-level=3 \
  -C llvm-args=-sanitizer-coverage-trace-pc-guard
```

Use `cargo rustc` (not `cargo build`) so the instrumentation flags only apply to the final binary crate, not to dependency build scripts — which would fail to link the coverage callbacks.

These flags are **required**. Without them the binary runs but produces no coverage signal.

## TOML config entry

```toml
[[target]]
name = "rust_impl"
language = "rust"
binary = "./rust_target/target/release/rust_impl"
build_cmd = "cd rust_target && cargo rustc --release -- -C passes=sancov-module -C llvm-args=-sanitizer-coverage-level=3 -C llvm-args=-sanitizer-coverage-trace-pc-guard"
```

## How coverage works

The harness implements `__sanitizer_cov_trace_pc_guard_init` and `__sanitizer_cov_trace_pc_guard` as `#[no_mangle] extern "C"` symbols. LLVM's SanitizerCoverage module pass (enabled via the RUSTFLAGS above) calls these on every covered edge. The callbacks write saturating counters directly into the 64 KB bitmap in shared memory — the same mechanism used by the C harness.

The coordinator resets the bitmap before each fuzz command, so the harness does not need to clear it.

### Verifying coverage symbols

```bash
nm target/release/my_target | grep sanitizer_cov
```

Should show both `__sanitizer_cov_trace_pc_guard_init` and `__sanitizer_cov_trace_pc_guard`.

## Settings

```rust
pub struct Settings {
    /// Enable SanitizerCoverage collection. Default: true.
    pub instrument: bool,
    /// Filter mode only: when true, the filter's returned bytes replace the
    /// original input for all downstream targets. Default: false.
    pub transform: bool,
}
```

`Default::default()` gives `{ instrument: true, transform: false }`.

## Filter

```rust
pub fn filter<F>(target: F, settings: Settings)
where
    F: Fn(&[u8]) -> (Vec<u8>, bool),
```

Returns `(output_bytes, accepted)`. When `accepted` is `false` the coordinator discards the input. When `true`:
- `settings.transform = false` (default): original input is forwarded unchanged.
- `settings.transform = true`: returned `output_bytes` replace the input.

**Validity filter:**

```rust
use crossfuzz::{filter, Settings};

fn main() {
    filter(
        |input| {
            let accepted = std::str::from_utf8(input).is_ok();
            (Vec::new(), accepted)
        },
        Default::default(),
    );
}
```

**Transform filter (normalise JSON):**

```rust
use crossfuzz::{filter, Settings};

fn main() {
    filter(
        |input| {
            match serde_json::from_slice::<serde_json::Value>(input) {
                Ok(v) => {
                    let normalised = serde_json::to_vec(&v).unwrap_or_default();
                    (normalised, true)
                }
                Err(_) => (Vec::new(), false),
            }
        },
        Settings { transform: true, ..Default::default() },
    );
}
```

## Compare

```rust
pub fn compare<F>(target: F, settings: Settings)
where
    F: Fn(&[u8], &[String], &[&[u8]]) -> String,
```

Parameters: `(input, names, outputs)`. Return `""` if all outputs agree, or a non-empty mismatch description.

```rust
use crossfuzz::{compare, Settings};

fn main() {
    compare(
        |input, names, outputs| {
            let first = &outputs[0];
            for (i, out) in outputs.iter().enumerate().skip(1) {
                if out != first {
                    return format!("{} vs {}: {:?} != {:?}", names[0], names[i], first, out);
                }
            }
            String::new()
        },
        Default::default(),
    );
}
```

Configure as `[comparator] type = "harness"`.

## Disabling instrumentation

Set `instrument: false` when the harness is a thin HTTP client and coverage comes from an instrumented server:

```rust
crossfuzz::fuzz(
    |input| { Ok(vec![]) },
    crossfuzz::Settings { instrument: false, ..Default::default() },
);
```
