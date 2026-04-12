package crossfuzz

import (
	"bytes"
	"fmt"
	"io"
	"os"
	rtcov "runtime/coverage"
)

// covCollector turns runtime/coverage counter snapshots into the 64 KB
// shared-memory coverage bitmap the coordinator reads. noiseMask holds
// slots that have proven flaky during startup warmup (GC/allocator paths);
// any set bit is cleared from every snapshot.
type covCollector struct {
	buf       bytes.Buffer
	reader    covReader
	enabled   bool
	bitmap    []byte
	noiseMask [65536]byte
	warmedUp  bool
}

// initCollector probes runtime/coverage to decide whether this binary was
// built with `-cover -covermode=atomic`. On failure the harness stays
// functional (zero coverage signal) and prints a warning.
func (c *covCollector) initCollector(bitmap []byte) {
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
// (pkgID, funcID, counterIdx) tuple into a 16-bit bitmap slot with a
// saturating 8-bit counter. Counters are NOT cleared here — clearing
// happens right before the next target() call.
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
// bitmap slots are non-deterministic across identical invocations.
func (c *covCollector) warmup(runOnce func() bool, iterations int) {
	if iterations < 2 {
		iterations = 200
	}
	c.warmedUp = true

	var first [65536]byte
	var scratch [65536]byte

	if !runOnce() {
		fmt.Fprintln(os.Stderr,
			"crossfuzz: coverage warmup skipped (target panicked on sample input)")
		return
	}
	if err := c.fill(first[:]); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: coverage warmup snapshot: %v\n", err)
		return
	}

	for k := 1; k < iterations; k++ {
		if !runOnce() {
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

// Standalone instrumentation functions for server-mode targets.

// global state for standalone usage
var standalone struct {
	collector covCollector
}

// StartInstrumentation initializes the coverage collector on the given bitmap
// slice (typically from SharedMem.Coverage()). This must be called after
// OpenSHM.
func StartInstrumentation(bitmap []byte) error {
	standalone.collector.initCollector(bitmap)
	if !standalone.collector.enabled {
		return fmt.Errorf("crossfuzz: instrumentation not available")
	}
	return nil
}

// ClearInstrumentation zeroes the Go runtime coverage counters. Call before
// the code path you want to attribute to the current input.
func ClearInstrumentation() {
	if !standalone.collector.enabled {
		return
	}
	_ = rtcov.ClearCounters()
}

// CollectInstrumentation snapshots the current runtime coverage counters into
// the shared memory bitmap.
func CollectInstrumentation() error {
	if !standalone.collector.enabled {
		return nil
	}
	return standalone.collector.snapshot()
}

// InitServer is a convenience that opens SHM and starts instrumentation in one
// call. If CROSSFUZZ_SHM is not set it is a no-op (for running the server
// outside a fuzzing campaign).
func InitServer() {
	shm, err := OpenSHM()
	if err != nil {
		return
	}
	standalone.collector.initCollector(shm.Coverage())
}

// Clear zeroes runtime coverage counters. No-op if InitServer was not called.
func Clear() {
	ClearInstrumentation()
}

// Collect snapshots coverage into SHM. No-op if InitServer was not called.
func Collect() {
	if err := CollectInstrumentation(); err != nil {
		fmt.Fprintf(os.Stderr, "crossfuzz: coverage snapshot: %v\n", err)
	}
}
