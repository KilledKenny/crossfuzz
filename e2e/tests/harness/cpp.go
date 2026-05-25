package harness

import "crossfuzz/e2e/framework"

var cppCase = langCase{
	Tag:          "cpp",
	Flag:         "Cpp",
	TargetName:   "cpp_echo",
	ArtifactPath: "cpp/cpp_echo",
	RequireToolchain: func(t *framework.T) {
		framework.RequireClang19(t)
		framework.RequireBinary(t, "clang++-19")
	},
}

func init() { register(cppCase) }
