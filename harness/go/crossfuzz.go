// Package crossfuzz provides the cross_fuzz harness for Go targets.
//
// It exposes three entry points — Fuzz, Filter, and Compare — plus standalone
// functions for server-mode targets that manage their own SHM and
// instrumentation lifecycle.
//
// Build fuzz and filter targets with Go's coverage instrumentation:
//
//	go build -cover -covermode=atomic -coverpkg=all -o target ./cmd/target
//
// Compare targets do not need coverage instrumentation.
package crossfuzz

import "os"

// Settings configures harness behaviour. All three entry points (Fuzz, Filter,
// Compare) accept the same Settings; fields that are not relevant for a
// particular mode are silently ignored.
type Settings struct {
	// Instrument enables automatic coverage instrumentation. Set to false when
	// the harness is a thin shim and coverage comes from an instrumented server.
	// Default: true.
	Instrument bool

	// Warmup runs the first input this many times before the main loop to
	// discover and mask flaky coverage slots (GC/allocator noise).
	// Default: 0 (no warmup; the harness still performs its internal warmup).
	Warmup int

	// Transform is only relevant for Filter mode. When true the filter may
	// transform the input; the returned bytes are written to the output region
	// for the coordinator to use instead of the original input. When false the
	// filter's byte output is ignored and accepted inputs are copied as-is.
	// Default: false.
	Transform bool

	// Hinting enables fuzzing hinting support. Currently a no-op placeholder
	// for a future feature.
	// Default: false.
	Hinting bool
}

// DefaultSettings returns a Settings with all fields at their recommended
// defaults.
func DefaultSettings() Settings {
	return Settings{
		Instrument: true,
	}
}

func mergeSettings(opts []Settings) Settings {
	if len(opts) > 0 {
		return opts[0]
	}
	return DefaultSettings()
}

// openPipes returns the coordinator→worker command pipe (fd 3) and the
// worker→coordinator response pipe (fd 4).
func openPipes() (cmdR *os.File, respW *os.File) {
	cmdR = os.NewFile(3, "crossfuzz-cmd")
	respW = os.NewFile(4, "crossfuzz-resp")
	return cmdR, respW
}
