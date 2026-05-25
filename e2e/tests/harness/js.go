package harness

import "crossfuzz/e2e/framework"

var jsCase = langCase{
	Tag:        "js",
	Flag:       "JS",
	TargetName: "js_echo",
	// JS has no build_cmd in this fixture — the harness is loaded directly
	// from {{.RepoRoot}}/harness/js, so there is no artifact to verify.
	ArtifactPath: "",
	RequireToolchain: func(t *framework.T) {
		framework.RequireBun(t)
		framework.RequireJSHarness(t)
	},
}

func init() { register(jsCase) }
