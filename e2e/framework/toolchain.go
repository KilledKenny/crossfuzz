//go:build e2e

package framework

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Each Require* helper t.Skip()s the calling test if the required toolchain
// or harness build artifact is missing on this machine. This lets developers
// run the subset of e2e tests their environment supports.

// RequireBinary skips the test if `name` is not found on PATH.
func RequireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("required binary %q not on PATH: %v", name, err)
	}
}

func RequireClang19(t *testing.T) { t.Helper(); RequireBinary(t, "clang-19") }
func RequireGradle(t *testing.T)  { t.Helper(); RequireBinary(t, "gradle") }
func RequireBun(t *testing.T)     { t.Helper(); RequireBinary(t, "bun") }
func RequireCargo(t *testing.T)   { t.Helper(); RequireBinary(t, "cargo") }
func RequireGo(t *testing.T)      { t.Helper(); RequireBinary(t, "go") }
func RequireJava(t *testing.T)    { t.Helper(); RequireBinary(t, "java") }

// RequirePythonVenv skips if harness/python/.venv/bin/python3 is missing.
func RequirePythonVenv(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "harness", "python", ".venv", "bin", "python3")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("python venv missing at %s — run `make harness` or set up harness/python/.venv manually", p)
	}
}

// RequireCrossfuzzBinary skips if bin/crossfuzz does not exist.
func RequireCrossfuzzBinary(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "bin", "crossfuzz")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("bin/crossfuzz missing — run `make bin/crossfuzz`: %v", err)
	}
}

// RequireJavaHarness skips if harness/java/build/libs/crossfuzz.jar is missing.
func RequireJavaHarness(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "harness", "java", "build", "libs", "crossfuzz.jar")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("java harness jar missing at %s — run `make harness`", p)
	}
}

// RequireRustHarness skips if the rust harness rlib is missing.
func RequireRustHarness(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "harness", "rust", "target", "release", "libcrossfuzz_harness.rlib")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("rust harness rlib missing at %s — run `make harness`", p)
	}
}

// RequireJSHarness skips if the JS harness's node_modules is missing.
func RequireJSHarness(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "harness", "js", "node_modules")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("js harness node_modules missing at %s — run `make harness`", p)
	}
}
