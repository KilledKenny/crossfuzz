//go:build e2e

package numeric_relative_test

import (
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestComparer_NumericRelative_AcceptsSmallRelativeDiffs(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// exact/ and noisy/ differ by a relative factor of 1e-12. The default
	// numeric_relative epsilon is 1e-9 so both must be accepted as equal.
	ws := framework.NewWorkspace(t, "comparers/numeric_relative")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric_relative must accept rel-diff 1e-12; got %d findings", res.Stats.Findings)
	}
}

func TestComparer_NumericRelative_AcceptsSmallRelativeDiffs_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/numeric_relative")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric_relative --workers=4 must accept rel-diff 1e-12; got %d findings", res.Stats.Findings)
	}
}
