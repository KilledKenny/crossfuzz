package compare

import (
	"bytes"
	"fmt"
)

// Comparator compares outputs from different targets for the same input.
type Comparator interface {
	Name() string
	Compare(input []byte, outputs map[string][]byte) *Discrepancy
}

// Discrepancy describes a behavioral difference between targets.
type Discrepancy struct {
	Input       []byte
	Outputs     map[string][]byte
	Description string
	Comparator  string
}

// ByteEqual compares outputs for exact byte equality.
type ByteEqual struct{}

func (ByteEqual) Name() string { return "byte_equal" }

func (ByteEqual) Compare(input []byte, outputs map[string][]byte) *Discrepancy {
	var refName string
	var refOut []byte
	for name, out := range outputs {
		if refName == "" {
			refName = name
			refOut = out
			continue
		}
		if !bytes.Equal(refOut, out) {
			return &Discrepancy{
				Input:       input,
				Outputs:     copyOutputs(outputs),
				Description: fmt.Sprintf("output mismatch between %q and %q (%d vs %d bytes)", refName, name, len(refOut), len(out)),
				Comparator:  "byte_equal",
			}
		}
	}
	return nil
}

// NoOp is a comparator that never reports a discrepancy.
// Used in server fuzz mode where there is only one harness target and
// output comparison is not meaningful.
type NoOp struct{}

func (NoOp) Name() string                                         { return "none" }
func (NoOp) Compare(_ []byte, _ map[string][]byte) *Discrepancy   { return nil }

func copyOutputs(m map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		cp := make([]byte, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
