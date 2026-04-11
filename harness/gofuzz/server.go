package gofuzz

import (
	"fmt"
	"os"
	rtcov "runtime/coverage"

	"crossfuzz/pkg/coverage"
)

// server-mode global state (one shmem region per process).
var srv struct {
	shm       *coverage.SharedMem
	collector covCollector
}

// InitServer opens the shared memory region advertised via CROSSFUZZ_SHM and
// prepares the coverage collector. Call this once from main() in a server
// target before starting to serve requests.
//
// If CROSSFUZZ_SHM is not set (e.g. running outside a fuzzing campaign)
// InitServer is a no-op and all subsequent Clear/Collect calls are no-ops too.
func InitServer() {
	shmPath := os.Getenv("CROSSFUZZ_SHM")
	if shmPath == "" {
		return
	}
	shm, err := coverage.Open(shmPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: open shm %s: %v\n", shmPath, err)
		return
	}
	srv.shm = shm
	srv.collector.init(shm.Coverage())
}

// Clear zeroes the Go runtime coverage counters. Call this immediately before
// the code path you want to observe so that only edges from that path are
// attributed to the current fuzzing iteration.
func Clear() {
	if !srv.collector.enabled {
		return
	}
	_ = rtcov.ClearCounters()
}

// Collect snapshots the current runtime coverage counters into the shared
// memory bitmap. The coordinator reads this bitmap after the harness signals
// completion for the iteration. Call this after the code path you want to
// capture.
func Collect() {
	if !srv.collector.enabled {
		return
	}
	if err := srv.collector.snapshot(); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: coverage snapshot: %v\n", err)
	}
}
