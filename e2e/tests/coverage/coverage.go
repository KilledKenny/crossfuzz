package coverage

import (
	"strings"
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "coverage." + name,
			Tags: append([]string{"coverage"}, tags...),
			Func: fn,
		})
	}
	r("DiscoveryGrowsCorpusAndEdges", nil, testDiscovery)
	r("WarmupReducesFlakiness", nil, testWarmupReducesFlakiness)
	r("DiscoveryGrowsCorpusAndEdges_Parallel", []string{"parallel"}, testDiscoveryParallel)
}

func testDiscovery(t *framework.T) {
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

	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--timeout", "1s")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !res.Stats.Found {
		t.Fatal("missing 'Campaign finished' line in stdout")
	}
	if got := res.Stats.Corpus; got < seedCount+5 {
		t.Errorf("expected corpus >= seeds+5 (>= %d), got %d", seedCount+5, got)
	}
	if len(res.Ticks) < 2 {
		t.Fatalf("expected multiple ticks during a 10s run, got %d", len(res.Ticks))
	}
	first, last := res.Ticks[0], res.Ticks[len(res.Ticks)-1]
	if last.Coverage <= first.Coverage {
		t.Errorf("expected edge count to grow across ticks; first=%d last=%d", first.Coverage, last.Coverage)
	}
}

func testWarmupReducesFlakiness(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

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
	if !strings.Contains(res.Stderr, "warmup masked") {
		t.Errorf("expected 'warmup masked N/M flaky slots' in stderr after --warmup=20; got stderr:\n%s", res.Stderr)
	}
}

func testDiscoveryParallel(t *framework.T) {
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
