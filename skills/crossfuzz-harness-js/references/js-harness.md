# JavaScript / TypeScript Harness — Complete Reference

## Settings

```typescript
import { fuzz, Settings } from "../../harness/js/crossfuzz";

fuzz(myTarget, {
    instrument: true,    // default: true — set false when harness is thin HTTP client
    warmup: 0,           // reserved
    transform: false,    // filter mode: returned bytes replace input
    hinting: false,      // reserved
});
```

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | `true` | Collect and report Istanbul coverage |
| `warmup` | `0` | Reserved |
| `transform` | `false` | Filter mode: when true, returned bytes replace the original input |
| `hinting` | `false` | Reserved |

## Filter target

```typescript
import { filter, FilterResult } from "../../harness/js/crossfuzz";

filter((input: Uint8Array): FilterResult => {
    // Return { output, accepted: true } to accept, { output: new Uint8Array(0), accepted: false } to reject.
    // output is only used when settings.transform = true.
    if (input.length < 4) {
        return { output: new Uint8Array(0), accepted: false };
    }
    return { output: input, accepted: true };
});
```

### FilterResult type

```typescript
export interface FilterResult {
    output: Uint8Array;
    accepted: boolean;
}

export type FilterFn = (input: Uint8Array) => FilterResult | Promise<FilterResult>;
```

Configure in `crossfuzz.toml` as `[input_filter]` (not as a `[[target]]`).

## Compare target

```typescript
import { compare } from "../../harness/js/crossfuzz";

compare((input: Uint8Array, names: string[], outputs: Uint8Array[]): string => {
    // Return "" for match, non-empty string for mismatch.
    if (outputs.length < 2) return "";
    const a = Buffer.from(outputs[0]).toString();
    const b = Buffer.from(outputs[1]).toString();
    if (a !== b) return `${names[0]} returned ${JSON.stringify(a)}, ${names[1]} returned ${JSON.stringify(b)}`;
    return "";
});
```

### CompareFn type

```typescript
export type CompareFn = (input: Uint8Array, names: string[], outputs: Uint8Array[]) => string | Promise<string>;
```

Configure in `crossfuzz.toml` as `[comparator] type = "harness"`.

## Server mode (standalone functions)

```typescript
import { openShm, clearInstrumentation, collectInstrumentation } from "../../harness/js/crossfuzz";

// Call once during initialization:
const shm = openShm();  // maps CROSSFUZZ_SHM

// Before each request:
clearInstrumentation();

// ... send HTTP request to server ...

// After response:
collectInstrumentation(shm.subarray(COVERAGE_OFFSET, COVERAGE_OFFSET + 65536));
```

## Full example: base64 (TypeScript)

From `examples/base64/target_ts.ts`:

```typescript
import { fuzz } from "../../harness/js/crossfuzz";

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

function base64Encode(input: Uint8Array): Uint8Array {
    const outLen = 4 * Math.floor((input.length + 2) / 3);
    const out = new Uint8Array(outLen);
    let i = 0, j = 0;
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
    if (mod > 0) { out[outLen-1] = 0x3d; if (mod===1) out[outLen-2] = 0x3d; }
    return out;
}

fuzz(base64Encode);
```

Run with coverage:

```bash
bun run --preload ../../harness/js/instrument.ts ./target_ts.ts
```

## TOML config entry

```toml
[[target]]
name = "ts_impl"
language = "js"
binary = "bun"
args = ["run", "--preload", "../../harness/js/instrument.ts", "./target.ts"]
build_cmd = "cd ../../harness/js && bun install"
```

Both `.js` and `.ts` files use `language = "js"`.

## Async targets

`fuzz()` is `async` and awaits your target function, so async targets work natively:

```typescript
fuzz(async (input: Uint8Array): Promise<Uint8Array> => {
    const result = await someAsyncOperation(input);
    return result;
});
```

## Common pitfalls

- **Missing `--preload instrument.ts`**: binary runs but produces no coverage — the fuzzer never discovers new inputs.
- **Using Node instead of Bun**: the harness uses `Bun.mmap()` which is Bun-specific. Node is not supported.
- **Not running `bun install` in `harness/js/`**: Istanbul and other dependencies won't be available.
- **Output larger than 1 MB**: the output region is exactly 1 MB; the harness clamps automatically (`Math.min(output.length, MAX_OUTPUT)`).
- **Returning `null` or `undefined`**: the harness treats these as empty output (`new Uint8Array(0)`), not an error. Throw to signal error.
