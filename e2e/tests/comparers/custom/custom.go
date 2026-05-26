package custom

import (
	"time"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.custom." + name,
			Tags: append([]string{"comparer", "custom"}, tags...),
			Func: fn,
		})
	}
	r("CaseInsensitive", nil, testCaseInsensitive)
	r("CaseInsensitive_Parallel", []string{"parallel"}, testCaseInsensitiveParallel)
}

func testCaseInsensitive(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)
	framework.RequireBinary(t, "python3")

	ws := framework.NewWorkspace(t, "comparers/custom")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("custom comparator should ignore case; got %d findings", res.Stats.Findings)
	}
}

func testCaseInsensitiveParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)
	framework.RequireBinary(t, "python3")

	ws := framework.NewWorkspace(t, "comparers/custom")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("custom comparator --workers=4 should ignore case; got %d findings", res.Stats.Findings)
	}
}
