package main

import "crossfuzz/harness/go"

// Byte-level ASCII upper. Non-ASCII bytes pass through unchanged so the
// transformation is length-preserving and invertible by .lower() in the
// custom comparator script. bytes.ToUpper would do Unicode case folding
// (e.g. ß → SS) which changes byte length and breaks equality.
func target(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		if b >= 'a' && b <= 'z' {
			out[i] = b - 32
		} else {
			out[i] = b
		}
	}
	return out, nil
}

func main() { crossfuzz.Fuzz(target) }
