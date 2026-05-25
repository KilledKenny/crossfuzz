package differential

import (
	"strings"
	"time"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "differential." + name,
			Tags: append([]string{"differential"}, tags...),
			Func: fn,
		})
	}
	r("DivergenceFindingStructure", nil, testDivergence)
	r("CrashFinding", nil, testCrash)
	r("TimeoutFinding", nil, testTimeout)
	r("DivergenceFindingStructure_Parallel", []string{"parallel"}, testDivergenceParallel)
	r("CrashFinding_Parallel", []string{"parallel"}, testCrashParallel)
	r("TimeoutFinding_Parallel", []string{"parallel"}, testTimeoutParallel)
}

func testDivergence(t *framework.T) {
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
	var divergences []framework.Finding
	for _, f := range framework.Findings(t, ws, "findings") {
		if f.Kind == "divergence" {
			divergences = append(divergences, f)
		}
	}
	if len(divergences) < 1 {
		t.Fatalf("expected >=1 divergence finding; got %d", len(divergences))
	}
	f := divergences[0]
	if !hasFile(f.Files, "input.bin") {
		t.Errorf("expected input.bin in finding; got files: %v", f.Files)
	}
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

func testCrash(t *framework.T) {
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
	var crashes []framework.Finding
	for _, f := range framework.Findings(t, ws, "findings") {
		if f.Kind == "crash" {
			crashes = append(crashes, f)
		}
	}
	if len(crashes) < 1 {
		t.Fatalf("expected >=1 crash finding from crashy fixture; got 0")
	}
	if !strings.HasPrefix(crashes[0].Hash, "crash_crashy_") {
		t.Errorf("expected crash dir to start with 'crash_crashy_', got %q", crashes[0].Hash)
	}
	if !hasFile(crashes[0].Files, "input.bin") {
		t.Errorf("expected input.bin in crash finding; got files: %v", crashes[0].Files)
	}
}

func testTimeout(t *framework.T) {
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
	var timeouts []framework.Finding
	for _, f := range framework.Findings(t, ws, "findings") {
		if f.Kind == "timeout" {
			timeouts = append(timeouts, f)
		}
	}
	if len(timeouts) < 1 {
		t.Fatalf("expected >=1 timeout finding from slow fixture; got 0")
	}
	if !strings.HasPrefix(timeouts[0].Hash, "timeout_slow_") {
		t.Errorf("expected timeout dir to start with 'timeout_slow_', got %q", timeouts[0].Hash)
	}
}

func testDivergenceParallel(t *framework.T) {
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
	var divergences []framework.Finding
	for _, f := range framework.Findings(t, ws, "findings") {
		if f.Kind == "divergence" {
			divergences = append(divergences, f)
		}
	}
	if len(divergences) < 1 {
		t.Fatalf("expected >=1 divergence finding with --workers=4; got %d", len(divergences))
	}
}

func testCrashParallel(t *framework.T) {
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
		t.Fatalf("expected >=1 crash finding with --workers=4; got 0")
	}
}

func testTimeoutParallel(t *framework.T) {
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
		t.Fatalf("expected >=1 timeout finding with --workers=4; got 0")
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
