//go:build e2e

package json_structural_test

import (
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestComparer_JSONStructural_IgnoresKeyOrderAndWhitespace(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// compact/ and reordered/ emit the same JSON object but with different
	// key order and whitespace. json_structural must report no findings;
	// byte_equal would report on every single input.
	ws := framework.NewWorkspace(t, "comparers/json_structural")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("json_structural must ignore key order + whitespace; got %d findings", res.Stats.Findings)
	}
}

func TestComparer_JSONStructural_IgnoresKeyOrderAndWhitespace_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/json_structural")
	ws.RenderConfig(t, map[string]any{"CampaignTimeout": "8s"})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("json_structural --workers=4 must ignore key order + whitespace; got %d findings", res.Stats.Findings)
	}
}
