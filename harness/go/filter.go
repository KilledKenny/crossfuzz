package crossfuzz

import (
	"fmt"
	"os"

	"github.com/KilledKenny/crossfuzz/pkg/protocol"
)

// Filter enters the persistent-mode filter loop. The target function receives
// the candidate input and returns the (possibly transformed) output bytes and
// a boolean indicating whether the input is accepted.
//
// When Settings.Transform is false the returned bytes are ignored and accepted
// inputs are copied to the output region as-is. When Transform is true the
// returned bytes are written to the output region for the coordinator to use
// as the transformed input.
func Filter(target func([]byte) ([]byte, bool), opts ...Settings) {
	settings := mergeSettings(opts)

	shm, err := OpenSHM()
	if err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: %v\n", err)
		os.Exit(1)
	}
	defer shm.Close()

	cmdR, respW := openPipes()
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
			return
		}

		switch msg.Type {
		case protocol.TypeShutdown:
			return

		case protocol.TypeFilter:
			input := shm.ReadInput()
			output, accepted := target(input)

			if accepted {
				if settings.Transform && output != nil {
					shm.WriteOutput(output)
				} else {
					// Copy input to output region so coordinator can read it.
					shm.WriteOutput(input)
				}
			} else {
				shm.SetOutputLen(0)
			}

			if err := protocol.Encode(respW, &protocol.Message{
				Type: protocol.TypeFilterResult,
				OK:   accepted,
			}); err != nil {
				return
			}
		}
	}
}
