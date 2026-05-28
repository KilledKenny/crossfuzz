package main

import "github.com/KilledKenny/crossfuzz/harness/go"

// Byte-level ASCII lower. See upper/main.go for why bytes.ToLower isn't safe.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + 32
		} else {
			out[i] = b
		}
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
