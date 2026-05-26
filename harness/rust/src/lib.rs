//! crossfuzz Rust harness.
//!
//! Handles IPC between the crossfuzz coordinator and your Rust target. You
//! write the target function; the harness manages shared memory, pipes, and
//! coverage collection.
//!
//! # Coverage
//!
//! Compile your binary with SanitizerCoverage instrumentation:
//!
//! ```bash
//! RUSTFLAGS="-C passes=sancov-module \
//!            -C llvm-args=-sanitizer-coverage-level=3 \
//!            -C llvm-args=-sanitizer-coverage-trace-pc-guard" \
//!   cargo build --release
//! ```
//!
//! Without these flags the binary runs but produces no coverage signal.

mod coverage;
mod proto;
mod shm;

pub use shm::{ReadOnlyShm, SharedMem};

const CMD_FD: i32 = 3; // coordinator → harness
const RESP_FD: i32 = 4; // harness → coordinator

/// Harness configuration. All fields have safe defaults via [`Default`].
pub struct Settings {
    /// Enable coverage collection via SanitizerCoverage. Default: `true`.
    pub instrument: bool,
    /// Run the target this many extra times before the main loop on the first
    /// input to pre-heat instruction caches. Default: `0` (no warmup).
    ///
    /// Unlike the Go harness, SanitizerCoverage writes directly into the bitmap
    /// each iteration; the coordinator resets the bitmap before every fuzz
    /// command, so noise masking is not required.
    pub warmup: u32,
    /// Filter mode only: when `true`, the bytes returned by the filter replace
    /// the original input for downstream targets. Default: `false`.
    pub transform: bool,
}

impl Default for Settings {
    fn default() -> Self {
        Self { instrument: true, warmup: 0, transform: false }
    }
}

// ---- helpers ----------------------------------------------------------------

fn escape_json_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c => out.push(c),
        }
    }
    out
}

fn die(msg: &str) -> ! {
    eprintln!("crossfuzz: {msg}");
    std::process::exit(1);
}

// ---- entry points -----------------------------------------------------------

/// Enter the persistent fuzzing loop.
///
/// `target` receives the raw input bytes and returns output bytes or an error.
/// An error result sets `status = error` and `ok = false` in the response;
/// the coordinator records the input but does not treat it as a crash.
///
/// The loop exits cleanly on `shutdown` or when the command pipe closes.
pub fn fuzz<F>(target: F, settings: Settings)
where
    F: Fn(&[u8]) -> Result<Vec<u8>, Box<dyn std::error::Error>>,
{
    let shm = SharedMem::open_from_env().unwrap_or_else(|e| die(&e));

    if settings.instrument {
        coverage::set_bitmap(shm.coverage_ptr());
    }

    if !proto::write_msg(RESP_FD, b"{\"type\":\"ready\"}") {
        die("send ready");
    }

    loop {
        let raw = match proto::read_msg(CMD_FD) {
            Some(b) => b,
            None => return,
        };

        let msg: serde_json::Value = match serde_json::from_slice(&raw) {
            Ok(v) => v,
            Err(_) => continue,
        };

        match msg["type"].as_str() {
            Some("shutdown") => return,

            Some("fuzz") => {
                let input = shm.read_input();

                // Optional warmup: pre-heat without recording results.
                if settings.warmup > 0 {
                    for _ in 0..settings.warmup {
                        let _ = target(&input);
                    }
                }

                match target(&input) {
                    Ok(output) => {
                        shm.write_output(&output);
                        shm.set_status(shm::STATUS_OK);
                        proto::write_msg(RESP_FD, b"{\"type\":\"fuzz_result\",\"ok\":true}");
                    }
                    Err(e) => {
                        shm.set_output_len(0);
                        shm.set_status(shm::STATUS_ERROR);
                        let resp = format!(
                            "{{\"type\":\"fuzz_result\",\"error\":\"{}\"}}",
                            escape_json_string(&e.to_string())
                        );
                        proto::write_msg(RESP_FD, resp.as_bytes());
                    }
                }
            }

            Some("ping") => {
                proto::write_msg(RESP_FD, b"{\"type\":\"pong\"}");
            }

            _ => {} // ignore unknown message types
        }
    }
}

