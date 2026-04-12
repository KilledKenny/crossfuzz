//! Length-prefixed JSON protocol over inherited pipe fds.
//!
//! Wire format: [4 bytes big-endian u32 length][N bytes UTF-8 JSON]
//!
//! fd 3: coordinator → harness (command pipe, read-only)
//! fd 4: harness → coordinator (response pipe, write-only)

/// Read a length-prefixed JSON message from `fd`.
/// Returns `None` on EOF or I/O error.
pub fn read_msg(fd: i32) -> Option<Vec<u8>> {
    let mut hdr = [0u8; 4];
    read_exact(fd, &mut hdr)?;
    let len = u32::from_be_bytes(hdr) as usize;
    if len == 0 || len > (1 << 20) {
        return None;
    }
    let mut buf = vec![0u8; len];
    read_exact(fd, &mut buf)?;
    Some(buf)
}

/// Write a length-prefixed JSON message to `fd`.
/// Returns `false` on I/O error.
pub fn write_msg(fd: i32, json: &[u8]) -> bool {
    let hdr = (json.len() as u32).to_be_bytes();
    write_exact(fd, &hdr) && write_exact(fd, json)
}

fn read_exact(fd: i32, buf: &mut [u8]) -> Option<()> {
    let mut done = 0;
    while done < buf.len() {
        let n = unsafe {
            libc::read(
                fd,
                buf.as_mut_ptr().add(done) as *mut libc::c_void,
                buf.len() - done,
            )
        };
        if n <= 0 {
            return None;
        }
        done += n as usize;
    }
    Some(())
}

fn write_exact(fd: i32, buf: &[u8]) -> bool {
    let mut done = 0;
    while done < buf.len() {
        let n = unsafe {
            libc::write(
                fd,
                buf.as_ptr().add(done) as *const libc::c_void,
                buf.len() - done,
            )
        };
        if n <= 0 {
            return false;
        }
        done += n as usize;
    }
    true
}
