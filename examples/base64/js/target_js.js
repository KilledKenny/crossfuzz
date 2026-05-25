/**
 * JavaScript/TypeScript base64 target for cross_fuzz.
 *
 * Run without coverage (no feedback, still finds byte-level discrepancies):
 *   bun run ./target.ts
 *
 * Run with Istanbul coverage instrumentation (recommended):
 *   bun --preload @crossfuzz/crossfuzz/instrument.ts ./target.ts
 */

import { fuzz } from "@crossfuzz/crossfuzz";

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

function base64Encode(input) {
  const outLen = 4 * Math.floor((input.length + 2) / 3);
  const out = new Uint8Array(outLen);

  let i = 0;
  let j = 0;
  while (i < input.length) {
    const a = input[i++];
    const b = i < input.length ? input[i++] : 0;
    const c = i < input.length ? input[i++] : 0;
    const triple = (a << 16) | (b << 8) | c;

    out[j++] = ALPHABET.charCodeAt((triple >>> 18) & 0x3f);
    out[j++] = ALPHABET.charCodeAt((triple >>> 12) & 0x3f);
    out[j++] = ALPHABET.charCodeAt((triple >>> 6)  & 0x3f);
    out[j++] = ALPHABET.charCodeAt( triple         & 0x3f);
  }

  const mod = input.length % 3;
  if (mod > 0) {
    out[outLen - 1] = 0x3d; // '='
    if (mod === 1) out[outLen - 2] = 0x3d;
  }

  return out;
}

fuzz(base64Encode);
