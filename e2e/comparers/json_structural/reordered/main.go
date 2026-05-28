package main

import (
	"fmt"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

// Emits the same JSON object as compact/ but with keys in order y, x and
// extra whitespace. byte_equal would flag every input — json_structural
// must not, because the parsed structure is identical.
func target(data []byte) ([]byte, error) {
	return []byte(fmt.Sprintf(`{ "y": %d , "x": %d }`, sum(data), len(data))), nil
}

func sum(data []byte) int {
	s := 0
	for _, b := range data {
		s += int(b)
	}
	return s
}

func main() { crossfuzz.Fuzz(target) }
