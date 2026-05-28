package numeric

import (
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.numeric." + name,
			Tags: append([]string{"comparer", "numeric"}, tags...),
			Func: fn,
		})
	}
	r("IgnoresFormattingDifferences", nil, testIgnoresFormatting)
	r("IgnoresFormattingDifferences_Parallel", []string{"parallel"}, testIgnoresFormattingParallel)
}

func testIgnoresFormatting(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/numeric")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric must accept whitespace-formatted same-value outputs; got %d findings", res.Stats.Findings)
	}
}

func testIgnoresFormattingParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/numeric")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric --workers=4 must accept whitespace-formatted same-value outputs; got %d findings", res.Stats.Findings)
	}
}
