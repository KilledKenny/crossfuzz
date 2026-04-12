---
name: crossfuzz-harness-js
description: Use this skill when the user is writing a JavaScript or TypeScript target for cross_fuzz, needs to know the JS harness API, wants to know how to set up Bun and the preload flag for coverage, or is setting up a JS/TS fuzzing target. Trigger for questions like "how do I write a JavaScript target?", "how do I write a TypeScript target?", "how do I use the JS harness?", "what is the preload flag for?", "how do I install the JS harness?", "how do I use fuzz() in Bun?", or "how do I write a JS filter or comparator?".
---

# JavaScript / TypeScript Harness

The JS harness runs under **Bun** (not Node). Coverage uses Istanbul AST instrumentation via a preload script.

## Setup

```bash
cd harness/js && bun install
```

This installs Istanbul and Bun dependencies. Only needed once (or after updating the repo).

## Fuzz target (TypeScript)

```typescript
import { fuzz } from "../../harness/js/crossfuzz";

fuzz((input: Uint8Array): Uint8Array => {
    // Process input, return output.
    // Throw to mark execution as error-status.
    return input;
});
```

## Fuzz target (JavaScript)

```javascript
import { fuzz } from "../../harness/js/crossfuzz";

fuzz((input) => {
    return input;
});
```

### Function signature

```typescript
export type FuzzFn = (input: Uint8Array) => Uint8Array | Buffer | Promise<Uint8Array> | Promise<Buffer>;

export async function fuzz(target: FuzzFn, settings?: Settings): Promise<void>
```

## Running with coverage (recommended)

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

Both `.js` and `.ts` files use `language = "js"` in the config.

For Settings, Filter/Compare entry points, and a full annotated example read `<skill-dir>/references/js-harness.md`.
