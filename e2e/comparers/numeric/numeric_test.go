//go:build e2e

package numeric_test

import (
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestComparer_Numeric_IgnoresFormattingDifferences(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// Both targets emit the same integer but with different whitespace and
	// newline. byte_equal would flag every input; numeric parses both as
	// float64 and finds them equal within default epsilon.
	ws := framework.NewWorkspace(t, "comparers/numeric")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric must accept whitespace-formatted same-value outputs; got %d findings", res.Stats.Findings)
	}
}

func TestComparer_Numeric_IgnoresFormattingDifferences_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/numeric")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("numeric --workers=4 must accept whitespace-formatted same-value outputs; got %d findings", res.Stats.Findings)
	}
}
