package main

import "github.com/KilledKenny/crossfuzz/harness/go"

// Identical to a/ when .Diverge is false; flips the high bit on every byte
// when .Diverge is true, producing byte_equal findings on every non-empty
// input.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	mask := byte(0x42)
	if diverge {
		mask = 0x43
	}
	for i, b := range data {
		out[i] = b ^ mask
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
