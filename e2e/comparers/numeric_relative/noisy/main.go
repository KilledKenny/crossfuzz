package main

import (
	"fmt"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

// Same value as exact/ scaled by (1 + 1e-12). Relative diff is 1e-12, which
// is well within the default numeric epsilon of 1e-9. byte_equal would
// flag every input.
func target(data []byte) ([]byte, error) {
	val := (1000.0 + float64(len(data))) * (1.0 + 1e-12)
	return []byte(fmt.Sprintf("%.15f", val)), nil
}

func main() { crossfuzz.Fuzz(target) }
