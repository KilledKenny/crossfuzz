//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestDifferential_DivergenceFindingStructure(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "divergent")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "20s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 40*time.Second, "--max-findings", "5")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}

	findings := framework.Findings(t, ws, "findings")
	var divergences []framework.Finding
	for _, f := range findings {
		if f.Kind == "divergence" {
			divergences = append(divergences, f)
		}
	}
	if len(divergences) < 1 {
		t.Fatalf("expected >=1 divergence finding from divergent fixture, got %d (all findings: %v)", len(divergences), findings)
	}

	// Inspect the first divergence finding's structure.
	f := divergences[0]
	if !hasFile(f.Files, "input.bin") {
		t.Errorf("expected finding to contain input.bin; got files: %v", f.Files)
	}
	// Per-target output files: output_good.bin and output_buggy.bin.
	if !hasFile(f.Files, "output_good.bin") {
		t.Errorf("expected output_good.bin in finding; got files: %v", f.Files)
	}
	if !hasFile(f.Files, "output_buggy.bin") {
		t.Errorf("expected output_buggy.bin in finding; got files: %v", f.Files)
	}
	if f.Metadata == nil {
		t.Errorf("expected metadata.json to be parsed; got nil")
	}
}

func TestDifferential_CrashFinding(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireClang19(t)

	ws := framework.NewWorkspace(t, "crashy")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "15s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "5")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}

	findings := framework.Findings(t, ws, "findings")
	var crashes []framework.Finding
	for _, f := range findings {
		if f.Kind == "crash" {
			crashes = append(crashes, f)
		}
	}
	if len(crashes) < 1 {
		t.Fatalf("expected >=1 crash finding from crashy fixture, got %d findings: %v", len(findings), findings)
	}
	if !strings.HasPrefix(crashes[0].Hash, "crash_crashy_") {
		t.Errorf("expected crash dir to start with 'crash_crashy_', got %q", crashes[0].Hash)
	}
	if !hasFile(crashes[0].Files, "input.bin") {
		t.Errorf("expected input.bin in crash finding; got files: %v", crashes[0].Files)
	}
}

func TestDifferential_TimeoutFinding(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "slow")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "200ms",
		"CampaignTimeout": "10s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second,
		"--timeout", "200ms",
		"--max-findings", "5",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}

	findings := framework.Findings(t, ws, "findings")
	var timeouts []framework.Finding
	for _, f := range findings {
		if f.Kind == "timeout" {
			timeouts = append(timeouts, f)
		}
	}
	if len(timeouts) < 1 {
		t.Fatalf("expected >=1 timeout finding from slow fixture, got %d findings: %v", len(findings), findings)
	}
	if !strings.HasPrefix(timeouts[0].Hash, "timeout_slow_") {
		t.Errorf("expected timeout dir to start with 'timeout_slow_', got %q", timeouts[0].Hash)
	}
}

// ---- Parallel variants ------------------------------------------------------
// Findings (divergence/crash/timeout) must still be detected and recorded
// when multiple workers run concurrently. Each worker has its own target
// processes, so a finding can come from any worker.

func TestDifferential_DivergenceFindingStructure_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "divergent")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "20s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 40*time.Second, "--max-findings", "5", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	findings := framework.Findings(t, ws, "findings")
	var divergences []framework.Finding
	for _, f := range findings {
		if f.Kind == "divergence" {
			divergences = append(divergences, f)
		}
	}
	if len(divergences) < 1 {
		t.Fatalf("expected >=1 divergence finding from divergent fixture with --workers=4; got %d", len(divergences))
	}
}

func TestDifferential_CrashFinding_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireClang19(t)

	ws := framework.NewWorkspace(t, "crashy")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "15s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "5", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	var crashes []framework.Finding
	for _, f := range framework.Findings(t, ws, "findings") {
		if f.Kind == "crash" {
			crashes = append(crashes, f)
		}
	}
	if len(crashes) < 1 {
		t.Fatalf("expected >=1 crash finding from crashy fixture with --workers=4; got 0")
	}
}

func TestDifferential_TimeoutFinding_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "slow")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "200ms",
		"CampaignTimeout": "10s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second,
		"--timeout", "200ms",
		"--max-findings", "5",
		"--workers", "4",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	var timeouts []framework.Finding
	for _, f := range framework.Findings(t, ws, "findings") {
		if f.Kind == "timeout" {
			timeouts = append(timeouts, f)
		}
	}
	if len(timeouts) < 1 {
		t.Fatalf("expected >=1 timeout finding from slow fixture with --workers=4; got 0")
	}
}

func hasFile(files []string, name string) bool {
	for _, f := range files {
		if f == name {
			return true
		}
	}
	return false
}
