package framework

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunResult is the captured outcome of one bin/crossfuzz invocation.
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Stats    FinalStats
	Ticks    []TickStats
}

// Run invokes `bin/crossfuzz run <ws.ConfigPath> <args...>` in the workspace
// directory. Exec-level failures (binary missing) fail the test; non-zero
// exit codes are returned in ExitCode for the caller to assert on.
func Run(t *T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "run", args, 5*time.Minute)
}

// Build invokes `bin/crossfuzz build <ws.ConfigPath>`.
func Build(t *T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "build", args, 5*time.Minute)
}

// Reduce invokes `bin/crossfuzz reduce ...`.
func Reduce(t *T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "reduce", args, 5*time.Minute)
}

// Analyze invokes `bin/crossfuzz analyze ...`.
func Analyze(t *T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "analyze", args, 5*time.Minute)
}

// RunWithTimeout is like Run but kills the process if it exceeds wall.
func RunWithTimeout(t *T, ws *Workspace, wall time.Duration, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "run", args, wall)
}

func runSubcommand(t *T, ws *Workspace, sub string, args []string, wall time.Duration) RunResult {
	t.Helper()
	bin := filepath.Join(ws.RepoRoot, "bin", "crossfuzz")

	ctx, cancel := context.WithTimeout(context.Background(), wall)
	defer cancel()

	// All e2e `run` invocations get a deterministic mutator seed by default so
	// cross-run comparisons aren't polluted by mutator divergence. A test that
	// actually wants the wall-clock default can pass --seed 0 explicitly.
	if sub == "run" && !hasFlag(args, "--seed") {
		args = append([]string{"--seed", "1"}, args...)
	}

	all := append([]string{sub, ws.ConfigPath}, args...)
	cmd := exec.CommandContext(ctx, bin, all...)
	cmd.Dir = ws.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("crossfuzz exceeded wall timeout of %s\nstdout:\n%s\nstderr:\n%s", wall, stdout.String(), stderr.String())
		} else {
			t.Fatalf("crossfuzz exec error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
	}

	res := RunResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	res.Stats, res.Ticks = ParseOutput(res.Stdout)
	return res
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}
