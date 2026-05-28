package compare

import (
	"fmt"
	"sort"

	"github.com/KilledKenny/crossfuzz/pkg/runner"
)

// Harness is a comparator backed by an external harness process.
// It implements the Comparator interface by delegating to a CompareProcess
// that reads target outputs directly from their shared memory regions.
type Harness struct {
	Proc *runner.CompareProcess
}

func (h Harness) Name() string { return "harness" }

func (h Harness) Compare(input []byte, outputs map[string][]byte) *Discrepancy {
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)

	mismatch, err := h.Proc.Compare(names)
	if err != nil {
		return &Discrepancy{
			Input:       input,
			Outputs:     copyOutputs(outputs),
			Description: fmt.Sprintf("comparator harness error: %v", err),
			Comparator:  "harness",
		}
	}
	if mismatch == "" {
		return nil
	}
	return &Discrepancy{
		Input:       input,
		Outputs:     copyOutputs(outputs),
		Description: mismatch,
		Comparator:  "harness",
	}
}
