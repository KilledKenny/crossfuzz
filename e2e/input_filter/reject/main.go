package main

import "crossfuzz/harness/go"

// Rejects every input. The coordinator should count all candidate inputs as
// "rejected" in the stats and execute zero of them against the targets.
func filter(input []byte) ([]byte, bool) {
	return nil, false
}

func main() { crossfuzz.Filter(filter) }
