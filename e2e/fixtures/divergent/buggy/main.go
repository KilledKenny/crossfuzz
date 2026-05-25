package main

import "crossfuzz/harness/go"

// Intentional bug: XORs with 0x55 for every byte except when the input length
// is exactly 3, where it XORs with 0x54. The fuzzer must discover the
// 3-byte-input class to trigger a divergence — exercises that the comparator
// + findings pipeline reports differential bugs.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	mask := byte(0x55)
	if len(data) == 3 {
		mask = 0x54
	}
	for i, b := range data {
		out[i] = b ^ mask
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
