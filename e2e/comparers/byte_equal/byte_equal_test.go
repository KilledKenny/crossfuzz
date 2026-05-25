//go:build e2e

package byte_equal_test

import (
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestComparer_ByteEqual_AgreementProducesNoFindings(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "comparers/byte_equal")
	ws.RenderConfig(t, map[string]any{
		"CampaignTimeout": "5s",
		"Diverge":         false,
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("byte_equal must produce 0 findings when both targets agree; got %d", res.Stats.Findings)
	}
}

func TestComparer_ByteEqual_DivergenceProducesFindings(t *testing.T) {
	t.Parallel()
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

// Parallel variant: each worker has its own pair of target processes; the
// comparator is per-worker. Findings must still be detected.
func TestComparer_ByteEqual_DivergenceProducesFindings_Parallel(t *testing.T) {
	t.Parallel()
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
