package harness

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"crossfuzz/e2e/framework"
)

// langCase encapsulates everything that varies between per-harness tests.
// All four assertions (build artifact, path discovery, agreement, post-warmup
// stability) are the same shape; only the template flag, target name, and
// expected build artifact change.
type langCase struct {
	// Tag is the lowercase language identifier used both as a test tag and
	// as part of the registered test name (e.g. "go", "c", "java").
	Tag string
	// Flag is the {{if .X}} key in crossfuzz.toml.tmpl (e.g. "Go", "C", "JS").
	Flag string
	// TargetName matches the [[target]] name in the rendered TOML.
	TargetName string
	// ArtifactPath is checked after Build (relative to workspace root).
	// Empty skips the artifact existence check.
	ArtifactPath string
	// RequireToolchain is invoked at the start of each test to t.Skip() when
	// the language toolchain or harness build product is missing.
	RequireToolchain func(t *framework.T)
}

// register adds the four standard harness tests for lc to the global registry.
func register(lc langCase) {
	framework.Register(framework.Test{
		Name: "harness." + lc.Tag + ".Build",
		Tags: []string{"harness", lc.Tag},
		Func: func(t *framework.T) { runBuild(t, lc) },
	})
	framework.Register(framework.Test{
		Name: "harness." + lc.Tag + ".PathDiscoveryAndAgreement",
		Tags: []string{"harness", lc.Tag},
		Func: func(t *framework.T) { runPathDiscoveryAndAgreement(t, lc, 1) },
	})
	framework.Register(framework.Test{
		Name: "harness." + lc.Tag + ".CoverageStabilityAfterWarmup",
		Tags: []string{"harness", lc.Tag, "warmup"},
		Func: func(t *framework.T) { runCoverageStability(t, lc) },
	})
}

// registerParallel adds the parallel variant of path-discovery for lc.
func registerParallel(lc langCase) {
	framework.Register(framework.Test{
		Name: "harness." + lc.Tag + ".PathDiscoveryAndAgreement_Parallel",
		Tags: []string{"harness", lc.Tag, "parallel"},
		Func: func(t *framework.T) { runPathDiscoveryAndAgreement(t, lc, 4) },
	})
}

func (lc langCase) renderWith(t *framework.T, ws *framework.Workspace, extra map[string]any) {
	t.Helper()
	vars := map[string]any{lc.Flag: true}
	for k, v := range extra {
		vars[k] = v
	}
	ws.RenderConfig(t, vars)
}

func runBuild(t *framework.T, lc langCase) {
	framework.RequireCrossfuzzBinary(t)
	lc.RequireToolchain(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	lc.renderWith(t, ws, nil)

	res := framework.Build(t, ws)
	if res.ExitCode != 0 {
		t.Fatalf("build failed (exit %d)\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if lc.ArtifactPath != "" && !workspaceFileExists(ws, lc.ArtifactPath) {
		t.Errorf("expected build artifact %q to exist after build", lc.ArtifactPath)
	}
}

func runPathDiscoveryAndAgreement(t *framework.T, lc langCase, workers int) {
	framework.RequireCrossfuzzBinary(t)
	lc.RequireToolchain(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	lc.renderWith(t, ws, map[string]any{
		"ExecTimeout":     "1s",
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	seedCount := len(framework.CorpusFiles(t, ws, "seeds"))
	if seedCount == 0 {
		t.Fatal("fixture must ship with at least one seed")
	}

	// Per-worker exec cap keeps the test fast and deterministic. 500 execs is
	// well above the threshold required to grow byte_echo's corpus beyond the
	// seed count via mutation, on every language we ship a harness for.
	args := []string{"--timeout", "5s", "--max-findings", "9999", "--stop-after", "500"}
	if workers > 1 {
		args = append(args, "--workers", strconv.Itoa(workers))
	}
	res := framework.RunWithTimeout(t, ws, 60*time.Second, args...)
	if res.ExitCode != 0 {
		t.Fatalf("run failed (exit %d)\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !res.Stats.Found {
		t.Fatalf("missing 'Campaign finished' line in stdout:\n%s", res.Stdout)
	}
	if res.Stats.Corpus <= seedCount {
		t.Errorf("expected corpus > %d seeds, got %d (no new paths discovered)", seedCount, res.Stats.Corpus)
	}
	if len(res.Ticks) == 0 {
		t.Fatal("no stats ticks observed")
	}
	if last := res.Ticks[len(res.Ticks)-1]; last.Coverage == 0 {
		t.Errorf("expected coverage > 0 in final tick, got 0")
	}
	if res.Stats.Findings != 0 {
		t.Errorf("expected 0 findings (echo is identity), got %d", res.Stats.Findings)
	}
	if res.Stats.Crashes != 0 {
		t.Errorf("expected 0 crashes, got %d", res.Stats.Crashes)
	}
	if res.Stats.Timeouts != 0 {
		t.Errorf("expected 0 timeouts, got %d", res.Stats.Timeouts)
	}
}

func runCoverageStability(t *framework.T, lc langCase) {
	framework.RequireCrossfuzzBinary(t)
	lc.RequireToolchain(t)

	runOnce := func() int {
		ws := framework.NewWorkspace(t, "byte_echo")
		lc.renderWith(t, ws, map[string]any{
			"ExecTimeout":     "1s",
			"CampaignTimeout": "30s",
		})
		if r := framework.Build(t, ws); r.ExitCode != 0 {
			t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
		}
		// Fixed exec budget per run makes the cross-run coverage comparison
		// deterministic: both runs execute the same number of mutations from
		// the same seeds (the mutator is reseeded per process so coverage
		// may still drift slightly via the per-language harness's own
		// scheduling, hence the ±2 tolerance further below).
		res := framework.RunWithTimeout(t, ws, 45*time.Second,
			"--timeout", "5s",
			"--warmup", "30",
			"--max-findings", "9999",
			"--stop-after", "300",
		)
		if res.ExitCode != 0 {
			t.Fatalf("run failed (exit %d)\nstderr:\n%s", res.ExitCode, res.Stderr)
		}
		if len(res.Ticks) == 0 {
			t.Fatal("no ticks observed")
		}
		return res.Ticks[len(res.Ticks)-1].Coverage
	}

	cov1 := runOnce()
	cov2 := runOnce()
	const tolerance = 2
	if diff := abs(cov1 - cov2); diff > tolerance {
		t.Errorf("post-warmup coverage flaked across runs: %d vs %d (diff %d > tolerance %d) — warmup may be broken", cov1, cov2, diff, tolerance)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func workspaceFileExists(ws *framework.Workspace, rel string) bool {
	_, err := os.Stat(filepath.Join(ws.Dir, rel))
	return err == nil
}
