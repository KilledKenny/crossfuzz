package main

import (
	"time"

	"crossfuzz/harness/go"
)

// target sleeps forever when the input begins with 'S', producing a timeout
// finding once the fuzzer discovers that input class. Other inputs return
// quickly so the campaign still makes progress.
func target(data []byte) ([]byte, error) {
	if len(data) > 0 && data[0] == 'S' {
		time.Sleep(10 * time.Second)
	}
	return data, nil
}

func main() { crossfuzz.Fuzz(target) }
