//go:build e2e

package harness_compare_test

import (
	"strings"
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestComparer_Harness_LengthOnly(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// raw/ returns input, shuffled/ returns reversed input — same length,
	// totally different bytes. The harness comparator binary only checks
	// length equality, so no findings are expected. byte_equal would flag
	// every non-palindrome.
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

// Parallel variant — each worker must spawn its own comparator harness
// process bound to that worker's targets' SHM regions; a single shared
// comparator would race on cross-worker target outputs.
func TestComparer_Harness_LengthOnly_Parallel(t *testing.T) {
	t.Parallel()
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
	// One "Started comparator harness." line per worker.
	if got := strings.Count(res.Stdout, "Started comparator harness."); got != 4 {
		t.Errorf("expected 4 'Started comparator harness.' lines (one per worker), got %d", got)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("harness comparator --workers=4 should report 0 findings; got %d", res.Stats.Findings)
	}
}
