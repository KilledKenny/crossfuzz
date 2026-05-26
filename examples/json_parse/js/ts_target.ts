/**
 * TypeScript JSON parse target for crossfuzz.
 *
 * Hand-rolled recursive-descent parser — no built-in JSON parser used.
 * Returns the root value type as ASCII bytes:
 *   object / array / string / number / true / false / null / error
 *
 * Run with Istanbul coverage instrumentation (recommended):
 *   bun --preload @crossfuzz/crossfuzz/instrument.ts ./ts_target.ts
 */

import { fuzz } from "@crossfuzz/crossfuzz";

// ---- parser state ----

let src: Uint8Array;
let pos: number;

function skipWs(): void {
  while (pos < src.length) {
    const c = src[pos];
    if (c === 0x20 || c === 0x09 || c === 0x0d || c === 0x0a) pos++;
    else break;
  }
}

function parseString(): void {
  if (pos >= src.length || src[pos] !== 0x22 /* '"' */) throw new Error("expected '\"'");
  pos++;
  while (pos < src.length) {
    const c = src[pos++];
    if (c === 0x22 /* '"' */) return;
    if (c === 0x5c /* '\\' */) {
      if (pos >= src.length) throw new Error("eof in escape");
      const esc = src[pos++];
      if (esc === 0x75 /* 'u' */) {
        for (let i = 0; i < 4; i++) {
          if (pos >= src.length) throw new Error("eof in \\u");
          const h = src[pos++];
          const isHex =
            (h >= 0x30 && h <= 0x39) || // 0-9
            (h >= 0x61 && h <= 0x66) || // a-f
            (h >= 0x41 && h <= 0x46);   // A-F
          if (!isHex) throw new Error("bad hex digit");
        }
      } else {
        const codes = Array.from("\"\\/bfnrt", ch => ch.charCodeAt(0))
        if (!codes.includes(esc)) {
          throw new Error("invalid char in escape code");
        }
      }
      // other escapes accepted as-is
    } else if (c < 0x20) {
      throw new Error("control char in string");
    }
  }
  throw new Error("unterminated string");
}

function parseNumber(): void {
  if (pos < src.length && src[pos] === 0x2d /* '-' */) pos++;
  if (pos >= src.length) throw new Error("eof in number");
  if (src[pos] === 0x30 /* '0' */) {
    pos++;
  } else {
    if (src[pos] < 0x31 || src[pos] > 0x39) throw new Error("bad number start");
    while (pos < src.length && src[pos] >= 0x30 && src[pos] <= 0x39) pos++;
  }
  if (pos < src.length && src[pos] === 0x2e /* '.' */) {
    pos++;
    if (pos >= src.length || src[pos] < 0x30 || src[pos] > 0x39) throw new Error("bad fraction");
    while (pos < src.length && src[pos] >= 0x30 && src[pos] <= 0x39) pos++;
  }
  if (pos < src.length && (src[pos] === 0x65 /* 'e' */ || src[pos] === 0x45 /* 'E' */)) {
    pos++;
    if (pos < src.length && (src[pos] === 0x2b /* '+' */ || src[pos] === 0x2d /* '-' */)) pos++;
    if (pos >= src.length || src[pos] < 0x30 || src[pos] > 0x39) throw new Error("bad exponent");
    while (pos < src.length && src[pos] >= 0x30 && src[pos] <= 0x39) pos++;
  }
}

function matchLiteral(lit: string): void {
  for (let i = 0; i < lit.length; i++) {
    if (pos >= src.length || src[pos] !== lit.charCodeAt(i))
      throw new Error("literal mismatch");
    pos++;
  }
}

function parseArray(): void {
  if (pos >= src.length || src[pos] !== 0x5b /* '[' */) throw new Error("expected '['");
  pos++;
  skipWs();
  if (pos < src.length && src[pos] === 0x5d /* ']' */) { pos++; return; }
  while (true) {
    parseValue();
    skipWs();
    if (pos >= src.length) throw new Error("eof in array");
    if (src[pos] === 0x5d /* ']' */) { pos++; return; }
    if (src[pos] !== 0x2c /* ',' */) throw new Error("expected ',' or ']'");
    pos++;
    skipWs();
  }
}

function parseObject(): void {
  if (pos >= src.length || src[pos] !== 0x7b /* '{' */) throw new Error("expected '{'");
  pos++;
  skipWs();
  if (pos < src.length && src[pos] === 0x7d /* '}' */) { pos++; return; }
  while (true) {
    skipWs();
    parseString();
    skipWs();
    if (pos >= src.length || src[pos] !== 0x3a /* ':' */) throw new Error("expected ':'");
    pos++;
    skipWs();
    parseValue();
    skipWs();
    if (pos >= src.length) throw new Error("eof in object");
    if (src[pos] === 0x7d /* '}' */) { pos++; return; }
    if (src[pos] !== 0x2c /* ',' */) throw new Error("expected ',' or '}'");
    pos++;
  }
}

function parseValue(): string {
  skipWs();
  if (pos >= src.length) throw new Error("eof");
  const c = src[pos];
  if (c === 0x22 /* '"' */) { parseString(); return "string"; }
  if (c === 0x7b /* '{' */) { parseObject(); return "object"; }
  if (c === 0x5b /* '[' */) { parseArray(); return "array"; }
  if (c === 0x74 /* 't' */) { matchLiteral("true"); return "true"; }
  if (c === 0x66 /* 'f' */) { matchLiteral("false"); return "false"; }
  if (c === 0x6e /* 'n' */) { matchLiteral("null"); return "null"; }
  if (c === 0x2d /* '-' */ || (c >= 0x30 && c <= 0x39)) { parseNumber(); return "number"; }
  throw new Error("unexpected char: " + c);
}

// ---- crossfuzz target ----

function target(input: Uint8Array): Uint8Array {
  src = input;
  pos = 0;
  let type: string;
  try {
    type = parseValue();
    skipWs();
    if (pos !== src.length) return enc("error");
    return enc(type);
  } catch {
    return enc("error");
  }
}

function enc(s: string): Uint8Array {
  return new TextEncoder().encode(s);
}

fuzz(target);
