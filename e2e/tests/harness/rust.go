package harness

import "crossfuzz/e2e/framework"

var rustCase = langCase{
	Tag:          "rust",
	Flag:         "Rust",
	TargetName:   "rust_echo",
	ArtifactPath: "rust/target/release/rust_echo",
	RequireToolchain: func(t *framework.T) {
		framework.RequireCargo(t)
		framework.RequireRustHarness(t)
	},
}

func init() { register(rustCase) }
