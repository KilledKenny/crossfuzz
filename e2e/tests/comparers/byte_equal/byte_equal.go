package byte_equal

import (
	"time"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.byte_equal." + name,
			Tags: append([]string{"comparer", "byte_equal"}, tags...),
			Func: fn,
		})
	}
	r("AgreementProducesNoFindings", nil, testAgree)
	r("DivergenceProducesFindings", nil, testDiverge)
	r("DivergenceProducesFindings_Parallel", []string{"parallel"}, testDivergeParallel)
}

func testAgree(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/byte_equal")
	ws.RenderConfig(t, map[string]any{
		"CampaignTimeout": "30s",
		"Diverge":         false,
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("byte_equal must produce 0 findings when both targets agree; got %d", res.Stats.Findings)
	}
}

func testDiverge(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/byte_equal")
	ws.RenderConfig(t, map[string]any{
		"CampaignTimeout": "10s",
		"Diverge":         true,
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "5")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings == 0 {
		t.Errorf("byte_equal must produce >=1 finding when targets diverge; got 0")
	}
}

func testDivergeParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/byte_equal")
	ws.RenderConfig(t, map[string]any{
		"CampaignTimeout": "10s",
		"Diverge":         true,
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "5", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings == 0 {
		t.Errorf("byte_equal --workers=4 must produce >=1 finding when targets diverge; got 0")
	}
}
