package main

import (
	"fmt"

	"crossfuzz/harness/go"
)

// Emits the byte-sum surrounded by whitespace. byte_equal would flag every
// input; numeric strips whitespace and parses the value.
func target(data []byte) ([]byte, error) {
	s := 0
	for _, b := range data {
		s += int(b)
	}
	return []byte(fmt.Sprintf("   %d\n", s)), nil
}

func main() { crossfuzz.Fuzz(target) }
