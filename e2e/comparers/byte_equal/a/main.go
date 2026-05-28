package main

import "github.com/KilledKenny/crossfuzz/harness/go"

// Returns input bytes XOR'd with 0x42 — identical behaviour to b/.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ 0x42
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
