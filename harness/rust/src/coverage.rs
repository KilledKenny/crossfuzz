//! SanitizerCoverage trace-pc-guard callbacks.
//!
//! These symbols are called by LLVM's SanitizerCoverage instrumentation when
//! the binary is compiled with:
//!   RUSTFLAGS="-C passes=sancov-module \
//!              -C llvm-args=-sanitizer-coverage-level=3 \
//!              -C llvm-args=-sanitizer-coverage-trace-pc-guard"
//!
//! The harness writes edge counts directly into the coordinator's shared
//! memory bitmap. The coordinator resets the bitmap before each fuzz command,
//! so the harness does not need to clear it between iterations.

use std::sync::atomic::{AtomicPtr, Ordering};

/// Points into the mmap'd SHM coverage region (64 KB at offset 0x200040).
/// Set by `set_bitmap()` once the SHM is open. Null when instrumentation is
/// disabled.
static COV_BITMAP: AtomicPtr<u8> = AtomicPtr::new(std::ptr::null_mut());

const COVERAGE_SIZE: usize = 1 << 16; // 64 KB

/// Activate coverage collection by directing callbacks into `ptr`.
/// `ptr` must point to the 64 KB coverage region inside the mmap'd SHM and
/// must remain valid for the lifetime of the process.
pub fn set_bitmap(ptr: *mut u8) {
    COV_BITMAP.store(ptr, Ordering::Relaxed);
}

/// Assign sequential guard IDs to a new range of coverage guards.
/// Called once per compilation unit by the LLVM SanitizerCoverage pass.
#[no_mangle]
pub extern "C" fn __sanitizer_cov_trace_pc_guard_init(start: *mut u32, stop: *mut u32) {
    if start == stop {
        return;
    }
    unsafe {
        // If the first guard is already non-zero this range was already
        // initialised (e.g. the init function was called twice). Skip.
        if *start != 0 {
            return;
        }
        let len = stop.offset_from(start) as usize;
        for i in 0..len {
            *start.add(i) = (i + 1) as u32;
        }
    }
}

/// Increment the bitmap counter for the edge identified by `guard`.
/// Called on every covered edge during execution.
#[no_mangle]
pub extern "C" fn __sanitizer_cov_trace_pc_guard(guard: *mut u32) {
    let bmp = COV_BITMAP.load(Ordering::Relaxed);
    if bmp.is_null() {
        return;
    }
    unsafe {
        let g = *guard;
        if g == 0 {
            return;
        }
        let slot = bmp.add((g as usize) % COVERAGE_SIZE);
        let v = slot.read();
        if v < 255 {
            slot.write(v + 1);
        }
    }
}
