# crossfuzz e2e tests

End-to-end tests that drive `bin/crossfuzz` as a subprocess and assert on its
observable outputs (stdout stats, exit codes, corpus/findings directories).
Complement the unit tests in `pkg/*` by exercising the full coordinator ↔
harness flow across every supported language.

The suite is a standalone binary `bin/crossfuzz-e2e`, not a `go test` package.
The earlier `go test`-based version was replaced because Go's test runner
bundles features that are at best irrelevant to an integration suite that
shells out to a binary (caching, coverage, package boundaries) and at worst
obscure the actual results.

## Running

```bash
make test-e2e                 # builds the suite + harnesses, runs everything
make test-e2e E2E_ARGS='-v'   # forward flags to the binary
```

Or invoke the binary directly:

```bash
bin/crossfuzz-e2e                          # run everything
bin/crossfuzz-e2e -list                    # list registered tests with their tags
bin/crossfuzz-e2e -run '^cli\.'            # regex on test name (cli.* only)
bin/crossfuzz-e2e -run MaxMemory           # any test whose name contains "MaxMemory"
bin/crossfuzz-e2e -tag harness -tag go     # AND across multiple -tag flags
bin/crossfuzz-e2e -parallel 1 -v           # serial, with per-test log lines
bin/crossfuzz-e2e -failfast                # stop dispatching after the first failure
```

Exit code is `0` only when every selected test passes; failures and panics
return `1`.

## Toolchain matrix

Each per-harness test skips if its toolchain or pre-built harness artifact is
missing. Run `make harness` first to populate the artifacts.

| Test tag | Needs |
|----------|-------|
| `harness:go`     | `go` |
| `harness:c`      | `clang-19` |
| `harness:cpp`    | `clang-19`, `clang++-19` |
| `harness:java`   | `java`, `javac`, `harness/java/build/libs/crossfuzz.jar` |
| `harness:js`     | `bun`, `harness/js/node_modules` |
| `harness:python` | `harness/python/.venv/bin/python3` |
| `harness:rust`   | `cargo`, `harness/rust/target/release/libcrossfuzz_harness.rlib` |
| `cli`            | `go` (uses byte_echo Go fixture) |
| `coverage`       | `go` |
| `differential`   | `go`, `clang-19` (crashy fixture) |
| `subcommand`     | `go` |
| `comparer:*`     | `go`, plus `python3` for `comparer:custom` |
| `input_filter`   | `go` |

A skipped test prints `skip` and the reason; CI can be configured to fail on
any skip if full coverage is required.

## Layout

```
e2e/
├── main.go              # CLI entry point
├── framework/           # the test runtime + helpers
│   ├── ctx.go           # framework.T (mimics *testing.T's surface)
│   ├── registry.go      # global test registry
│   ├── orchestrator.go  # parallel test runner with summary
│   ├── workspace.go     # tmpdir + .tmpl renderer
│   ├── runner.go        # subprocess wrappers (Run/Build/Reduce/Analyze)
│   ├── stats.go         # parses crossfuzz ticker + final summary
│   ├── artifacts.go     # walks corpus/ and findings/
│   └── toolchain.go     # Require* helpers (skip when toolchain missing)
│
├── tests/               # test bodies; each subpackage's init() registers
│   ├── all.go           # side-effect imports of every subpackage
│   ├── cli/             # CLI flag tests
│   ├── coverage/        # discovery + warmup
│   ├── differential/    # finding artifact structure
│   ├── restart/         # corpus reload on second run
│   ├── subcommands/     # build, reduce, analyze
│   ├── input_filter/    # baseline + reject + transform (+ parallel)
│   ├── harness/         # per-language harness tests (see ./harness/README.md)
│   └── comparers/       # per-comparator-type tests
│
└── fixtures/            # minimal target programs used by tests
    ├── byte_echo/       # all 7 langs
    ├── branchy/         # broad branch surface
    ├── divergent/       # two Go targets, one buggy
    ├── slow/            # timeout fixture
    ├── crashy/          # crash fixture
    └── memhog/          # max-memory fixture
```

