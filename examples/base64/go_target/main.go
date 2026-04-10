package main

import (
	"encoding/base64"

	"crossfuzz/harness/gofuzz"
)

func target(data []byte) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	return []byte(encoded), nil
}

func main() {
	gofuzz.Run(target)
}
