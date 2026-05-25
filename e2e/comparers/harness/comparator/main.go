package main

import (
	"fmt"

	"crossfuzz/harness/go"
)

// compare is invoked by the coordinator for every executed input. It receives
// the input bytes, an ordered list of target names, and a map of target name
// to output bytes (read directly from each target's shared memory). Returning
// an empty string means "no mismatch"; a non-empty string is recorded as the
// finding description.
//
// This comparator only checks that all targets produced outputs of equal
// length, ignoring content. It exercises the harness-comparator pipeline
// (separate process, SHM read-only access to target outputs, pipe-based
// request/response) end-to-end.
func compare(input []byte, names []string, outputs map[string][]byte) string {
	if len(names) < 2 {
		return ""
	}
	refLen := len(outputs[names[0]])
	for _, n := range names[1:] {
		if got := len(outputs[n]); got != refLen {
			return fmt.Sprintf("length mismatch: %s=%d vs %s=%d", names[0], refLen, n, got)
		}
	}
	return ""
}

func main() {
	crossfuzz.Compare(compare)
}
