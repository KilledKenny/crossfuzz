/**
 * cross_fuzz harness for JavaScript/TypeScript targets running under Bun.
 *
 * Usage:
 *   import { run } from "../../harness/js/crossfuzz";
 *   run((input: Uint8Array): Uint8Array => { ... });
 *
 * Shared memory is mapped via Bun.mmap(). Coverage is collected from the
 * Istanbul __coverage__ global if present (enabled by preloading instrument.ts):
 *   bun run --preload ../../harness/js/instrument.ts ./target.ts
 */

import { readSync, writeSync } from "node:fs";

// Shared memory layout — must match pkg/coverage/shmem.go
const OFF_INPUT_LEN   = 8;
const OFF_OUTPUT_LEN  = 12;
const OFF_STATUS      = 16;
const INPUT_OFFSET    = 64;
const OUTPUT_OFFSET   = 64 + 1_048_576;
const COVERAGE_OFFSET = 64 + 1_048_576 * 2;
const COVERAGE_SIZE   = 65_536;
const MAX_OUTPUT      = 1_048_576;

const STATUS_OK    = 0;
const STATUS_ERROR = 1;

export type TargetFn = (input: Uint8Array) => Uint8Array | Buffer;

// ---- shared memory helpers ----

function readU32LE(arr: Uint8Array, off: number): number {
  return (arr[off] | arr[off + 1] << 8 | arr[off + 2] << 16 | arr[off + 3] << 24) >>> 0;
}

function writeU32LE(arr: Uint8Array, off: number, v: number): void {
  arr[off]     =  v         & 0xFF;
  arr[off + 1] = (v >>> 8)  & 0xFF;
  arr[off + 2] = (v >>> 16) & 0xFF;
  arr[off + 3] = (v >>> 24) & 0xFF;
}

// ---- pipe protocol (fd 3 = cmd in, fd 4 = resp out) ----
// Wire format: 4-byte big-endian length followed by JSON payload.

function readMsg(): Uint8Array | null {
  const header = new Uint8Array(4);
  let total = 0;
  while (total < 4) {
    const n = readSync(3, header, total, 4 - total, null);
    if (n === 0) return null;
    total += n;
  }
  const len = (header[0] << 24 | header[1] << 16 | header[2] << 8 | header[3]) >>> 0;
  if (len === 0 || len > 1 << 20) return null;

  const payload = new Uint8Array(len);
  total = 0;
  while (total < len) {
    const n = readSync(3, payload, total, len - total, null);
    if (n === 0) return null;
    total += n;
  }
  return payload;
}

function writeMsg(json: string): void {
  const payload = Buffer.from(json, "utf8");
  const header = new Uint8Array(4);
  const plen = payload.length;
  header[0] = (plen >>> 24) & 0xFF;
  header[1] = (plen >>> 16) & 0xFF;
  header[2] = (plen >>> 8)  & 0xFF;
  header[3] =  plen         & 0xFF;
  writeSync(4, header);
  writeSync(4, payload);
}

// ---- Istanbul coverage ----

type IstanbulCovData = {
  s: Record<string, number>;
  f: Record<string, number>;
  b: Record<string, number[]>;
};

// Maps (fileIndex, kind, counterIndex) to a 16-bit bitmap slot.
// Mirrors the multiplicative hashing used in the Go harness.
function hashSlot(fileIdx: number, kind: number, counterIdx: number): number {
  let h = (Math.imul(fileIdx, 0x9E3779B9) >>> 0);
  h = (Math.imul(h ^ kind,        0xBF58476D) >>> 0);
  h = (Math.imul(h ^ counterIdx,  0x94D049BB) >>> 0);
  return (h ^ (h >>> 16)) & 0xFFFF;
}

function resetIstanbulCounters(): void {
  const cov = (globalThis as any).__coverage__ as Record<string, IstanbulCovData> | undefined;
  if (!cov) return;
  for (const data of Object.values(cov)) {
    for (const k of Object.keys(data.s)) data.s[k] = 0;
    for (const k of Object.keys(data.f)) data.f[k] = 0;
    for (const k of Object.keys(data.b)) data.b[k] = data.b[k].map(() => 0);
  }
}

function collectCoverage(bitmap: Uint8Array): void {
  const cov = (globalThis as any).__coverage__ as Record<string, IstanbulCovData> | undefined;
  if (!cov) return;

  bitmap.fill(0);

  let fileIdx = 0;
  for (const data of Object.values(cov)) {
    const fi = fileIdx++;

    for (const [k, v] of Object.entries(data.s) as [string, number][]) {
      if (v > 0) {
        const idx = hashSlot(fi, 0, parseInt(k));
        const val = Math.min(v, 255);
        if (val > bitmap[idx]) bitmap[idx] = val;
      }
    }

    for (const [k, v] of Object.entries(data.f) as [string, number][]) {
      if (v > 0) {
        const idx = hashSlot(fi, 1, parseInt(k));
        const val = Math.min(v, 255);
        if (val > bitmap[idx]) bitmap[idx] = val;
      }
    }

    for (const [k, branches] of Object.entries(data.b) as [string, number[]][]) {
      for (let i = 0; i < branches.length; i++) {
        const v = branches[i];
        if (v > 0) {
          const idx = hashSlot(fi, 2 + i, parseInt(k));
          const val = Math.min(v, 255);
          if (val > bitmap[idx]) bitmap[idx] = val;
        }
      }
    }
  }
}

// ---- main harness entry point ----

export function run(target: TargetFn): void {
  const shmPath = process.env.CROSSFUZZ_SHM;
  if (!shmPath) {
    process.stderr.write("crossfuzz: CROSSFUZZ_SHM not set\n");
    process.exit(1);
  }

  if (typeof (Bun as any).mmap !== "function") {
    process.stderr.write("crossfuzz: Bun.mmap not available — upgrade to Bun >= 1.0\n");
    process.exit(1);
  }

  // MAP_SHARED: writes are immediately visible to the coordinator's mmap.
  const shm: Uint8Array = (Bun as any).mmap(shmPath);

  writeMsg('{"type":"ready"}');

  for (;;) {
    const msgBytes = readMsg();
    if (!msgBytes) break;

    const msg = JSON.parse(Buffer.from(msgBytes).toString("utf8")) as { type: string };

    if (msg.type === "shutdown") break;

    if (msg.type === "fuzz") {
      const inputLen = readU32LE(shm, OFF_INPUT_LEN);
      // Copy input so target cannot inadvertently observe later SHM writes.
      const input = shm.slice(INPUT_OFFSET, INPUT_OFFSET + inputLen);

      resetIstanbulCounters();

      let output: Uint8Array | Buffer;
      let status = STATUS_OK;
      try {
        output = target(input);
        if (!output) output = new Uint8Array(0);
      } catch {
        output = new Uint8Array(0);
        status = STATUS_ERROR;
      }

      const outLen = Math.min(output.length, MAX_OUTPUT);
      const outBytes =
        output instanceof Buffer
          ? new Uint8Array(output.buffer, output.byteOffset, outLen)
          : output.subarray(0, outLen);

      writeU32LE(shm, OFF_OUTPUT_LEN, outLen);
      writeU32LE(shm, OFF_STATUS, status);
      shm.set(outBytes, OUTPUT_OFFSET);

      collectCoverage(shm.subarray(COVERAGE_OFFSET, COVERAGE_OFFSET + COVERAGE_SIZE));

      writeMsg('{"type":"fuzz_result","ok":true}');
    }
  }
}
