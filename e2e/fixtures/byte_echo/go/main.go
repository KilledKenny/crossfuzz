package main

import (
	"crossfuzz/harness/go"
)

// target echoes the input. The byte-category switch is there only so the
// fuzzer has a few branches to discover — without it, coverage would saturate
// on the first input and the corpus would never grow, defeating the
// "paths discovered" assertion in the e2e harness tests.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		switch {
		case b < 0x20:
			out[i] = b
		case b < 0x40:
			out[i] = b
		case b < 0x60:
			out[i] = b
		case b < 0x80:
			out[i] = b
		default:
			out[i] = b
		}
	}
	return out, nil
}

func main() {
	crossfuzz.Fuzz(target)
}
