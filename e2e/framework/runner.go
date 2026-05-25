//go:build e2e

package framework

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"testing"
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
// directory and returns the captured result. The test fails on exec errors
// (binary missing, etc.) but a non-zero exit code is returned, not failed —
// callers can assert on ExitCode.
func Run(t *testing.T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "run", args)
}

// Build invokes `bin/crossfuzz build <ws.ConfigPath>`.
func Build(t *testing.T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "build", args)
}

// Reduce invokes `bin/crossfuzz reduce ...`.
func Reduce(t *testing.T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "reduce", args)
}

// Analyze invokes `bin/crossfuzz analyze ...`.
func Analyze(t *testing.T, ws *Workspace, args ...string) RunResult {
	t.Helper()
	return runSubcommand(t, ws, "analyze", args)
}

// RunWithTimeout is like Run but kills the process if it exceeds wall.
func RunWithTimeout(t *testing.T, ws *Workspace, wall time.Duration, args ...string) RunResult {
	t.Helper()
	return runSubcommandCtx(t, ws, "run", args, wall)
}

func runSubcommand(t *testing.T, ws *Workspace, sub string, args []string) RunResult {
	return runSubcommandCtx(t, ws, sub, args, 5*time.Minute)
}

func runSubcommandCtx(t *testing.T, ws *Workspace, sub string, args []string, wall time.Duration) RunResult {
	t.Helper()
	bin := filepath.Join(ws.RepoRoot, "bin", "crossfuzz")

	ctx, cancel := context.WithTimeout(context.Background(), wall)
	defer cancel()

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
