//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var javaCase = langCase{
	Flag:         "Java",
	TargetName:   "java_echo",
	ArtifactPath: "java/JavaEcho.class",
	RequireToolchain: func(t *testing.T) {
		framework.RequireJava(t)
		framework.RequireBinary(t, "javac")
		framework.RequireJavaHarness(t)
	},
}

func TestJavaHarness_Build(t *testing.T)                       { runBuildTest(t, javaCase) }
func TestJavaHarness_PathDiscoveryAndAgreement(t *testing.T)   { runPathDiscoveryAndAgreementTest(t, javaCase) }
func TestJavaHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, javaCase)
}
