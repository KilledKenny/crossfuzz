//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var pythonCase = langCase{
	Flag:       "Python",
	TargetName: "python_echo",
	// Python has no build artifact — the venv is pre-existing.
	ArtifactPath: "",
	RequireToolchain: func(t *testing.T) {
		framework.RequirePythonVenv(t)
	},
}

func TestPythonHarness_Build(t *testing.T)                       { runBuildTest(t, pythonCase) }
func TestPythonHarness_PathDiscoveryAndAgreement(t *testing.T)   { runPathDiscoveryAndAgreementTest(t, pythonCase) }
func TestPythonHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, pythonCase)
}
