package main

import (
	"encoding/base64"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

func target(data []byte) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	return []byte(encoded), nil
}

func main() {
	crossfuzz.Fuzz(target)
}
