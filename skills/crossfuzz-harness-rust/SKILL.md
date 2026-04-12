---
name: crossfuzz-harness-rust
description: Use this skill when the user is writing a Rust target for cross_fuzz, needs to know the Rust harness API, wants to know which RUSTFLAGS are required for coverage, or is setting up a Rust fuzzing target. Trigger for questions like "how do I write a Rust target?", "what RUSTFLAGS do I need?", "how do I add Rust to cross_fuzz?", "my Rust target isn't producing coverage", or "how do I build a Rust harness?".
---

# Rust Harness

## Harness crate

`harness/rust/` — add as a path dependency in your binary's `Cargo.toml`:

```toml
[dependencies]
crossfuzz-harness = { path = "../../harness/rust" }
```

## Fuzz target

Implement a closure and pass it to `crossfuzz_harness::fuzz`.

```rust
fn main() {
    crossfuzz_harness::fuzz(
        |input| {
            // Process input bytes, return output bytes.
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

## Build command (required RUSTFLAGS)

```bash
cargo rustc --release -- \
  -C passes=sancov-module \
  -C llvm-args=-sanitizer-coverage-level=3 \
  -C llvm-args=-sanitizer-coverage-trace-pc-guard
```

Using `cargo rustc` (not `cargo build`) passes the instrumentation flags only to the final binary crate, not to dependency build scripts — which would fail to link the coverage callbacks.

These flags are **required** for coverage. Without them the binary runs correctly but produces no coverage signal and the campaign degrades to blind fuzzing.

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

## Verifying coverage symbols

```bash
nm target/release/my_target | grep sanitizer_cov
```

Should show both `__sanitizer_cov_trace_pc_guard_init` and `__sanitizer_cov_trace_pc_guard`.

For Settings, filter, and compare variants with annotated examples, read `<skill-dir>/references/rust-harness.md`.
