//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var jsCase = langCase{
	Flag:       "JS",
	TargetName: "js_echo",
	// JS has no build_cmd in this fixture — the harness is loaded directly
	// from {{.RepoRoot}}/harness/js, so there is no artifact to verify.
	ArtifactPath: "",
	RequireToolchain: func(t *testing.T) {
		framework.RequireBun(t)
		framework.RequireJSHarness(t)
	},
}

func TestJSHarness_Build(t *testing.T)                       { runBuildTest(t, jsCase) }
func TestJSHarness_PathDiscoveryAndAgreement(t *testing.T)   { runPathDiscoveryAndAgreementTest(t, jsCase) }
func TestJSHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, jsCase)
}
