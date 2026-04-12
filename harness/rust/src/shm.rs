//! Shared memory layout and accessors.
//!
//! Layout (must match pkg/coverage/shmem.go exactly):
//!
//! ```text
//! 0x000000  8B   exec_count (unused by harness)
//! 0x000008  4B   input_len  (u32 LE, coordinator writes)
//! 0x00000C  4B   output_len (u32 LE, harness writes)
//! 0x000010  4B   status     (u32 LE, harness writes: 0=ok, 1=error)
//! 0x000040  1MB  input region
//! 0x100040  1MB  output region
//! 0x200040  64KB coverage bitmap
//! Total: 0x210040 bytes
//! ```

use std::ffi::CString;
use libc::{self, c_void, MAP_FAILED, MAP_SHARED, O_RDONLY, O_RDWR, PROT_READ, PROT_WRITE};

const OFF_INPUT_LEN: usize = 8;
const OFF_OUTPUT_LEN: usize = 12;
const OFF_STATUS: usize = 16;

const INPUT_OFFSET: usize = 0x000040;
const INPUT_SIZE: usize = 1 << 20; // 1 MB
const OUTPUT_OFFSET: usize = 0x100040;
const OUTPUT_SIZE: usize = 1 << 20; // 1 MB
pub const COVERAGE_OFFSET: usize = 0x200040;
pub const TOTAL_SHM_SIZE: usize = 0x210040;

pub const STATUS_OK: u32 = 0;
pub const STATUS_ERROR: u32 = 1;

fn read_u32_le(base: *const u8, offset: usize) -> u32 {
    let mut v = [0u8; 4];
    unsafe { std::ptr::copy_nonoverlapping(base.add(offset), v.as_mut_ptr(), 4) }
    u32::from_le_bytes(v)
}

fn write_u32_le(base: *mut u8, offset: usize, val: u32) {
    let v = val.to_le_bytes();
    unsafe { std::ptr::copy_nonoverlapping(v.as_ptr(), base.add(offset), 4) }
}

/// Mmap-backed shared memory region for fuzz/filter targets.
/// Opened read-write from `CROSSFUZZ_SHM`.
pub struct SharedMem {
    base: *mut u8,
}

// The harness runs single-threaded; raw pointer access is safe within that model.
unsafe impl Send for SharedMem {}

impl SharedMem {
    /// Open and mmap the SHM file path from the `CROSSFUZZ_SHM` env var.
    pub fn open_from_env() -> Result<Self, String> {
        let path = std::env::var("CROSSFUZZ_SHM")
            .map_err(|_| "CROSSFUZZ_SHM not set".to_string())?;
        Self::open(&path)
    }

    fn open(path: &str) -> Result<Self, String> {
        let cpath = CString::new(path).map_err(|e| e.to_string())?;
        let fd = unsafe { libc::open(cpath.as_ptr(), O_RDWR) };
        if fd < 0 {
            return Err(format!(
                "open {path}: errno {}",
                unsafe { *libc::__errno_location() }
            ));
        }
        let base = unsafe {
            libc::mmap(
                std::ptr::null_mut(),
                TOTAL_SHM_SIZE,
                PROT_READ | PROT_WRITE,
                MAP_SHARED,
                fd,
                0,
            )
        };
        unsafe { libc::close(fd) };
        if base == MAP_FAILED {
            return Err(format!("mmap {path} failed"));
        }
        Ok(Self { base: base as *mut u8 })
    }

    /// Read the current input bytes (owned copy).
    pub fn read_input(&self) -> Vec<u8> {
        let n = (read_u32_le(self.base, OFF_INPUT_LEN) as usize).min(INPUT_SIZE);
        let mut v = vec![0u8; n];
        unsafe { std::ptr::copy_nonoverlapping(self.base.add(INPUT_OFFSET), v.as_mut_ptr(), n) }
        v
    }

    /// Write output bytes into the output region and set output_len.
    pub fn write_output(&self, data: &[u8]) {
        let n = data.len().min(OUTPUT_SIZE);
        unsafe { std::ptr::copy_nonoverlapping(data.as_ptr(), self.base.add(OUTPUT_OFFSET), n) }
        write_u32_le(self.base, OFF_OUTPUT_LEN, n as u32);
    }

    /// Set output_len without writing data (used to signal empty output).
    pub fn set_output_len(&self, n: u32) {
        write_u32_le(self.base, OFF_OUTPUT_LEN, n);
    }

    /// Write the execution status code.
    pub fn set_status(&self, status: u32) {
        write_u32_le(self.base, OFF_STATUS, status);
    }

    /// Return a pointer to the 64 KB coverage bitmap region.
    pub fn coverage_ptr(&self) -> *mut u8 {
        unsafe { self.base.add(COVERAGE_OFFSET) }
    }
}

impl Drop for SharedMem {
    fn drop(&mut self) {
        if !self.base.is_null() {
            unsafe { libc::munmap(self.base as *mut c_void, TOTAL_SHM_SIZE) };
            self.base = std::ptr::null_mut();
        }
    }
}

/// Read-only mmap of a target's SHM, used by the compare harness.
pub struct ReadOnlyShm {
    base: *const u8,
}

unsafe impl Send for ReadOnlyShm {}

impl ReadOnlyShm {
    /// Open and mmap the file at `path` read-only.
    pub fn open(path: &str) -> Result<Self, String> {
        let cpath = CString::new(path).map_err(|e| e.to_string())?;
        let fd = unsafe { libc::open(cpath.as_ptr(), O_RDONLY) };
        if fd < 0 {
            return Err(format!(
                "open {path}: errno {}",
                unsafe { *libc::__errno_location() }
            ));
        }
        let base = unsafe {
            libc::mmap(
                std::ptr::null_mut(),
                TOTAL_SHM_SIZE,
                PROT_READ,
                MAP_SHARED,
                fd,
                0,
            )
        };
        unsafe { libc::close(fd) };
        if base == MAP_FAILED {
            return Err(format!("mmap {path} failed"));
        }
        Ok(Self { base: base as *const u8 })
    }

    /// Read the current input bytes (owned copy).
    pub fn read_input(&self) -> Vec<u8> {
        let n = (read_u32_le(self.base, OFF_INPUT_LEN) as usize).min(INPUT_SIZE);
        let mut v = vec![0u8; n];
        unsafe { std::ptr::copy_nonoverlapping(self.base.add(INPUT_OFFSET), v.as_mut_ptr(), n) }
        v
    }

    /// Read the current output bytes (owned copy).
    pub fn read_output(&self) -> Vec<u8> {
        let n = (read_u32_le(self.base, OFF_OUTPUT_LEN) as usize).min(OUTPUT_SIZE);
        let mut v = vec![0u8; n];
        unsafe { std::ptr::copy_nonoverlapping(self.base.add(OUTPUT_OFFSET), v.as_mut_ptr(), n) }
        v
    }
}

impl Drop for ReadOnlyShm {
    fn drop(&mut self) {
        if !self.base.is_null() {
            unsafe { libc::munmap(self.base as *mut c_void, TOTAL_SHM_SIZE) };
            self.base = std::ptr::null_mut();
        }
    }
}
