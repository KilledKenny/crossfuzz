package main

import "crossfuzz/harness/go"

// Returns the input bytes unchanged.
func target(data []byte) ([]byte, error) { return data, nil }

func main() { crossfuzz.Fuzz(target) }
