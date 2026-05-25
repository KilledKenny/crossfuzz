//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var goCase = langCase{
	Flag:             "Go",
	TargetName:       "go_echo",
	ArtifactPath:     "go/go_echo",
	RequireToolchain: func(t *testing.T) { framework.RequireGo(t) },
}

func TestGoHarness_Build(t *testing.T)                      { runBuildTest(t, goCase) }
func TestGoHarness_PathDiscoveryAndAgreement(t *testing.T)  { runPathDiscoveryAndAgreementTest(t, goCase) }
func TestGoHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, goCase)
}
