//go:build e2e

package input_filter_test

import (
	"strings"
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

// TestInputFilter_NoFilter_Baseline establishes that without any filter the
// divergent targets produce findings. This is the control case — if it does
// not produce findings, the subsequent filter tests prove nothing.
func TestInputFilter_NoFilter_Baseline(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       false,
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings == 0 {
		t.Errorf("baseline (no filter): expected divergent targets to produce findings; got 0")
	}
}

// TestInputFilter_Reject_All asserts that a filter rejecting every input
// produces no findings and a non-zero rejected count, proving the filter is
// actually invoked.
func TestInputFilter_Reject_All(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "reject",
		"Transform":       false,
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
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

// TestInputFilter_Transform_RewritesInput uses transform mode: the filter
// rewrites every input to "ZZZZZZZZ". target_identity then returns
// "ZZZZZZZZ", agreeing with target_const → no findings. Without the
// transform the targets would diverge on every input (proved by the
// baseline test above).
func TestInputFilter_Transform_RewritesInput(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "transform",
		"Transform":       true,
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
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

// ---- Parallel variants ------------------------------------------------------
// Each worker gets its own filter process (see buildFilter in cmd/crossfuzz/
// main.go); a single shared filter would serialise all workers through one
// process. These cases re-run each scenario with --workers=4.

func TestInputFilter_NoFilter_Baseline_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       false,
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings == 0 {
		t.Errorf("baseline parallel (no filter): expected divergent targets to produce findings; got 0")
	}
}

func TestInputFilter_Reject_All_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "reject",
		"Transform":       false,
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	// One "Started input filter." line per worker.
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

func TestInputFilter_Transform_RewritesInput_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "input_filter")
	ws.RenderConfig(t, map[string]any{
		"UseFilter":       true,
		"FilterDir":       "transform",
		"Transform":       true,
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("transform filter --workers=4 should rewrite all inputs to 'ZZZZZZZZ' yielding 0 findings; got %d", res.Stats.Findings)
	}
}
