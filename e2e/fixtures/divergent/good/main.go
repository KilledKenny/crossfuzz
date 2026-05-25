package main

import "crossfuzz/harness/go"

func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ 0x55
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
