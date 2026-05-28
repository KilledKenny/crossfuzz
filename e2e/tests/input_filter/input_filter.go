package input_filter

import (
	"strings"
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "input_filter." + name,
			Tags: append([]string{"input_filter"}, tags...),
			Func: fn,
		})
	}
	r("NoFilter_Baseline", nil, testNoFilterBaseline)
	r("Reject_All", nil, testRejectAll)
	r("Transform_RewritesInput", nil, testTransform)
	r("NoFilter_Baseline_Parallel", []string{"parallel"}, testNoFilterBaselineParallel)
	r("Reject_All_Parallel", []string{"parallel"}, testRejectAllParallel)
	r("Transform_RewritesInput_Parallel", []string{"parallel"}, testTransformParallel)
}

// testNoFilterBaseline establishes that without any filter the divergent
// targets produce findings. If this fails, the other filter tests prove nothing.
func testNoFilterBaseline(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       false,
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings == 0 {
		t.Errorf("baseline (no filter): expected divergent targets to produce findings; got 0")
	}
}

func testRejectAll(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "reject",
		"Transform":       false,
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	// reject filter discards every input, so --stop-after=<N> (an exec
	// counter) would never trip. Use the duration form instead.
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "2s")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Started input filter.") {
		t.Errorf("expected 'Started input filter.' in stdout; got:\n%s", res.Stdout)
	}
	if res.Stats.Rejected == 0 {
		t.Errorf("expected reject-all filter to produce rejected > 0; got 0")
	}
	if res.Stats.Findings != 0 {
		t.Errorf("expected 0 findings when filter rejects everything; got %d", res.Stats.Findings)
	}
}

func testTransform(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "transform",
		"Transform":       true,
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Started input filter.") {
		t.Errorf("expected 'Started input filter.' in stdout; got:\n%s", res.Stdout)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("transform filter rewrites every input to 'ZZZZZZZZ' which both targets return — expected 0 findings, got %d", res.Stats.Findings)
	}
}

func testNoFilterBaselineParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       false,
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings == 0 {
		t.Errorf("baseline parallel (no filter): expected divergent targets to produce findings; got 0")
	}
}

func testRejectAllParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "reject",
		"Transform":       false,
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	// See testRejectAll: exec-count --stop-after never trips with reject-all.
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "2s")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if got := strings.Count(res.Stdout, "Started input filter."); got != 4 {
		t.Errorf("expected 4 'Started input filter.' lines (one per worker), got %d", got)
	}
	if res.Stats.Rejected == 0 {
		t.Errorf("expected reject-all filter to produce rejected > 0; got 0")
	}
	if res.Stats.Findings != 0 {
		t.Errorf("expected 0 findings when filter rejects everything; got %d", res.Stats.Findings)
	}
}

func testTransformParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "transform",
		"Transform":       true,
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "200")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("transform filter --workers=4 should rewrite all inputs to 'ZZZZZZZZ' yielding 0 findings; got %d", res.Stats.Findings)
	}
}
