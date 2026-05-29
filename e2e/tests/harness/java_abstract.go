package harness

import (
	"time"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
)

func init() {
	framework.Register(framework.Test{
		Name: "harness.java.AbstractAndNativeMethods",
		Tags: []string{"harness", "java"},
		Func: testJavaAbstractAndNativeMethods,
	})
}

// testJavaAbstractAndNativeMethods is a regression test for the bug where
// CoverageTransformer.instrumentMethod() unconditionally inserted probe
// instructions into abstract and native methods. Adding any instruction to
// such a method creates a Code attribute, which JVMS §4.7.3 forbids; the JVM
// rejects the instrumented class bytes with ClassFormatError at defineClass
// time — after transform() has already returned, so the catch in transform()
// cannot recover — crashing the harness before it sends its first "ready"
// message.
//
// The test uses a fuzz target whose inner class hierarchy includes an abstract
// class (AbstractMethodsTarget$Processor). If the guard in instrumentMethod is
// missing, the harness crashes at startup and the campaign never produces a
// stats tick. A non-zero exit code or empty tick list is the regression signal.
func testJavaAbstractAndNativeMethods(t *framework.T) {
	framework.RequireCrossfuzzBinary(t)
	framework.RequireJava(t)
	framework.RequireBinary(t, "javac")
	framework.RequireJavaHarness(t)

	ws := framework.NewWorkspace(t, "java_abstract_methods")
	ws.RenderConfig(t, map[string]any{
		"ExecTimeout":     "1s",
		"CampaignTimeout": "30s",
	})

	if r := framework.Build(t, ws); r.ExitCode != 0 {
		t.Fatalf("build failed (exit %d)\nstdout:\n%s\nstderr:\n%s", r.ExitCode, r.Stdout, r.Stderr)
	}

	res := framework.RunWithTimeout(t, ws, 60*time.Second,
		"--timeout", "5s",
		"--max-findings", "9999",
		"--stop-after", "50",
	)
	if res.ExitCode != 0 {
		t.Fatalf("run failed (exit %d) — harness likely crashed at startup with ClassFormatError\nstdout:\n%s\nstderr:\n%s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
	if len(res.Ticks) == 0 {
		t.Fatal("no stats ticks observed — harness likely crashed before sending 'ready'")
	}
	// Coverage > 0 confirms the concrete methods (ReverseProcessor.process,
	// ReverseProcessor.describe) are still instrumented after the fix; the
	// abstract methods are simply skipped, not all instrumentation disabled.
	if last := res.Ticks[len(res.Ticks)-1]; last.Coverage == 0 {
		t.Errorf("expected coverage > 0 after instrumentation; got 0 — concrete methods may not be instrumented")
	}
}
