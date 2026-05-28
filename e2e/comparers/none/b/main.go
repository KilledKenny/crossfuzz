package main

import "github.com/KilledKenny/crossfuzz/harness/go"

// Deliberately returns very different output from a/ — every byte flipped.
// Under the "none" comparator the coordinator should never flag this as a
// divergence.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = ^b
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
