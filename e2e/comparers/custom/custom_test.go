//go:build e2e

package custom_test

import (
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestComparer_Custom_CaseInsensitive(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)
	framework.RequireBinary(t, "python3")

	// upper/ and lower/ disagree byte-wise on every letter; the custom
	// comparator script normalises case before comparing, so no findings
	// should appear.
	ws := framework.NewWorkspace(t, "comparers/custom")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("custom comparator should ignore case; got %d findings", res.Stats.Findings)
	}
}

func TestComparer_Custom_CaseInsensitive_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)
	framework.RequireBinary(t, "python3")

	ws := framework.NewWorkspace(t, "comparers/custom")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("custom comparator --workers=4 should ignore case; got %d findings", res.Stats.Findings)
	}
}
