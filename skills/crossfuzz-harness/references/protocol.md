# IPC Protocol Reference

## Overview

Each target process communicates with the coordinator via two mechanisms:
- **Pipes**: control messages (start, stop, result)
- **Shared memory**: bulk data (input, output, coverage bitmap)

## Pipes

- **fd 3** — coordinator → worker (command pipe)
- **fd 4** — worker → coordinator (response pipe)
- stdin/stdout are left free for target logging

All messages are **length-prefixed JSON**:
```
[4 bytes: big-endian uint32 payload length][N bytes: UTF-8 JSON]
```

## Message types

### Coordinator → Worker

```json
{"type": "ping"}
{"type": "fuzz", "timeout_ms": 500}
{"type": "filter"}
{"type": "compare", "targets": ["go_impl", "java_impl"]}
{"type": "shutdown"}
```

### Worker → Coordinator

```json
{"type": "ready"}
{"type": "pong"}
{"type": "fuzz_result", "ok": true}
{"type": "fuzz_result", "ok": false, "error": "panic: index out of range"}
{"type": "filter_result", "ok": true}
{"type": "filter_result", "ok": false}
{"type": "compare_result"}
{"type": "compare_result", "error": "go returned 'object', java returned 'array'"}
```

## Startup sequence

```
Coordinator starts target process
Target opens SHM, opens pipes
Target → Coordinator:  {"type":"ready"}
[Main loop begins]
Coordinator writes input to SHM input region
Coordinator → Target:  {"type":"fuzz"}
Target executes, writes output + coverage to SHM
Target → Coordinator:  {"type":"fuzz_result","ok":true}
[Repeat]
Coordinator → Target:  {"type":"shutdown"}
Target exits cleanly
```

## Shared memory layout

Each target gets one region (total ~2 MB + 64 KB):

```
Offset      Size       Field
────────────────────────────────────────────
0x000000    8 bytes    exec_count (uint64 LE)
0x000008    4 bytes    input_len  (uint32 LE)
0x00000C    4 bytes    output_len (uint32 LE)
0x000010    4 bytes    status     (uint32 LE: 0=ok, 1=error, 2=crash)
0x000014    44 bytes   reserved / padding to 64-byte header
0x000040    1 MB       input region  (coordinator writes before sending fuzz)
0x100040    1 MB       output region (target writes before responding)
0x200040    64 KB      coverage bitmap (target writes after executing)
────────────────────────────────────────────
Total: 0x210040 bytes ≈ 2.06 MB
```

`CROSSFUZZ_SHM` is set to the file path of the mmap'd region before the target starts.

## Coverage bitmap

A 64 KB (65,536-byte) array of saturating 8-bit counters following the AFL model:

- Each byte tracks one coverage edge (control-flow transition between basic blocks)
- Counters are **saturating** — they stop at 255
- Before merging, counters are **bucketized** to powers of two: `{1, 2, 4, 8, 16, 32, 64, 128}`
- An input is **interesting** if it sets any slot where the global bitmap is 0, or moves a counter to a higher bucket

The global bitmap is the bitwise OR of all per-iteration bitmaps across all targets. New coverage in *any* target adds the input to the corpus.

## Comparator harness env var

When `[comparator] type = "harness"`, the comparator process also receives:

```
CROSSFUZZ_SHM_TARGETS={"go_impl":"/dev/shm/crossfuzz-go_impl-...","java_impl":"/dev/shm/..."}
```

The comparator maps each target's SHM read-only and reads outputs directly from shared memory.
