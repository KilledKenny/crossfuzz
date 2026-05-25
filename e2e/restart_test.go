//go:build e2e

package e2e_test

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"crossfuzz/e2e/framework"
)

// seedCountRE extracts the "<N> seed inputs" count from the campaign startup
// banner, which is the size of the corpus right after Load() but before
// fuzzing begins. On restart this is the prior run's corpus + seeds.
var seedCountRE = regexp.MustCompile(`(\d+) seed inputs`)

func parseSeedCount(t *testing.T, stdout string) int {
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

// TestRestart_CorpusIsReloaded runs a campaign, then re-runs it against the
// same workspace and asserts that:
//  1. The second run sees the prior corpus as seeds (≥ what the first run saved).
//  2. Coverage edges do not regress.
//  3. The second run discovers few or no new entries, because the first run
//     already explored most of the byte_echo branch surface.
func TestRestart_CorpusIsReloaded(t *testing.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "8s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}

	// First run: populate corpus + global coverage from scratch.
	first := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if first.ExitCode != 0 {
		t.Fatalf("first run failed: %s\n%s", first.Stdout, first.Stderr)
	}
	if first.Stats.Corpus < 2 {
		t.Fatalf("first run only produced corpus=%d; expected >=2 to make restart meaningful", first.Stats.Corpus)
	}
	if len(first.Ticks) == 0 {
		t.Fatal("first run produced no stats ticks")
	}
	firstCoverage := first.Ticks[len(first.Ticks)-1].Coverage
	firstCorpus := first.Stats.Corpus

	// Second run: same workspace, so seed_dir + corpus_dir are populated
	// from the first run's outputs.
	second := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999")
	if second.ExitCode != 0 {
		t.Fatalf("second run failed: %s\n%s", second.Stdout, second.Stderr)
	}
	if len(second.Ticks) == 0 {
		t.Fatal("second run produced no stats ticks")
	}

	// 1. Corpus was reloaded: the startup banner's seed count should match
	//    what the first run saved (with possible deduplication against the
	//    fixture's seed_dir entries).
	secondSeedCount := parseSeedCount(t, second.Stdout)
	if secondSeedCount < firstCorpus {
		t.Errorf("expected restart to load >= %d seed inputs (first run's corpus), got %d", firstCorpus, secondSeedCount)
	}

	// 2. Coverage does not regress (within the same tolerance the harness
	//    stability test uses for warmup noise).
	secondCoverage := second.Ticks[len(second.Ticks)-1].Coverage
	if secondCoverage+2 < firstCoverage {
		t.Errorf("coverage regressed across restart: first=%d second=%d", firstCoverage, secondCoverage)
	}

	// 3. Second-run corpus growth (over what was already loaded) should be
	//    small, because the byte_echo branch space is mostly exhausted after
	//    the first run. We bound by what the first run discovered.
	newEntries := second.Stats.Corpus - secondSeedCount
	if newEntries < 0 {
		newEntries = 0
	}
	if firstNew := firstCorpus - len(framework.CorpusFiles(t, ws, "seeds")); newEntries > firstNew {
		t.Errorf("second run discovered more new entries (%d) than the first (%d); corpus reuse not effective", newEntries, firstNew)
	}

	t.Log(fmt.Sprintf("first: corpus=%d coverage=%d  |  second: seeded=%d corpus=%d coverage=%d (new=%d)",
		firstCorpus, firstCoverage, secondSeedCount, second.Stats.Corpus, secondCoverage, newEntries))
}

// Parallel variant — multiple workers share the corpus across both runs.
// On restart all workers must observe the reloaded corpus and the shared
// coverage bitmap (cross-worker coverage merge happens via the global mu).
func TestRestart_CorpusIsReloaded_Parallel(t *testing.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "8s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}

	first := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if first.ExitCode != 0 {
		t.Fatalf("first run failed: %s\n%s", first.Stdout, first.Stderr)
	}
	if first.Stats.Corpus < 2 {
		t.Fatalf("first run only produced corpus=%d; expected >=2", first.Stats.Corpus)
	}
	firstCorpus := first.Stats.Corpus

	second := framework.RunWithTimeout(t, ws, 30*time.Second, "--max-findings", "9999", "--workers", "4")
	if second.ExitCode != 0 {
		t.Fatalf("second run failed: %s\n%s", second.Stdout, second.Stderr)
	}
	secondSeedCount := parseSeedCount(t, second.Stdout)
	if secondSeedCount < firstCorpus {
		t.Errorf("expected restart to load >= %d seed inputs from prior corpus, got %d", firstCorpus, secondSeedCount)
	}
}
