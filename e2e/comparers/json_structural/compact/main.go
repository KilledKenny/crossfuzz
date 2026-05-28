package main

import (
	"fmt"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

// Emits a compact JSON object with keys in order x, y.
func target(data []byte) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"x":%d,"y":%d}`, len(data), sum(data))), nil
}

func sum(data []byte) int {
	s := 0
	for _, b := range data {
		s += int(b)
	}
	return s
}

func main() { crossfuzz.Fuzz(target) }
