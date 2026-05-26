package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"crossfuzz/e2e/framework"
)

// register all CLI flag tests. Each test focuses on one flag's wiring.
func init() {
	r := func(name string, tags []string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "cli." + name,
			Tags: append([]string{"cli"}, tags...),
			Func: fn,
		})
	}
	r("NameFilter_NoMatch", nil, testNameFilterNoMatch)
	r("NameFilter_Match", nil, testNameFilterMatch)
	r("MaxFindings", nil, testMaxFindings)
	r("WarmupAnnounced", nil, testWarmupAnnounced)
	r("DebugEdge", nil, testDebugEdge)
	r("LogFile", nil, testLogFile)
	r("Workers", []string{"parallel"}, testWorkers)
	r("CustomCorpusAndFindingsDirs", nil, testCustomCorpusAndFindingsDirs)
	r("PerExecTimeoutTriggersFinding", nil, testPerExecTimeoutTriggersFinding)
	r("Validate", nil, testValidate)
	r("MaxMemory_CrashesTargetOnOverflow", nil, testMaxMemory)
	r("RootBuildFlag", nil, testRootBuildFlag)
	r("StopAfter_Execs", nil, testStopAfterExecs)
	r("StopAfter_Execs_Parallel", []string{"parallel"}, testStopAfterExecsParallel)
	r("StopAfter_Duration", nil, testStopAfterDuration)
	r("StopAfter_InvalidArg", nil, testStopAfterInvalid)
}

// buildGoByteEcho is the shared "fast Go-only build" used by most CLI flag
// tests. It returns a freshly built workspace ready to `run` against.
func buildGoByteEcho(t *framework.T, extra map[string]any) *framework.Workspace {
	t.Helper()
	ws := framework.NewWorkspace(t, "byte_echo")
	vars := map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "5s",
	}
	for k, v := range extra {
		vars[k] = v
	}
	ws.RenderConfig(t, vars)
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	return ws
}

func testNameFilterNoMatch(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 10*time.Second, "--name", "does_not_exist")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit when --name matches no targets, got 0")
	}
	if !strings.Contains(res.Stderr, "no targets matched") {
		t.Errorf("expected 'no targets matched' in stderr, got:\n%s", res.Stderr)
	}
}

func testNameFilterMatch(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--name", "go_echo")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !res.Stats.Found {
		t.Errorf("expected campaign to complete with --name=go_echo")
	}
}

func testMaxFindings(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// divergent fixture: 3-byte inputs diverge, so findings appear quickly.
	ws := framework.NewWorkspace(t, "divergent")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "30s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 60*time.Second, "--max-findings", "3")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings < 1 {
		t.Errorf("expected >=1 finding from divergent fixture, got %d", res.Stats.Findings)
	}
	if res.Stats.Execs > 50000 {
		t.Errorf("expected --max-findings to stop early; got %d execs (campaign did not exit on findings cap)", res.Stats.Execs)
	}
}

func testWarmupAnnounced(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--warmup", "5")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Warmup: running") {
		t.Errorf("expected 'Warmup: running' in stdout when --warmup > 0, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "Warmup complete.") {
		t.Errorf("expected 'Warmup complete.' in stdout, got:\n%s", res.Stdout)
	}
}

func testDebugEdge(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--debug-edge")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if len(res.Ticks) == 0 {
		t.Fatal("no ticks observed")
	}
	last := res.Ticks[len(res.Ticks)-1]
	if _, ok := last.TargetEdges["go_echo"]; !ok {
		t.Errorf("expected per-target edge count for go_echo in --debug-edge ticker; got TargetEdges=%v", last.TargetEdges)
	}
}

func testLogFile(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	logName := "run.log"
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--log-file", logName)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	logPath := filepath.Join(ws.Dir, logName)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected log file at %s: %v", logPath, err)
	}
	if !strings.Contains(string(data), "Campaign finished.") {
		t.Errorf("expected log file to contain campaign output, got %d bytes:\n%s", len(data), string(data))
	}
}

