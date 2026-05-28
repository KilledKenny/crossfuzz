package main

import (
	"fmt"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

// Emits the byte-sum without any decoration — numerically equal to spaced/'s
// output but byte-different.
func target(data []byte) ([]byte, error) {
	s := 0
	for _, b := range data {
		s += int(b)
	}
	return []byte(fmt.Sprintf("%d", s)), nil
}

func main() { crossfuzz.Fuzz(target) }
