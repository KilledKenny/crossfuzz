package main

import (
	"bytes"
	"regexp"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

var regUrl = regexp.MustCompile(`^[a-zA-Z]+://[a-zA-Z0-9]`)

// filter accepts inputs that look like plausible URL candidates.
// Rejected inputs are discarded without being sent to any fuzz target,
// which focuses the campaign on structurally interesting inputs and
// speeds up coverage growth on URL parsing code paths.
func filter(input []byte) ([]byte, bool) {
	// Must contain "://" to have any chance of being parsed as a URL.
	if !bytes.Contains(input, []byte("://")) {
		return nil, false
	}
	// Reject purely binary inputs — URL parsers operate on printable ASCII.
	for _, b := range input {
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			return nil, false
		}
	}

	return nil, !regUrl.Match(input)
}

func main() {
	crossfuzz.Filter(filter)
}
