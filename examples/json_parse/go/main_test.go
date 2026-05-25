package main

import (
	"testing"
)

func FuzzTarget(f *testing.F) {
	f.Add([]byte(`1.7976931348623157e+308
`))
	f.Add([]byte(`[1, 2, 3, "hello", true, null]
`))
	f.Add([]byte(`{"key": "value", "num": 42}
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		target(data)
	})
}
