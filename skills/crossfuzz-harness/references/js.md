# JavaScript / TypeScript Harness

The JS harness runs under **Bun** (not Node). Coverage uses Istanbul AST instrumentation via a preload script.

## Setup

```bash
cd harness/js && bun install
```

Installs Istanbul and Bun dependencies. Only needed once (or after updating the repo).

## Fuzz target

```typescript
import { fuzz } from "../../harness/js/crossfuzz";

fuzz((input: Uint8Array): Uint8Array => {
    // Throw to mark execution as error-status.
    return input;
});
```

### Function signature

```typescript
export type FuzzFn = (input: Uint8Array) => Uint8Array | Buffer | Promise<Uint8Array> | Promise<Buffer>;

export async function fuzz(target: FuzzFn, settings?: Settings): Promise<void>
```

## Running with coverage

```bash
bun run --preload ../../harness/js/instrument.ts ./target.ts
```

`--preload instrument.ts` applies Istanbul AST instrumentation at load time so all your source files are coverage-instrumented. Without it the binary runs but produces no coverage signal.

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

## Settings

```typescript
import { fuzz, Settings } from "../../harness/js/crossfuzz";

fuzz(myTarget, {
    instrument: true,    // default: true — set false when harness is thin HTTP client
    transform: false,    // filter mode: returned bytes replace input
});
```

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | `true` | Collect and report Istanbul coverage |
| `transform` | `false` | Filter mode: when true, returned bytes replace the original input |

## Filter target

```typescript
import { filter, FilterResult } from "../../harness/js/crossfuzz";

filter((input: Uint8Array): FilterResult => {
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

Configure as `[input_filter]`.

## Compare target

```typescript
import { compare } from "../../harness/js/crossfuzz";

compare((input: Uint8Array, names: string[], outputs: Uint8Array[]): string => {
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

Configure as `[comparator] type = "harness"`.

## Server mode (standalone functions)

```typescript
import { openShm, clearInstrumentation, collectInstrumentation } from "../../harness/js/crossfuzz";

const shm = openShm();
clearInstrumentation();
// ... send HTTP request ...
collectInstrumentation(shm.subarray(COVERAGE_OFFSET, COVERAGE_OFFSET + 65536));
```

## Async targets

`fuzz()` is `async` and awaits your target function natively:

```typescript
fuzz(async (input: Uint8Array): Promise<Uint8Array> => {
    return await someAsyncOperation(input);
});
```

## Common pitfalls

- **Missing `--preload instrument.ts`**: binary runs but produces no coverage.
- **Using Node instead of Bun**: the harness uses `Bun.mmap()`. Node is not supported.
- **Not running `bun install` in `harness/js/`**: Istanbul won't be available.
- **Output larger than 1 MB**: the harness clamps automatically.
- **Returning `null` or `undefined`**: treated as empty output, not an error. Throw to signal error.
