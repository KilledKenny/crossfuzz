//go:build e2e

package harness_test

import (
	"testing"

	"crossfuzz/e2e/framework"
)

var rustCase = langCase{
	Flag:         "Rust",
	TargetName:   "rust_echo",
	ArtifactPath: "rust/target/release/rust_echo",
	RequireToolchain: func(t *testing.T) {
		framework.RequireCargo(t)
		framework.RequireRustHarness(t)
	},
}

func TestRustHarness_Build(t *testing.T)                       { runBuildTest(t, rustCase) }
func TestRustHarness_PathDiscoveryAndAgreement(t *testing.T)   { runPathDiscoveryAndAgreementTest(t, rustCase) }
func TestRustHarness_CoverageStabilityAfterWarmup(t *testing.T) {
	runCoverageStabilityTest(t, rustCase)
}
