package main

import (
	"fmt"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

func target(data []byte) ([]byte, error) {
	val := 1000.0 + float64(len(data))
	return []byte(fmt.Sprintf("%.15f", val)), nil
}

func main() { crossfuzz.Fuzz(target) }