Fixtures live next to the comparer/input_filter test subdirectories too (under
`e2e/comparers/<x>/` and `e2e/input_filter/`); `framework.NewWorkspace` looks
in both `e2e/fixtures/<name>` and `e2e/<name>` so colocated fixtures work.

## Tags

Every registered test carries tags. Use them to slice runs:

```bash
bin/crossfuzz-e2e -tag comparer            # all comparator tests
bin/crossfuzz-e2e -tag parallel            # every multi-worker variant
bin/crossfuzz-e2e -tag harness -tag rust   # rust harness only
bin/crossfuzz-e2e -tag warmup              # post-warmup stability tests
```

`-tag` is repeatable and combines with AND. Use `-run` (regex on name) for
finer slicing.

## How a test is written

Tests register themselves via `init()`; the binary imports every test
subpackage for side effects.

```go
package mycategory

import (
    "time"
    "crossfuzz/e2e/framework"
)

func init() {
    framework.Register(framework.Test{
        Name: "mycategory.MyCheck",
        Tags: []string{"mycategory"},
        Func: testMyCheck,
    })
}

func testMyCheck(t *framework.T) {
    framework.RequireCrossfuzzBinary(t)
    framework.RequireGo(t)

    ws := framework.NewWorkspace(t, "byte_echo")
    ws.RenderConfig(t, map[string]any{"Go": true, "CampaignTimeout": "5s"})
    if r := framework.Build(t, ws); r.ExitCode != 0 {
        t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
    }
    res := framework.RunWithTimeout(t, ws, 30*time.Second)
    if res.Stats.Findings != 0 {
        t.Errorf("expected 0 findings, got %d", res.Stats.Findings)
    }
}
```

`framework.T` exposes `Errorf` / `Fatalf` / `Skipf` / `Logf` / `Cleanup` /
`TempDir` / `Helper` with the same semantics as `*testing.T`. `Fatalf` aborts
the current test via panic and is recovered by the orchestrator; other tests
keep running unless `-failfast` is set. Tests must not call `t.Parallel()` —
parallelism is controlled centrally by `-parallel N`.

## Fixtures and the template system

A "fixture" is a directory containing a target program and a
`crossfuzz.toml.tmpl`. `framework.NewWorkspace(t, name)`:

1. Copies the fixture into a tmpdir owned by the test.
2. Renders every `*.tmpl` file in the copy, writing the result to the same
   path with `.tmpl` stripped.

`{{.RepoRoot}}` is always available, so templates can reference repo-relative
paths like `{{.RepoRoot}}/harness/c`. Tests pass their own vars
(`CampaignTimeout`, `ExecTimeout`, language flags like `Go` / `C` / `Java`)
via the second argument to `RenderConfig`.

Common template files:

- `crossfuzz.toml.tmpl` — the campaign config.
- `go.mod.tmpl` — for Go targets, supplies `replace crossfuzz => {{.RepoRoot}}`.
- `Cargo.toml.tmpl` — same idea for Rust.
- `echo.py.tmpl`, `echo.js.tmpl` — when the script itself needs the harness path.

## Adding a new fixture

1. Create `fixtures/<name>/` with at least `crossfuzz.toml.tmpl`, one target
   subdirectory, and a `seeds/` directory.
2. If the target language is Go or Rust, include the corresponding
   `go.mod.tmpl` / `Cargo.toml.tmpl` with `replace ... => {{.RepoRoot}}`.
3. Write a test under `e2e/tests/<category>/` that calls
   `framework.NewWorkspace(t, "<name>")`, renders, builds, and runs.

## Adding a new harness language

See `tests/harness/README.md` — adding a new language is a copy-paste of the
four-category test contract plus a target source file under
`fixtures/byte_echo/<lang>/`.

## Determinism

Mutation is non-deterministic so all assertions are written in terms of
ranges and invariants (`corpus > seeds`, `findings >= 1`, `coverage diff <= 2`)
rather than exact counts. A future `--seed` flag in the coordinator would let
us tighten these.

Tests use short `CampaignTimeout` values (5–20s) so the suite runs in well
under two minutes on a full-toolchain machine.
