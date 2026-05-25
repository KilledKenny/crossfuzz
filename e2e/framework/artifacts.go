//go:build e2e

package framework

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CorpusFiles returns the list of corpus entry filenames under <ws.Dir>/<sub>.
// Returns an empty slice if the directory does not exist.
func CorpusFiles(t *testing.T, ws *Workspace, sub string) []string {
	t.Helper()
	dir := filepath.Join(ws.Dir, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Finding is one entry under findings/. Hash is the directory name; Kind is
// "divergence" (no prefix), "crash", or "timeout".
type Finding struct {
	Hash     string
	Kind     string
	Dir      string
	Metadata map[string]any
	Files    []string
}

// Findings returns all subdirectories of <ws.Dir>/<sub>, parsed.
func Findings(t *testing.T, ws *Workspace, sub string) []Finding {
	t.Helper()
	dir := filepath.Join(ws.Dir, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	var out []Finding
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		f := Finding{Hash: name, Dir: filepath.Join(dir, name)}
		switch {
		case strings.HasPrefix(name, "crash_"):
			f.Kind = "crash"
		case strings.HasPrefix(name, "timeout_"):
			f.Kind = "timeout"
		default:
			f.Kind = "divergence"
		}
		meta, _ := os.ReadFile(filepath.Join(f.Dir, "metadata.json"))
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &f.Metadata)
		}
		inner, _ := os.ReadDir(f.Dir)
		for _, i := range inner {
			if !i.IsDir() {
				f.Files = append(f.Files, i.Name())
			}
		}
		out = append(out, f)
	}
	return out
}
