package restart

import (
	"regexp"
	"strconv"
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	framework.Register(framework.Test{
		Name: "restart.CorpusIsReloaded",
		Tags: []string{"restart"},
		Func: testCorpusIsReloaded,
	})
	framework.Register(framework.Test{
		Name: "restart.CorpusIsReloaded_Parallel",
		Tags: []string{"restart", "parallel"},
		Func: testCorpusIsReloadedParallel,
	})
}

// seedCountRE extracts the "<N> seed inputs" count from the campaign startup
// banner, which is the size of the corpus right after Load() but before
// fuzzing begins. On restart this is the prior run's corpus + seeds.
var seedCountRE = regexp.MustCompile(`(\d+) seed inputs`)

func parseSeedCount(t *framework.T, stdout string) int {
	t.Helper()
	m := seedCountRE.FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("could not parse 'seed inputs' count from stdout:\n%s", stdout)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("seed count %q is not an int: %v", m[1], err)
	}
	return n
}

func testCorpusIsReloaded(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}

	first := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "10s")
	if first.ExitCode != 0 {
		t.Fatalf("first run failed: %s\n%s", first.Stdout, first.Stderr)
	}
	if first.Stats.Corpus < 2 {
		t.Fatalf("first run only produced corpus=%d; expected >=2", first.Stats.Corpus)
	}
	if len(first.Ticks) == 0 {
		t.Fatal("first run produced no stats ticks")
	}
	firstCoverage := first.Ticks[len(first.Ticks)-1].Coverage
	firstCorpus := first.Stats.Corpus

	second := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--stop-after", "100")
	if second.ExitCode != 0 {
		t.Fatalf("second run failed: %s\n%s", second.Stdout, second.Stderr)
	}
	if len(second.Ticks) == 0 {
		t.Fatal("second run produced no stats ticks")
	}

	secondSeedCount := parseSeedCount(t, second.Stdout)
	if secondSeedCount < firstCorpus {
		t.Errorf("expected restart to load >= %d seed inputs (first run's corpus), got %d", firstCorpus, secondSeedCount)
	}

	secondCoverage := second.Ticks[len(second.Ticks)-1].Coverage
	if secondCoverage+2 < firstCoverage {
		t.Errorf("coverage regressed across restart: first=%d second=%d", firstCoverage, secondCoverage)
	}

	newEntries := second.Stats.Corpus - secondSeedCount
	if newEntries < 0 {
		newEntries = 0
	}
	if firstNew := firstCorpus - len(framework.CorpusFiles(t, ws, "seeds")); newEntries > firstNew {
		t.Errorf("second run discovered more new entries (%d) than the first (%d); corpus reuse not effective", newEntries, firstNew)
	}

	t.Logf("first: corpus=%d coverage=%d  |  second: seeded=%d corpus=%d coverage=%d (new=%d)",
		firstCorpus, firstCoverage, secondSeedCount, second.Stats.Corpus, secondCoverage, newEntries)
}

func testCorpusIsReloadedParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}

	first := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "10s")
	if first.ExitCode != 0 {
		t.Fatalf("first run failed: %s\n%s", first.Stdout, first.Stderr)
	}
	if first.Stats.Corpus < 2 {
		t.Fatalf("first run only produced corpus=%d; expected >=2", first.Stats.Corpus)
	}
	firstCorpus := first.Stats.Corpus

	second := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4", "--stop-after", "100")
	if second.ExitCode != 0 {
		t.Fatalf("second run failed: %s\n%s", second.Stdout, second.Stderr)
	}
	secondSeedCount := parseSeedCount(t, second.Stdout)
	if secondSeedCount < firstCorpus {
		t.Errorf("expected restart to load >= %d seed inputs from prior corpus, got %d", firstCorpus, secondSeedCount)
	}
}
