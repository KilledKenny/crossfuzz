package main

import (
	"testing"
)

func FuzzTarget(f *testing.F) {
	f.Add([]byte("Hello, World!"))
	f.Add([]byte("The quick brown fox jumps over the lazy dog."))

	f.Fuzz(func(t *testing.T, data []byte) {
		target(data)
	})
}
