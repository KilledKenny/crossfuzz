package engine

import (
	"github.com/KilledKenny/crossfuzz/pkg/compare"
	"github.com/KilledKenny/crossfuzz/pkg/runner"
)

// Minimize shrinks input to the smallest byte sequence that still triggers
// the given comparator to report a discrepancy when run through targets.
//
// The algorithm has two passes:
//  1. Binary reduction: repeatedly remove chunks of decreasing size.
//  2. Byte-by-byte deletion: remove one byte at a time.
//
// Returns the minimized input (which may equal the original if no reduction
// was possible) and the final discrepancy produced by the minimized input.
func Minimize(input []byte, targets []runner.Runner, comp compare.Comparator) ([]byte, *compare.Discrepancy) {
	current := make([]byte, len(input))
	copy(current, input)

	check := func(candidate []byte) (*compare.Discrepancy, bool) {
		outputs := make(map[string][]byte, len(targets))
		for _, r := range targets {
			out, _, err := r.Execute(candidate)
			if err != nil {
				return nil, false
			}
			outputs[r.Name()] = out
		}
		disc := comp.Compare(candidate, outputs)
		return disc, disc != nil
	}

	// ---- Pass 1: binary reduction ----------------------------------------
	// Try removing chunks of size len/2, len/4, … 1.
	for chunkSize := len(current) / 2; chunkSize >= 1; chunkSize /= 2 {
		offset := 0
		for offset < len(current) {
			end := offset + chunkSize
			if end > len(current) {
				end = len(current)
			}
			// Candidate = current[:offset] + current[end:]
			candidate := make([]byte, 0, len(current)-(end-offset))
			candidate = append(candidate, current[:offset]...)
			candidate = append(candidate, current[end:]...)

			if _, ok := check(candidate); ok {
				current = candidate
				// Don't advance offset — the next chunk now starts here.
			} else {
				offset += chunkSize
			}
		}
	}

	// ---- Pass 2: byte-by-byte deletion ------------------------------------
	i := 0
	for i < len(current) {
		candidate := make([]byte, 0, len(current)-1)
		candidate = append(candidate, current[:i]...)
		candidate = append(candidate, current[i+1:]...)

		if _, ok := check(candidate); ok {
			current = candidate
			// Re-check position i (now holds what was i+1).
		} else {
			i++
		}
	}

	disc, _ := check(current)
	if disc != nil {
		disc.Input = current
	}
	return current, disc
}
