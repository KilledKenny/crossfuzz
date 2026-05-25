package main

import "crossfuzz/harness/go"

// Identity echo. Used as both target_a and target_b; byte_equal always
// agrees on identity outputs, so any finding would indicate a wiring bug.
func target(data []byte) ([]byte, error) { return data, nil }

func main() { crossfuzz.Fuzz(target) }