/// Enter the persistent filter loop.
///
/// `target` receives the raw input bytes and returns `(output, accepted)`.
/// When `accepted` is `false` the input is discarded by the coordinator.
/// When `accepted` is `true` and `settings.transform` is `true`, the returned
/// `output` bytes replace the original input for all targets; otherwise the
/// original input is forwarded unchanged.
pub fn filter<F>(target: F, settings: Settings)
where
    F: Fn(&[u8]) -> (Vec<u8>, bool),
{
    let shm = SharedMem::open_from_env().unwrap_or_else(|e| die(&e));

    if !proto::write_msg(RESP_FD, b"{\"type\":\"ready\"}") {
        die("send ready");
    }

    loop {
        let raw = match proto::read_msg(CMD_FD) {
            Some(b) => b,
            None => return,
        };

        let msg: serde_json::Value = match serde_json::from_slice(&raw) {
            Ok(v) => v,
            Err(_) => continue,
        };

        match msg["type"].as_str() {
            Some("shutdown") => return,

            Some("filter") => {
                let input = shm.read_input();
                let (output, accepted) = target(&input);

                if accepted {
                    if settings.transform && !output.is_empty() {
                        shm.write_output(&output);
                    } else {
                        shm.write_output(&input);
                    }
                    proto::write_msg(RESP_FD, b"{\"type\":\"filter_result\",\"ok\":true}");
                } else {
                    shm.set_output_len(0);
                    proto::write_msg(RESP_FD, b"{\"type\":\"filter_result\"}");
                }
            }

            Some("ping") => {
                proto::write_msg(RESP_FD, b"{\"type\":\"pong\"}");
            }

            _ => {}
        }
    }
}

/// Enter the persistent comparator loop.
///
/// `target` receives the fuzz input, an ordered list of target names, and
/// the corresponding output slices. It returns `""` if all outputs agree, or
/// a non-empty string describing the mismatch.
///
/// The compare harness does not use `CROSSFUZZ_SHM`. Instead it opens each
/// target's SHM read-only via `CROSSFUZZ_SHM_TARGETS` (a JSON map of
/// `{"name": "/path/to/shm", ...}`).
pub fn compare<F>(target: F, _settings: Settings)
where
    F: Fn(&[u8], &[String], &[&[u8]]) -> String,
{
    let targets_json = std::env::var("CROSSFUZZ_SHM_TARGETS").unwrap_or_else(|_| {
        die("CROSSFUZZ_SHM_TARGETS not set");
    });

    let target_paths: std::collections::HashMap<String, String> =
        serde_json::from_str(&targets_json).unwrap_or_else(|e| {
            die(&format!("parse CROSSFUZZ_SHM_TARGETS: {e}"));
        });

    // mmap each target's SHM read-only.
    let mut target_shms: std::collections::HashMap<String, shm::ReadOnlyShm> =
        std::collections::HashMap::new();
    for (name, path) in &target_paths {
        match shm::ReadOnlyShm::open(path) {
            Ok(s) => { target_shms.insert(name.clone(), s); }
            Err(e) => die(&format!("open target SHM {name} ({path}): {e}")),
        }
    }

    if !proto::write_msg(RESP_FD, b"{\"type\":\"ready\"}") {
        die("send ready");
    }

    loop {
        let raw = match proto::read_msg(CMD_FD) {
            Some(b) => b,
            None => return,
        };

        let msg: serde_json::Value = match serde_json::from_slice(&raw) {
            Ok(v) => v,
            Err(_) => continue,
        };

        match msg["type"].as_str() {
            Some("shutdown") => return,

            Some("compare") => {
                // "targets" is an ordered JSON array of target names.
                let names: Vec<String> = msg["targets"]
                    .as_array()
                    .map(|a| {
                        a.iter()
                            .filter_map(|v| v.as_str().map(|s| s.to_string()))
                            .collect()
                    })
                    .unwrap_or_default();

                let mut input: Option<Vec<u8>> = None;
                let mut outputs: Vec<Vec<u8>> = Vec::with_capacity(names.len());

                for name in &names {
                    if let Some(ts) = target_shms.get(name) {
                        if input.is_none() {
                            input = Some(ts.read_input());
                        }
                        outputs.push(ts.read_output());
                    } else {
                        outputs.push(Vec::new());
                    }
                }

                let input_ref = input.as_deref().unwrap_or(&[]);
                let output_refs: Vec<&[u8]> = outputs.iter().map(|v| v.as_slice()).collect();
                let mismatch = target(input_ref, &names, &output_refs);

                if mismatch.is_empty() {
                    proto::write_msg(RESP_FD, b"{\"type\":\"compare_result\"}");
                } else {
                    let resp = format!(
                        "{{\"type\":\"compare_result\",\"error\":\"{}\"}}",
                        escape_json_string(&mismatch)
                    );
                    proto::write_msg(RESP_FD, resp.as_bytes());
                }
            }

            Some("ping") => {
                proto::write_msg(RESP_FD, b"{\"type\":\"pong\"}");
            }

            _ => {}
        }
    }
}
