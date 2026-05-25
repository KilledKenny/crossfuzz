package harness

import "crossfuzz/e2e/framework"

var cCase = langCase{
	Tag:              "c",
	Flag:             "C",
	TargetName:       "c_echo",
	ArtifactPath:     "c/c_echo",
	RequireToolchain: func(t *framework.T) { framework.RequireClang19(t) },
}

func init() { register(cCase) }
