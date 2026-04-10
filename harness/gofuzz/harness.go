// Package gofuzz provides the cross_fuzz harness for Go fuzz targets.
//
// Usage:
//
//	func target(data []byte) ([]byte, error) { ... }
//	func main() { gofuzz.Run(target) }
package gofuzz

import (
	"fmt"
	"os"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// TargetFunc is the signature for a Go fuzz target.
type TargetFunc func(data []byte) ([]byte, error)

// Run enters the persistent-mode harness loop.
// It reads inputs from shared memory, calls target, and writes outputs back.
func Run(target TargetFunc) {
	shmPath := os.Getenv("CROSSFUZZ_SHM")
	if shmPath == "" {
		fmt.Fprintf(os.Stderr, "crossfuzz: CROSSFUZZ_SHM not set\n")
		os.Exit(1)
	}

	shm, err := coverage.Open(shmPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: open shm: %v\n", err)
		os.Exit(1)
	}
	defer shm.Close()

	// fd 3 = commands from coordinator, fd 4 = responses to coordinator.
	cmdR := os.NewFile(3, "crossfuzz-cmd")
	respW := os.NewFile(4, "crossfuzz-resp")
	if cmdR == nil || respW == nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: cannot open protocol pipes (fd 3/4)\n")
		os.Exit(1)
	}
	defer cmdR.Close()
	defer respW.Close()

	// Handshake.
	if err := protocol.Encode(respW, &protocol.Message{Type: protocol.TypeReady}); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: send ready: %v\n", err)
		os.Exit(1)
	}

	for {
		msg, err := protocol.Decode(cmdR)
		if err != nil {
			return // pipe closed
		}

		switch msg.Type {
		case protocol.TypeShutdown:
			return

		case protocol.TypeFuzz:
			input := shm.ReadInput()
			output, targetErr := target(input)

			if output != nil {
				shm.WriteOutput(output)
			} else {
				shm.SetOutputLen(0)
			}

			resp := &protocol.Message{Type: protocol.TypeFuzzResult, OK: true}
			if targetErr != nil {
				resp.OK = false
				resp.Error = targetErr.Error()
				shm.SetStatus(coverage.StatusError)
			} else {
				shm.SetStatus(coverage.StatusOK)
			}

			if err := protocol.Encode(respW, resp); err != nil {
				return
			}
		}
	}
}
