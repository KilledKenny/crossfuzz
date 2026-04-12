package crossfuzz

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// Compare enters the persistent-mode comparator loop. The target function
// receives the fuzz input, an ordered list of target names, and a map of
// target name to output bytes. It returns a string describing any mismatch;
// an empty string means all outputs match.
//
// The comparator process does not get its own CROSSFUZZ_SHM. Instead it reads
// CROSSFUZZ_SHM_TARGETS (a JSON map of target name to SHM file path) and
// opens each target's shared memory to read outputs directly.
func Compare(target func(input []byte, names []string, outputs map[string][]byte) string, opts ...Settings) {
	_ = mergeSettings(opts) // validate; no settings used by compare currently

	// Parse target SHM paths from environment.
	targetsJSON := os.Getenv("CROSSFUZZ_SHM_TARGETS")
	if targetsJSON == "" {
		fmt.Fprintf(os.Stderr, "crossfuzz: CROSSFUZZ_SHM_TARGETS not set\n")
		os.Exit(1)
	}

	var targetPaths map[string]string
	if err := json.Unmarshal([]byte(targetsJSON), &targetPaths); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: parse CROSSFUZZ_SHM_TARGETS: %v\n", err)
		os.Exit(1)
	}

	// mmap each target's SHM file read-only.
	type targetSHM struct {
		data []byte
		fd   int
	}
	targetMaps := make(map[string]targetSHM, len(targetPaths))
	for name, path := range targetPaths {
		f, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "crossfuzz: open target SHM %s (%s): %v\n", name, path, err)
			os.Exit(1)
		}
		data, err := syscall.Mmap(int(f.Fd()), 0, coverage.TotalShmSize,
			syscall.PROT_READ, syscall.MAP_SHARED)
		if err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "crossfuzz: mmap target SHM %s: %v\n", name, err)
			os.Exit(1)
		}
		targetMaps[name] = targetSHM{data: data, fd: int(f.Fd())}
		// Keep file open for the lifetime of the process (mmap requires it).
	}

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

		case protocol.TypeCompare:
			names := msg.Targets
			outputs := make(map[string][]byte, len(names))

			// Read input from the first target's SHM (all targets receive
			// the same input).
			var input []byte
			for _, name := range names {
				ts, ok := targetMaps[name]
				if !ok {
					continue
				}
				if input == nil {
					inputLen := binary.LittleEndian.Uint32(ts.data[coverage.OffInputLen:])
					if inputLen > coverage.InputRegionSize {
						inputLen = coverage.InputRegionSize
					}
					input = make([]byte, inputLen)
					copy(input, ts.data[coverage.InputRegionOffset:coverage.InputRegionOffset+int(inputLen)])
				}
				outputLen := binary.LittleEndian.Uint32(ts.data[coverage.OffOutputLen:])
				if outputLen > coverage.OutputRegionSize {
					outputLen = coverage.OutputRegionSize
				}
				out := make([]byte, outputLen)
				copy(out, ts.data[coverage.OutputRegionOffset:coverage.OutputRegionOffset+int(outputLen)])
				outputs[name] = out
			}

			mismatch := target(input, names, outputs)

			resp := &protocol.Message{Type: protocol.TypeCompareResult}
			if mismatch != "" {
				resp.Error = mismatch
			}
			if err := protocol.Encode(respW, resp); err != nil {
				return
			}
		}
	}
}
