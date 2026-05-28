package harness

import "github.com/KilledKenny/crossfuzz/e2e/framework"

var goCase = langCase{
	Tag:              "go",
	Flag:             "Go",
	TargetName:       "go_echo",
	ArtifactPath:     "go/go_echo",
	RequireToolchain: func(t *framework.T) { framework.RequireGo(t) },
}

func init() {
	register(goCase)
	registerParallel(goCase)
}
