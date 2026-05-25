package harness

import (
	"strings"
	"time"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "comparer.harness." + name,
			Tags: append([]string{"comparer", "harness"}, tags...),
			Func: fn,
		})
	}
	r("LengthOnly", nil, testLengthOnly)
	r("LengthOnly_Parallel", []string{"parallel"}, testLengthOnlyParallel)
}

func testLengthOnly(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/harness")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Started comparator harness.") {
		t.Errorf("expected 'Started comparator harness.' in stdout; got:\n%s", res.Stdout)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("harness comparator (length-only) should report 0 findings on same-length outputs; got %d", res.Stats.Findings)
	}
}

func testLengthOnlyParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/harness")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if got := strings.Count(res.Stdout, "Started comparator harness."); got != 4 {
		t.Errorf("expected 4 'Started comparator harness.' lines (one per worker), got %d", got)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("harness comparator --workers=4 should report 0 findings; got %d", res.Stats.Findings)
	}
}
