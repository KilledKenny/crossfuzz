package subcommands

import (
	"os"
	"path/filepath"
	"strings"

	"crossfuzz/e2e/framework"
)

func init() {
	r := func(name string, fn func(*framework.T)) {
		framework.Register(framework.Test{
			Name: "subcommand." + name,
			Tags: []string{"subcommand"},
			Func: fn,
		})
	}
	r("Build", testBuild)
	r("Reduce", testReduce)
	r("Analyze_Payload", testAnalyzePayload)
	r("Analyze_RequiresInput", testAnalyzeRequiresInput)
}

func testBuild(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{"Go": true})
	res := framework.Build(t, ws)
	if res.ExitCode != 0 {
		t.Fatalf("build failed (exit %d)\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Build complete.") {
		t.Errorf("expected 'Build complete.' in stdout, got:\n%s", res.Stdout)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "go", "go_echo")); err != nil {
		t.Errorf("expected go/go_echo binary after build: %v", err)
	}
}

func testReduce(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{
		"Go":              true,
		"ExecTimeout":     "500ms",
		"CampaignTimeout": "5s",
	})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	if r := framework.Run(t, ws); r.ExitCode != 0 {
		t.Fatalf("seed-run failed: %s\n%s", r.Stdout, r.Stderr)
	}
	inputCount := len(framework.CorpusFiles(t, ws, "corpus")) + len(framework.CorpusFiles(t, ws, "seeds"))
	if inputCount < 3 {
		t.Skipf("only %d input entries after fuzzing; need >=3 to meaningfully test reduce", inputCount)
	}

	res := framework.Reduce(t, ws)
	if res.ExitCode != 0 {
		t.Fatalf("reduce failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Reduced") {
		t.Errorf("expected 'Reduced N → M entries' summary in stdout, got:\n%s", res.Stdout)
	}
	corpusAfter := len(framework.CorpusFiles(t, ws, "corpus-reduced"))
	if corpusAfter == 0 {
		t.Errorf("expected reduce to keep some entries, got 0")
	}
	if corpusAfter > inputCount {
		t.Errorf("reduced corpus (%d) larger than input set (%d)", corpusAfter, inputCount)
	}
}

func testAnalyzePayload(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{"Go": true})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.Analyze(t, ws, "--payload", "hello")
	if res.ExitCode != 0 {
		t.Fatalf("analyze failed: %s\n%s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Payload:") {
		t.Errorf("expected 'Payload:' header in stdout, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "Target: go_echo") {
		t.Errorf("expected per-target output for go_echo, got:\n%s", res.Stdout)
	}
}

func testAnalyzeRequiresInput(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireGo(t)

	ws := framework.NewWorkspace(t, "byte_echo")
	ws.RenderConfig(t, map[string]any{"Go": true})
	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed: %s\n%s", r.Stdout, r.Stderr)
	}
	res := framework.Analyze(t, ws)
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit when analyze called without --payload or --payload-path")
	}
}
