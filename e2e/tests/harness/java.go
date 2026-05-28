package harness

import "github.com/KilledKenny/crossfuzz/e2e/framework"

var javaCase = langCase{
	Tag:          "java",
	Flag:         "Java",
	TargetName:   "java_echo",
	ArtifactPath: "java/JavaEcho.class",
	RequireToolchain: func(t *framework.T) {
		framework.RequireJava(t)
		framework.RequireBinary(t, "javac")
		framework.RequireJavaHarness(t)
	},
}

func init() { register(javaCase) }
