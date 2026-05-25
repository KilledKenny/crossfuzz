# cross_fuzz e2e tests

End-to-end tests that drive `bin/crossfuzz` as a subprocess and assert on its
observable outputs (stdout stats, exit codes, corpus/findings directories).
Complement the unit tests in `pkg/*` by exercising the full coordinator ↔
harness flow across every supported language.

## Running

```bash
make test-e2e          # builds bin/crossfuzz + all harnesses, then runs the suite
```

Or run a subset directly:

```bash
go test -tags=e2e ./e2e/harness/ -run TestGoHarness          # one language only
go test -tags=e2e ./e2e/ -run TestCLI_                       # only CLI flag tests
go test -tags=e2e ./e2e/ -run TestDifferential_              # only findings tests
go test -tags=e2e -count=1 ./e2e/...                         # everything, no caching
```

All tests are gated behind the `e2e` build tag so the default `make test`
stays fast and toolchain-free.

## Toolchain matrix

Each per-harness test skips if its toolchain or pre-built harness artifact is
missing. Run `make harness` first to populate the artifacts.

| Test | Needs |
|------|-------|
| `harness/go_test.go`     | `go` |
| `harness/c_test.go`      | `clang-19` |
| `harness/cpp_test.go`    | `clang-19`, `clang++-19` |
| `harness/java_test.go`   | `java`, `javac`, `harness/java/build/libs/crossfuzz.jar` |
| `harness/js_test.go`     | `bun`, `harness/js/node_modules` |
| `harness/python_test.go` | `harness/python/.venv/bin/python3` |
| `harness/rust_test.go`   | `cargo`, `harness/rust/target/release/libcrossfuzz_harness.rlib` |
| `cli_test.go`            | `go` (uses byte_echo Go fixture) |
| `coverage_test.go`       | `go` |
| `differential_test.go`   | `go`, `clang-19` (crashy fixture) |
| `subcommands_test.go`    | `go` |

A skipped test prints `SKIP` and the reason; CI can be configured to fail on
any skip if full coverage is required.

## Layout

```
e2e/
├── framework/         # shared test helpers (subprocess runner, stats parser,
│                      # workspace+template renderer, toolchain probes)
├── fixtures/          # minimal target programs used by tests
│   ├── byte_echo/     # all 7 langs; returns input unchanged with byte-class
│   │                  # branches so the fuzzer has paths to discover
│   ├── branchy/       # Go; broad branch surface for coverage discovery
│   ├── divergent/     # two Go targets, one with an intentional bug
│   ├── slow/          # Go; sleeps on trigger byte → timeout finding
│   └── crashy/        # C; abort() on trigger byte → crash finding
├── harness/           # per-language harness tests (see harness/README.md
│                      # for the four-category test contract)
├── cli_test.go        # CLI flag tests (--timeout, --workers, --warmup, …)
├── coverage_test.go   # coverage discovery + warmup behavior
├── differential_test.go # divergence/crash/timeout finding artifacts
└── subcommands_test.go  # build, reduce, analyze
```

## Fixtures and the template system

A "fixture" is a directory under `fixtures/` containing a target program and
a `crossfuzz.toml.tmpl`. When a test calls `framework.NewWorkspace(t, name)`,
the framework:

1. Copies the fixture into a `t.TempDir()` so tests are isolated.
2. Renders every `*.tmpl` file in the copy with the test's variables (writing
   the result to the path with `.tmpl` stripped).

`{{.RepoRoot}}` is always available, so templates can reference repo-relative
paths like `{{.RepoRoot}}/harness/c` without hardcoding. Tests can also pass
their own vars — `CampaignTimeout`, `ExecTimeout`, `MaxInputSize`, language
flags like `Go` / `C` / `Java` — to customize the rendered TOML per test
case.

Common template files:

- `crossfuzz.toml.tmpl` — the campaign config.
- `go.mod.tmpl` — required for Go targets, supplies `replace crossfuzz => {{.RepoRoot}}` so the fixture can import the local harness from the tmpdir.
- `Cargo.toml.tmpl` — same idea for Rust.
- `echo.py.tmpl`, `echo.js.tmpl` — used when the script itself needs a path to the harness directory.

## Adding a new fixture

1. Create `fixtures/<name>/` with at least:
   - `crossfuzz.toml.tmpl`
   - one target subdirectory (`go/`, `c/`, etc.)
   - `seeds/` with at least one seed file
2. If the target language is Go or Rust, include the corresponding
   `go.mod.tmpl` / `Cargo.toml.tmpl` with a `replace` pointing at `{{.RepoRoot}}`.
3. Write a test in `e2e/*_test.go` that calls `framework.NewWorkspace(t, "<name>")`,
   renders the config, builds, and runs.

## Adding a new harness language

See `harness/README.md` — adding a new language is a copy-paste of the four-
category test contract plus a target source file under `fixtures/byte_echo/<lang>/`.

## Determinism

Mutation is non-deterministic so all assertions are written in terms of
ranges and invariants (`corpus > seeds`, `findings >= 1`, `coverage diff <= 2`)
rather than exact counts. A future `--seed` flag in the coordinator would let
us tighten these.

Tests use short `CampaignTimeout` values (5–20s) so the suite runs in under
a minute on a full-toolchain machine.
