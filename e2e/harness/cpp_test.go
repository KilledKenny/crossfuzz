//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var cppCase = langCase{
	Flag:         "Cpp",
	TargetName:   "cpp_echo",
	ArtifactPath: "cpp/cpp_echo",
	RequireToolchain: func(t *testing.T) {
		framework.RequireClang19(t)
		framework.RequireBinary(t, "clang++-19")
	},
}

func TestCppHarness_Build(t *testing.T)                       { runBuildTest(t, cppCase) }
func TestCppHarness_PathDiscoveryAndAgreement(t *testing.T)   { runPathDiscoveryAndAgreementTest(t, cppCase) }
func TestCppHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, cppCase)
}
