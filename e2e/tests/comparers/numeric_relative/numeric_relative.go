package numeric_relative

import (
	"time"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.numeric_relative." + name,
			Tags: append([]string{"comparer", "numeric_relative"}, tags...),
			Func: fn,
		})
	}
	r("AcceptsSmallRelativeDiffs", nil, testAcceptsRelDiff)
	r("AcceptsSmallRelativeDiffs_Parallel", []string{"parallel"}, testAcceptsRelDiffParallel)
}

func testAcceptsRelDiff(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/numeric_relative")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric_relative must accept rel-diff 1e-12; got %d findings", res.Stats.Findings)
	}
}

func testAcceptsRelDiffParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/numeric_relative")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric_relative --workers=4 must accept rel-diff 1e-12; got %d findings", res.Stats.Findings)
	}
}
