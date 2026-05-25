package none

import (
	"time"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.none." + name,
			Tags: append([]string{"comparer", "none"}, tags...),
			Func: fn,
		})
	}
	r("NeverReportsFindings", nil, testNoFindings)
	r("NeverReportsFindings_Parallel", []string{"parallel"}, testNoFindingsParallel)
}

func testNoFindings(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/none")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("comparator=none must produce 0 findings even with divergent targets; got %d", res.Stats.Findings)
	}
}

func testNoFindingsParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/none")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("comparator=none with --workers=4 must produce 0 findings; got %d", res.Stats.Findings)
	}
}
