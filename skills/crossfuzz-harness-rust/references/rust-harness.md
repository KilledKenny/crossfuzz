# Rust Harness — Full Reference

## Settings

```rust
pub struct Settings {
    /// Enable SanitizerCoverage collection. Default: true.
    pub instrument: bool,
    /// Run the target this many extra times before the main loop on the first
    /// input to pre-heat caches. Default: 0 (no warmup).
    pub warmup: u32,
    /// Filter mode only: when true, the filter's returned bytes replace the
    /// original input for all downstream targets. Default: false.
    pub transform: bool,
}
```

`Default::default()` gives `{ instrument: true, warmup: 0, transform: false }`.

## Fuzz

```rust
pub fn fuzz<F>(target: F, settings: Settings)
where
    F: Fn(&[u8]) -> Result<Vec<u8>, Box<dyn std::error::Error>>,
```

**Example:**

```rust
use crossfuzz_harness::{fuzz, Settings};

fn my_parse(data: &[u8]) -> Result<Vec<u8>, Box<dyn std::error::Error>> {
    // ... your implementation ...
    Ok(data.to_vec())
}

fn main() {
    fuzz(my_parse, Default::default());
}
```

Return `Err(...)` to flag an error without crashing. The coordinator records
the input as an error case but continues fuzzing. A panic will kill the
process; the coordinator detects the crash via pipe EOF and restarts.

## Filter

```rust
pub fn filter<F>(target: F, settings: Settings)
where
    F: Fn(&[u8]) -> (Vec<u8>, bool),
```

The closure returns `(output_bytes, accepted)`. When `accepted` is `false`,
the coordinator discards the input. When `accepted` is `true`:
- If `settings.transform = false` (default): the original input is forwarded to targets unchanged.
- If `settings.transform = true`: the returned `output_bytes` replace the input for all targets.

**Example (validity filter):**

```rust
use crossfuzz_harness::{filter, Settings};

fn main() {
    filter(
        |input| {
            // Accept only valid UTF-8 inputs.
            let accepted = std::str::from_utf8(input).is_ok();
            (Vec::new(), accepted)
        },
        Default::default(),
    );
}
```

**Example (transform filter — normalise JSON before fuzzing):**

```rust
use crossfuzz_harness::{filter, Settings};

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

Parameters: `(input, names, outputs)` where `names[i]` is the target name and
`outputs[i]` is its output bytes. Return `""` if all outputs agree, or a
non-empty string describing the mismatch.

The compare process reads `CROSSFUZZ_SHM_TARGETS` (set by the coordinator)
instead of `CROSSFUZZ_SHM`. It mmaps each target's shared memory read-only.

**Example:**

```rust
use crossfuzz_harness::{compare, Settings};

fn main() {
    compare(
        |input, names, outputs| {
            // Fail fast on any empty output.
            let first = &outputs[0];
            for (i, out) in outputs.iter().enumerate().skip(1) {
                if out != first {
                    return format!(
                        "{} vs {}: {:?} != {:?}",
                        names[0], names[i], first, out
                    );
                }
            }
            String::new()
        },
        Default::default(),
    );
}
```

**TOML config for a custom comparator:**

```toml
[[target]]
name = "rust_compare"
language = "rust"
type = "harness"
binary = "./rust_compare/target/release/rust_compare"
build_cmd = "cd rust_compare && cargo rustc --release -- -C passes=sancov-module -C llvm-args=-sanitizer-coverage-level=3 -C llvm-args=-sanitizer-coverage-trace-pc-guard"

[comparator]
type = "custom"
binary = "./rust_compare/target/release/rust_compare"
```

## Disabling instrumentation

Set `instrument: false` when the harness is a thin HTTP client and coverage
comes entirely from an instrumented server process (`type = "server"` in the
TOML config):

```rust
crossfuzz_harness::fuzz(
    |input| { /* send HTTP request, return response body */ Ok(vec![]) },
    crossfuzz_harness::Settings { instrument: false, ..Default::default() },
);
```
