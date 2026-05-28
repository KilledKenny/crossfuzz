package main

import "github.com/KilledKenny/crossfuzz/harness/go"

// Returns the input bytes reversed — same length, very different content.
// A byte_equal comparator would flag every non-palindrome. The harness
// comparator below only compares lengths, so no findings are expected.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		out[len(data)-1-i] = b
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
