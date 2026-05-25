//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var cCase = langCase{
	Flag:         "C",
	TargetName:   "c_echo",
	ArtifactPath: "c/c_echo",
	RequireToolchain: func(t *testing.T) {
		framework.RequireClang19(t)
	},
}

func TestCHarness_Build(t *testing.T)                       { runBuildTest(t, cCase) }
func TestCHarness_PathDiscoveryAndAgreement(t *testing.T)   { runPathDiscoveryAndAgreementTest(t, cCase) }
func TestCHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, cCase)
}
