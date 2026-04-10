// Package gofuzz provides the cross_fuzz harness for Go fuzz targets.
//
// Usage:
//
//	func target(data []byte) ([]byte, error) { ... }
//	func main() { gofuzz.Run(target) }
//
// Build the target with Go's coverage instrumentation enabled. If you
// only care about code inside your own module, `-coverpkg=./...` is
// enough; if the target delegates into stdlib or third-party packages
// (e.g. encoding/base64, encoding/json, a vendored parser) you MUST use
// `-coverpkg=all`, otherwise those packages are not instrumented and
// the fuzzer will see a constant bitmap regardless of input:
//
//	go build -cover -covermode=atomic -coverpkg=all -o target ./cmd/target
package gofuzz

import (
	"bytes"
	"fmt"
	"io"
	"os"
	rtcov "runtime/coverage"

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

	var collector covCollector
	collector.init(shm.Coverage())

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

			// Tighten the counter window: clear counters *immediately*
			// before calling target so that protocol/IO code executed
			// between iterations is not attributed to this input. On
			// the first real Fuzz call we also build the noise mask
			// (see covCollector.warmup).
			if collector.enabled {
				if !collector.warmedUp {
					collector.warmup(target, input)
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

// covCollector turns runtime/coverage counter snapshots into the 64 KB
// shmem coverage bitmap the coordinator reads. The coordinator zeros the
// bitmap before every iteration (see pkg/runner/process.go), so snapshot
// starts each call from a clean slate and only needs to OR in the new
// data.
//
// noiseMask holds slots that have proven flaky during startup warmup:
// runtime/GC/allocator paths whose counters vary between identical
// target invocations. Any bit set in noiseMask is cleared from every
// snapshot so those paths never cause a bogus "new coverage" event on
// the coordinator.
type covCollector struct {
	buf       bytes.Buffer
	reader    covReader
	enabled   bool
	bitmap    []byte
	noiseMask [65536]byte
	warmedUp  bool
}

// init probes runtime/coverage to decide whether this binary was built
// with `-cover -covermode=atomic`. On failure the harness stays
// functional (Phase 1 behaviour, zero coverage signal) and prints a
// single warning so the user knows to rebuild.
func (c *covCollector) init(bitmap []byte) {
	if err := rtcov.WriteCounters(io.Discard); err != nil {
		fmt.Fprintf(os.Stderr,
			"crossfuzz: coverage disabled for this Go target: %v\n"+
				"           rebuild with `go build -cover -covermode=atomic`\n",
			err)
		return
	}
	if err := rtcov.ClearCounters(); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: ClearCounters probe failed: %v\n", err)
		return
	}
	c.enabled = true
	c.bitmap = bitmap
}

// snapshot captures the current counter state, hashes every
// (pkgID, funcID, counterIdx) tuple into a 16-bit bitmap slot and
// stores a saturating 8-bit counter value (taking the max across
// collisions). Counters are NOT cleared here — clearing happens right
// before the next target() call so that protocol/IO code executed
// between iterations is not attributed to the target. Bucketization
// into powers of two happens later in the coordinator
// (coverage.Bucketize).
func (c *covCollector) snapshot() error {
	if err := c.fill(c.bitmap); err != nil {
		return err
	}
	for i, m := range c.noiseMask {
		if m != 0 {
			c.bitmap[i] = 0
		}
	}
	return nil
}

// fill parses one WriteCounters stream into the given bitmap buffer.
// Assumes buf is already zeroed by the caller.
func (c *covCollector) fill(bitmap []byte) error {
	c.buf.Reset()
	if err := rtcov.WriteCounters(&c.buf); err != nil {
		return fmt.Errorf("WriteCounters: %w", err)
	}
	funcs, err := c.reader.parse(c.buf.Bytes())
	if err != nil {
		return fmt.Errorf("parse covcounters: %w", err)
	}
	for _, f := range funcs {
		pkg := uint64(f.pkgID)
		fn := uint64(f.funcID)
		for i, v := range f.counters {
			key := pkg*0x9E3779B97F4A7C15 +
				fn*0xBF58476D1CE4E5B9 +
				uint64(i)*0x94D049BB133111EB
			idx := uint16(key ^ (key >> 32))

			val := v
			if val > 255 {
				val = 255
			}
			if byte(val) > bitmap[idx] {
				bitmap[idx] = byte(val)
			}
		}
	}
	return nil
}

// warmup runs target repeatedly on a sample input to discover which
// bitmap slots are non-deterministic across identical invocations
// (typically GC/allocator/scheduler paths instrumented by
// -coverpkg=all). Those slots get masked from every subsequent
// snapshot so they cannot produce spurious "new coverage" events.
//
// A panic inside target during warmup is recovered and coverage is
// simply kept unmasked — the campaign will be noisier but still
// functional.
func (c *covCollector) warmup(target TargetFunc, sample []byte) {
	const iterations = 200
	c.warmedUp = true

	var first [65536]byte
	var scratch [65536]byte

	safeRun := func() (ok bool) {
		defer func() {
			if r := recover(); r != nil {
				ok = false
			}
		}()
		_ = rtcov.ClearCounters()
		_, _ = target(sample)
		return true
	}

	if !safeRun() {
		fmt.Fprintln(os.Stderr,
			"crossfuzz: coverage warmup skipped (target panicked on sample input)")
		return
	}
	if err := c.fill(first[:]); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: coverage warmup snapshot: %v\n", err)
		return
	}

	for k := 1; k < iterations; k++ {
		if !safeRun() {
			break
		}
		for i := range scratch {
			scratch[i] = 0
		}
		if err := c.fill(scratch[:]); err != nil {
			return
		}
		for i := range scratch {
			if scratch[i] != first[i] {
				c.noiseMask[i] = 0xFF
			}
		}
	}

	noisy := 0
	for _, v := range c.noiseMask {
		if v != 0 {
			noisy++
		}
	}
	fmt.Fprintf(os.Stderr,
		"crossfuzz: coverage warmup masked %d/%d flaky slots\n",
		noisy, len(c.noiseMask))
}
