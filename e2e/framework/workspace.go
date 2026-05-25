package framework

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// Workspace is an isolated tmpdir for one e2e test. It holds a copy of a
// fixture and a rendered crossfuzz.toml. Removed automatically on test
// cleanup (via T.TempDir's cleanup).
type Workspace struct {
	Dir        string
	ConfigPath string
	RepoRoot   string
	FixtureDir string
}

// NewWorkspace creates a tmpdir, copies the named fixture into it, and
// renders every *.tmpl file with default vars. Fixtures are resolved first
// from e2e/fixtures/<name>, then from e2e/<name>.
func NewWorkspace(t *T, fixture string) *Workspace {
	t.Helper()
	root := repoRoot(t)
	candidates := []string{
		filepath.Join(root, "e2e", "fixtures", fixture),
		filepath.Join(root, "e2e", fixture),
	}
	var src string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			src = c
			break
		}
	}
	if src == "" {
		t.Fatalf("fixture %q not found in e2e/fixtures/ or e2e/", fixture)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatalf("copy fixture %q: %v", fixture, err)
	}
	ws := &Workspace{
		Dir:        dir,
		ConfigPath: filepath.Join(dir, "crossfuzz.toml"),
		RepoRoot:   root,
		FixtureDir: src,
	}
	ws.RenderConfig(t, nil)
	return ws
}

// RenderConfig walks the workspace, rendering every *.tmpl file with the
// given vars and writing the result to the same path with the .tmpl suffix
// removed. {{.RepoRoot}} is always available. Re-callable from tests to
// vary inputs.
func (w *Workspace) RenderConfig(t *T, vars map[string]any) {
	t.Helper()
	if vars == nil {
		vars = map[string]any{}
	}
	if _, ok := vars["RepoRoot"]; !ok {
		vars["RepoRoot"] = w.RepoRoot
	}
	err := filepath.Walk(w.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tmpl, err := template.New(filepath.Base(path)).Parse(string(raw))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, vars); err != nil {
			return err
		}
		out := strings.TrimSuffix(path, ".tmpl")
		return os.WriteFile(out, buf.Bytes(), 0o644)
	})
	if err != nil {
		t.Fatalf("render templates: %v", err)
	}
}

func repoRoot(t *T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
