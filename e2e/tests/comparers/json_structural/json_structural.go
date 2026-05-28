package json_structural

import (
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.json_structural." + name,
			Tags: append([]string{"comparer", "json_structural"}, tags...),
			Func: fn,
		})
	}
	r("IgnoresKeyOrderAndWhitespace", nil, testIgnores)
	r("IgnoresKeyOrderAndWhitespace_Parallel", []string{"parallel"}, testIgnoresParallel)
}

func testIgnores(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/json_structural")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("json_structural must ignore key order + whitespace; got %d findings", res.Stats.Findings)
	}
}

func testIgnoresParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/json_structural")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "30s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("json_structural --workers=4 must ignore key order + whitespace; got %d findings", res.Stats.Findings)
	}
}
