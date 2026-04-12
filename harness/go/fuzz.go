package crossfuzz

import (
	"fmt"
	"os"
	rtcov "runtime/coverage"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// Fuzz enters the persistent-mode fuzzing loop. It reads inputs from shared
// memory, calls target, and writes outputs back. The target function receives
// the fuzz input and returns the output bytes and an optional error.
func Fuzz(target func([]byte) ([]byte, error), opts ...Settings) {
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

	var collector covCollector
	if settings.Instrument {
		collector.initCollector(shm.Coverage())
	}

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

		case protocol.TypeFuzz:
			input := shm.ReadInput()

			if collector.enabled {
				if !collector.warmedUp {
					warmupIter := settings.Warmup
					if warmupIter < 2 {
						warmupIter = 200
					}
					collector.warmup(func() bool {
						defer func() { recover() }()
						_ = rtcov.ClearCounters()
						_, _ = target(input)
						return true
					}, warmupIter)
				}
				_ = rtcov.ClearCounters()
			}

			output, targetErr := target(input)

			if collector.enabled {
				if err := collector.snapshot(); err != nil {
					collector.enabled = false
					fmt.Fprintf(os.Stderr, "crossfuzz: coverage disabled: %v\n", err)
				}
			}

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
