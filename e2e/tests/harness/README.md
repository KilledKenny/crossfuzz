# Per-harness e2e tests

This directory contains one `<lang>.go` per language harness. Each file
declares a `langCase` and calls `register(...)` from its `init()` so the
e2e runner picks up the same four tests for every language: build, path
discovery, output agreement, and post-warmup coverage stability. Adding a
new harness is a copy-paste-and-implement task with a clear contract.

Each registered test starts by calling its `langCase.RequireToolchain(t)`.
When the toolchain (or the harness build artifact under `harness/<lang>/`)
is missing the test calls `t.Skipf(...)` rather than failing — developers
can run the subset their machine supports.

## Test categories applied to every harness

### 1. Build

Render the `byte_echo` fixture with only this language enabled, run
`crossfuzz build`, and assert:

- exit code 0,
- the expected binary artifact (or `.class` / `node_modules` / etc.) exists on
  disk afterwards,
- `Build complete.` appears in stdout.

Verifies the `build_cmd` in the TOML executes cleanly with this language's
toolchain.

### 2. Path discovery

Run `crossfuzz run --timeout 5s` against the `byte_echo` fixture (which has a
handful of branches per input byte for exactly this purpose). Assert:

- exit code 0,
- final corpus size is **strictly greater** than the seed count (the fuzzer
  discovered at least one new path beyond the seeds),
- final coverage edge count is `> 0`.

Verifies the harness's coverage plumbing actually reports edges back to the
coordinator and that new edges trigger corpus growth. A harness whose bitmap
never changes would fail this — corpus stays equal to seeds.

### 3. Output agreement

The `byte_echo` target returns its input unchanged in every language, so under
a `byte_equal` comparator the suite must report **zero differential findings**,
**zero crashes**, **zero timeouts**. Assert all three.

Verifies the harness round-trips input → output bytes correctly across the IPC
boundary (shared memory + pipes + length prefixes) without corruption.

### 4. Coverage stability after warmup

Run the same fixture **twice** back-to-back with `--warmup 30` on each run.
Assert the final coverage edge count is within ±2 across both runs.

Without warmup, GC / allocator / JIT noise can flip flaky bitmap slots on
early inputs, producing different edge counts run-to-run. The warmup phase
exists specifically to mask those flaky slots. The small tolerance accounts
for rare residual noise (e.g. a single GC slot in instrumented stdlib code)
that warmup can't always catch; a broken warmup would diverge by tens to
hundreds of edges, not 1–2.

## Adding a new harness

1. Copy an existing `<lang>.go` here (`go.go` is the simplest) and rename to
   `<newlang>.go`.
2. Update the `langCase` literal with the new `Tag`, template `Flag`, target
   name, expected build artifact path, and toolchain probe.
3. Add a `Require<NewLang>...` helper in `e2e/framework/toolchain.go` if a new
   probe is needed.
4. Add a `{{if .<Flag>}} [[target]] ... {{end}}` block to
   `e2e/fixtures/byte_echo/crossfuzz.toml.tmpl`.
5. Add `<lang>/` directory under `e2e/fixtures/byte_echo/` with the target
   source, implementing the same echo-with-byte-category-branches shape as the
   other languages so the path-discovery assertion has something to discover.
6. Run `bin/crossfuzz-e2e -tag harness -tag <newlang>` — only the new
   language's three tests should change behaviour.
