//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

func TestCoverage_DiscoveryGrowsCorpusAndEdges(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// The branchy fixture has ~16 branches per input byte plus length-based
	// branches, so mutation should discover many new edges and corpus
	// entries within a short campaign.
	ws := framework.NewWorkspace(t, "branchy")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "10s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	seedCount := len(framework.CorpusFiles(t, ws, "seeds"))

	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--timeout", "1s")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !res.Stats.Found {
		t.Fatal("missing 'Campaign finished' line in stdout")
	}

	// branchy has enough internal branches that the fuzzer should discover
	// at least 5 new paths beyond the seeds in a 10s run.
	if got := res.Stats.Corpus; got < seedCount+5 {
		t.Errorf("expected corpus >= seeds+5 (>= %d), got %d", seedCount+5, got)
	}

	// Edge count should grow monotonically across the run; assert the last
	// tick has more edges than the first.
	if len(res.Ticks) < 2 {
		t.Fatalf("expected multiple ticks during a 10s run, got %d", len(res.Ticks))
	}
	first, last := res.Ticks[0], res.Ticks[len(res.Ticks)-1]
	if last.Coverage <= first.Coverage {
		t.Errorf("expected edge count to grow across ticks; first=%d last=%d", first.Coverage, last.Coverage)
	}
}

func TestCoverage_WarmupReducesFlakiness(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// This complements the per-harness stability tests: assert specifically
	// that --warmup actually masks flaky bitmap slots by counting the
	// "warmup masked N/M flaky slots" message on stderr.
	ws := framework.NewWorkspace(t, "branchy")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--warmup", "20")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	// The Go harness prints "crossfuzz: coverage warmup masked N/M flaky
	// slots" to stderr after warmup. Assert that line appears.
	if !strings.Contains(res.Stderr, "warmup masked") {
		t.Errorf("expected 'warmup masked N/M flaky slots' in stderr after --warmup=20; got stderr:\n%s", res.Stderr)
	}
}

// Parallel variant — workers share the corpus and global coverage bitmap.
// Discovery should still work (and ideally be faster, but we only assert
// correctness here, not speed).
func TestCoverage_DiscoveryGrowsCorpusAndEdges_Parallel(t *testing.T) {
	t.Parallel()
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "branchy")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "10s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	seedCount := len(framework.CorpusFiles(t, ws, "seeds"))

	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--timeout", "1s", "--workers", "4")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if got := res.Stats.Corpus; got < seedCount+5 {
		t.Errorf("expected corpus >= seeds+5 (>= %d) with --workers=4, got %d", seedCount+5, got)
	}
	if len(res.Ticks) < 2 {
		t.Fatalf("expected multiple ticks during a 10s run, got %d", len(res.Ticks))
	}
	first, last := res.Ticks[0], res.Ticks[len(res.Ticks)-1]
	if last.Coverage <= first.Coverage {
		t.Errorf("expected edge count to grow across ticks with --workers=4; first=%d last=%d", first.Coverage, last.Coverage)
	}
}
