package main

import "github.com/KilledKenny/crossfuzz/harness/go"

// Always returns the 8-byte constant "ZZZZZZZZ", regardless of input.
// Combined with target_identity, this diverges from byte_equal on every
// input that is not exactly "ZZZZZZZZ".
func target(_ []byte) ([]byte, error) {
	return []byte("ZZZZZZZZ"), nil
}

func main() { crossfuzz.Fuzz(target) }
