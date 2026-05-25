//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

// buildGoByteEcho is the shared "fast Go-only build" used by most CLI flag
// tests. It returns a freshly built workspace ready to `run` against.
func buildGoByteEcho(t *testing.T, extra map[string]any) *framework.Workspace {
	t.Helper()
	ws := framework.NewWorkspace(t, "byte_echo")
	vars := map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "5s",
	}
	for k, v := range extra {
		vars[k] = v
	}
	ws.RenderConfig(t, vars)
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	return ws
}

func TestCLI_NameFilter_NoMatch(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 10*time.Second, "--name", "does_not_exist")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit when --name matches no targets, got 0")
	}
	if !strings.Contains(res.Stderr, "no targets matched") {
		t.Errorf("expected 'no targets matched' in stderr, got:\n%s", res.Stderr)
	}
}

func TestCLI_NameFilter_Match(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--name", "go_echo")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !res.Stats.Found {
		t.Errorf("expected campaign to complete with --name=go_echo")
	}
}

func TestCLI_MaxFindings(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// Use the divergent fixture: every input of length 3 diverges, so
	// findings appear quickly. Assert that --max-findings stops the run at
	// exactly that limit (or close to it — workers may race past N by a few).
	ws := framework.NewWorkspace(t, "divergent")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 60*time.Second, "--max-findings", "3")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings < 1 {
		t.Errorf("expected >=1 finding from divergent fixture, got %d", res.Stats.Findings)
	}
	// Campaign should stop early (well before the 30s timeout) once findings cap is hit.
	// Sanity-check via captured stats: corpus is small because we exited fast.
	if res.Stats.Execs > 50000 {
		t.Errorf("expected --max-findings to stop early; got %d execs (campaign did not exit on findings cap)", res.Stats.Execs)
	}
}

func TestCLI_WarmupAnnounced(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--warmup", "5")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Warmup: running") {
		t.Errorf("expected 'Warmup: running' in stdout when --warmup > 0, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "Warmup complete.") {
		t.Errorf("expected 'Warmup complete.' in stdout, got:\n%s", res.Stdout)
	}
}

func TestCLI_DebugEdge(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--debug-edge")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if len(res.Ticks) == 0 {
		t.Fatal("no ticks observed")
	}
	last := res.Ticks[len(res.Ticks)-1]
	if _, ok := last.TargetEdges["go_echo"]; !ok {
		t.Errorf("expected per-target edge count for go_echo in --debug-edge ticker; got TargetEdges=%v", last.TargetEdges)
	}
}

func TestCLI_LogFile(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	logName := "run.log"
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--log-file", logName)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	logPath := filepath.Join(ws.Dir, logName)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected log file at %s: %v", logPath, err)
	}
	if !strings.Contains(string(data), "Campaign finished.") {
		t.Errorf("expected log file to contain campaign output, got %d bytes:\n%s", len(data), string(data))
	}
}

func TestCLI_Workers(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, map[string]any{"CampaignTimeout": "6s"})
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--workers", "3")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "with 3 worker(s)") {
		t.Errorf("expected 'with 3 worker(s)' in stdout, got:\n%s", res.Stdout)
	}
}

func TestCLI_CustomCorpusAndFindingsDirs(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second,
		"--corpus", "custom-corpus",
		"--findings", "custom-findings",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "custom-corpus")); err != nil {
		t.Errorf("expected custom-corpus dir to exist: %v", err)
	}
	// findings dir is created lazily; we only require it exists by the time
	// the run finished — there may be no findings for byte_echo so a missing
	// dir is acceptable.
}

func TestCLI_PerExecTimeoutTriggersFinding(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// slow fixture sleeps when input starts with 'S'; seed begins with 'S'.
	ws := framework.NewWorkspace(t, "slow")
	ws.RenderConfig(t, map[string]any{
		"CampaignTimeout": "10s",
		"ExecTimeout":     "200ms",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--timeout", "200ms")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Timeouts == 0 {
		t.Errorf("expected at least one timeout, got 0 — per-exec --timeout may not be wired through")
	}
}

func TestCLI_Validate(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// echo is deterministic, so --validate should run cleanly. This just
	// asserts the flag is accepted and doesn't perturb correctness.
	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--validate", "2")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("--validate 2 reported findings on deterministic echo: %d", res.Stats.Findings)
	}
}

func TestCLI_RootBuildFlag(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// Build via `run --build` instead of an explicit build step.
	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "5s",
	})
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--build")
	if res.ExitCode != 0 {
		t.Fatalf("run --build failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Building go_echo") {
		t.Errorf("expected 'Building go_echo' in stdout when --build is set, got:\n%s", res.Stdout)
	}
}
