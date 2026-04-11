// Package gofilter provides the cross_fuzz harness for Go input filter programs.
//
// An input filter is an external process that decides whether a generated input
// should be sent to the fuzz targets. It receives the candidate input via shared
// memory and responds with accept or reject over the pipe protocol.
//
// Usage:
//
//	func filter(input []byte) bool {
//	    // return true to accept, false to reject
//	    return bytes.Contains(input, []byte("://"))
//	}
//	func main() { gofilter.Run(filter) }
//
// Build the filter as a plain Go binary (no coverage flags needed):
//
//	go build -o url_filter ./filter
package gofilter

import (
	"fmt"
	"os"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// Run starts the filter event loop. fn is called for each candidate input;
// it should return true to accept (forward to targets) or false to reject (skip).
func Run(fn func(input []byte) bool) {
	shmPath := os.Getenv("CROSSFUZZ_SHM")
	if shmPath == "" {
		fmt.Fprintln(os.Stderr, "crossfuzz filter: CROSSFUZZ_SHM not set")
		os.Exit(1)
	}

	shm, err := coverage.Open(shmPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz filter: open shm: %v\n", err)
		os.Exit(1)
	}
	defer shm.Close()

	cmdR := os.NewFile(3, "cmd")
	respW := os.NewFile(4, "resp")

	if err := protocol.Encode(respW, &protocol.Message{Type: protocol.TypeReady}); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz filter: send ready: %v\n", err)
		os.Exit(1)
	}

	for {
		msg, err := protocol.Decode(cmdR)
		if err != nil {
			return
		}
		switch msg.Type {
		case protocol.TypeFilter:
			input := shm.ReadInput()
			accept := fn(input)
			if err := protocol.Encode(respW, &protocol.Message{
				Type: protocol.TypeFilterResult,
				OK:   accept,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "crossfuzz filter: send result: %v\n", err)
				return
			}
		case protocol.TypeShutdown:
			return
		}
	}
}