func testWorkers(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, map[string]any{"CampaignTimeout": "6s"})
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--workers", "3")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "with 3 worker(s)") {
		t.Errorf("expected 'with 3 worker(s)' in stdout, got:\n%s", res.Stdout)
	}
}

func testCustomCorpusAndFindingsDirs(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second,
		"--corpus", "custom-corpus",
		"--findings", "custom-findings",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "custom-corpus")); err != nil {
		t.Errorf("expected custom-corpus dir to exist: %v", err)
	}
}

func testPerExecTimeoutTriggersFinding(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "slow")
	ws.RenderConfig(t, map[string]any{
		"CampaignTimeout": "10s",
		"ExecTimeout":     "200ms",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--timeout", "200ms")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Timeouts == 0 {
		t.Errorf("expected at least one timeout, got 0 — per-exec --timeout may not be wired through")
	}
}

func testValidate(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--validate", "2")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Findings != 0 {
		t.Errorf("--validate 2 reported findings on deterministic echo: %d", res.Stats.Findings)
	}
}

func testMaxMemory(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireClang19(t)

	// memhog allocates 512 MiB on 'M'; --max-memory=128M makes the malloc fail.
	ws := framework.NewWorkspace(t, "memhog")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "2s",
		"CampaignTimeout": "15s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.RunWithTimeout(t, ws, 45*time.Second,
		"--max-memory", "128M",
		"--max-findings", "3",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Crashes == 0 {
		t.Errorf("expected >=1 crash from memhog under --max-memory=128M; got 0\nstdout:\n%s", res.Stdout)
	}
}

func testStopAfterExecs(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// With --stop-after=50 and a single worker, total execs should land at
	// roughly 50 (some tolerance for already-in-flight iterations and the
	// fact that the cap is checked after the iteration's full bookkeeping).
	ws := buildGoByteEcho(t, map[string]any{"CampaignTimeout": "60s"})
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--stop-after", "50")
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if res.Stats.Execs < 40 || res.Stats.Execs > 80 {
		t.Errorf("expected execs ≈ 50 with --stop-after=50, got %d", res.Stats.Execs)
	}
}

func testStopAfterExecsParallel(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	// --stop-after is per-worker (no shared counter), so --workers=4
	// --stop-after=25 should yield about 4×25 = 100 execs total.
	ws := buildGoByteEcho(t, map[string]any{"CampaignTimeout": "60s"})
	res := framework.RunWithTimeout(t, ws, 30*time.Second,
		"--workers", "4",
		"--stop-after", "25",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	// Expect 4*25=100 with some tolerance.
	if res.Stats.Execs < 80 || res.Stats.Execs > 140 {
		t.Errorf("expected execs ≈ 100 with --workers=4 --stop-after=25 (per-worker), got %d", res.Stats.Execs)
	}
}

func testStopAfterDuration(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, map[string]any{"CampaignTimeout": "60s"})
	start := time.Now()
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--stop-after", "2s")
	elapsed := time.Since(start)
	if res.ExitCode != 0 {
		t.Fatalf("run failed: %s\n%s", res.Stdout, res.Stderr)
	}
	// Wall-clock should be close to 2s plus some startup + shutdown overhead.
	if elapsed > 10*time.Second {
		t.Errorf("expected --stop-after=2s to terminate quickly; took %s", elapsed)
	}
	if !res.Stats.Found {
		t.Errorf("expected 'Campaign finished' line after --stop-after duration")
	}
}

func testStopAfterInvalid(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := buildGoByteEcho(t, nil)
	res := framework.RunWithTimeout(t, ws, 10*time.Second, "--stop-after", "garbage")
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit for --stop-after=garbage, got 0")
	}
	if !strings.Contains(res.Stderr, "invalid --stop-after") {
		t.Errorf("expected 'invalid --stop-after' in stderr, got:\n%s", res.Stderr)
	}
}

func testRootBuildFlag(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "5s",
	})
	res := framework.RunWithTimeout(t, ws, 30*time.Second, "--build")
	if res.ExitCode != 0 {
		t.Fatalf("run --build failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Building go_echo") {
		t.Errorf("expected 'Building go_echo' in stdout when --build is set, got:\n%s", res.Stdout)
	}
}
