package main

import (
	"bytes"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

// Transform filter: accepts every input but rewrites it to a fixed marker
// byte sequence before the coordinator dispatches it to the targets.
// The crossfuzz.Settings{Transform: true} opt-in tells the runner to use the
// returned bytes instead of the original input.
func filter(input []byte) ([]byte, bool) {
	return bytes.Repeat([]byte{'Z'}, 8), true
}

func main() {
	crossfuzz.Filter(filter, crossfuzz.Settings{Transform: true})
}
