package compare

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Numeric compares outputs by parsing them as float64 numbers and checking
// whether they agree within an absolute or relative epsilon. NaN and Inf
// are treated as equal only when both sides produce the same special value.
type Numeric struct {
	// Epsilon is the absolute tolerance. Two values a and b are considered
	// equal when |a-b| <= Epsilon. Defaults to 1e-9 when zero.
	Epsilon float64
	// Relative, when true, switches to relative tolerance: |a-b|/max(|a|,|b|) <= Epsilon.
	Relative bool
}

func (n Numeric) Name() string { return "numeric" }

func (n Numeric) Compare(input []byte, outputs map[string][]byte) *Discrepancy {
	eps := n.Epsilon
	if eps == 0 {
		eps = 1e-9
	}

	type result struct {
		name string
		val  float64
		err  error
	}
	var results []result
	for name, out := range outputs {
		raw := strings.TrimSpace(string(out))
		val, err := strconv.ParseFloat(raw, 64)
		results = append(results, result{name: name, val: val, err: err})
	}

	// Need at least two for comparison.
	if len(results) < 2 {
		return nil
	}
	ref := results[0]
	for _, cur := range results[1:] {
		if ref.err != nil && cur.err != nil {
			continue
		}
		if ref.err != nil || cur.err != nil {
			which, other := ref, cur
			if which.err == nil {
				which, other = cur, ref
			}
			return &Discrepancy{
				Input:       input,
				Outputs:     copyOutputs(outputs),
				Description: fmt.Sprintf("%q failed to parse as number (%v) but %q returned %g", which.name, which.err, other.name, other.val),
				Comparator:  "numeric",
			}
		}

		if !numericsEqual(ref.val, cur.val, eps, n.Relative) {
			return &Discrepancy{
				Input:       input,
				Outputs:     copyOutputs(outputs),
				Description: fmt.Sprintf("numeric mismatch between %q (%g) and %q (%g) exceeds epsilon %g", ref.name, ref.val, cur.name, cur.val, eps),
				Comparator:  "numeric",
			}
		}
	}
	return nil
}

func numericsEqual(a, b, eps float64, relative bool) bool {
	// Handle special values first.
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return a == b // +Inf == +Inf, -Inf == -Inf, but not +Inf == -Inf
	}

	diff := math.Abs(a - b)
	if !relative {
		return diff <= eps
	}
	denom := math.Max(math.Abs(a), math.Abs(b))
	if denom == 0 {
		return diff == 0
	}
	return diff/denom <= eps
}
