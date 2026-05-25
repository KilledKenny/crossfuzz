package harness

import "crossfuzz/e2e/framework"

var pythonCase = langCase{
	Tag:              "python",
	Flag:             "Python",
	TargetName:       "python_echo",
	ArtifactPath:     "",
	RequireToolchain: func(t *framework.T) { framework.RequirePythonVenv(t) },
}

func init() { register(pythonCase) }
