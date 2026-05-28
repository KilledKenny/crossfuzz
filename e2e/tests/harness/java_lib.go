package harness

import (
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	framework.Register(framework.Test{
		Name: "harness.java.ThirdPartyLibCoverage",
		Tags: []string{"harness", "java"},
		Func: testJavaThirdPartyLibCoverage,
	})
}

// testJavaThirdPartyLibCoverage verifies that the javaagent instruments
// third-party library code (org.json) loaded by the application classloader.
// If instrumentation is working, the fuzzer discovers new JSON structure paths
// in the library and corpus grows beyond the initial seeds.
func testJavaThirdPartyLibCoverage(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireJava(t)
	framework.RequireGradle(t)
	framework.RequireJavaHarness(t)

	ws := framework.NewWorkspace(t, "java_lib_coverage")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "1s",
		"CampaignTimeout": "30s",
	})

	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed (exit %d)\nstdout:\n%s\nstderr:\n%s", r.ExitCode, r.Stdout, r.Stderr)
	}

	seedCount := len(framework.CorpusFiles(t, ws, "seeds"))
	if seedCount == 0 {
		t.Fatal("fixture must ship with at least one seed")
	}

	res := framework.RunWithTimeout(t, ws, 90*time.Second,
		"--timeout", "10s",
		"--max-findings", "9999",
		"--stop-after", "500",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed (exit %d)\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if len(res.Ticks) == 0 {
		t.Fatal("no stats ticks observed")
	}
	if last := res.Ticks[len(res.Ticks)-1]; last.Coverage < 10 {
		t.Errorf("expected coverage > 10 in final tick, got %d — third-party library code may not be instrumented", last.Coverage)
	}
	if res.Stats.Corpus <= seedCount {
		t.Errorf("expected corpus > %d seeds, got %d — fuzzer found no new paths in library code",
			seedCount, res.Stats.Corpus)
	}
}
