package compare

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// JSONStructural compares outputs by parsing them as JSON and performing a
// deep structural comparison that is insensitive to key ordering in objects.
type JSONStructural struct{}

func (JSONStructural) Name() string { return "json_structural" }

func (JSONStructural) Compare(input []byte, outputs map[string][]byte) *Discrepancy {
	type parsed struct {
		name string
		val  any
		err  error
	}
	results := make([]parsed, 0, len(outputs))
	for name, out := range outputs {
		var val any
		err := json.Unmarshal(out, &val)
		results = append(results, parsed{name: name, val: val, err: err})
	}
	// Sort by name for deterministic reference selection.
	sort.Slice(results, func(i, j int) bool { return results[i].name < results[j].name })

	ref := results[0]
	for _, cur := range results[1:] {
		// Both failed to parse — compare raw error strings so we don't
		// mask a real difference (e.g. one panics, the other returns an
		// error message).
		if ref.err != nil && cur.err != nil {
			if ref.err.Error() != cur.err.Error() {
				return &Discrepancy{
					Input:       input,
					Outputs:     copyOutputs(outputs),
					Description: fmt.Sprintf("JSON parse error mismatch between %q and %q: %v vs %v", ref.name, cur.name, ref.err, cur.err),
					Comparator:  "json_structural",
				}
			}
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
				Description: fmt.Sprintf("%q failed JSON parse (%v) but %q succeeded", which.name, which.err, other.name),
				Comparator:  "json_structural",
			}
		}
		if !jsonEqual(ref.val, cur.val) {
			return &Discrepancy{
				Input:       input,
				Outputs:     copyOutputs(outputs),
				Description: fmt.Sprintf("JSON structural mismatch between %q and %q", ref.name, cur.name),
				Comparator:  "json_structural",
			}
		}
	}
	return nil
}

// jsonEqual performs a deep comparison of two JSON-decoded values.
// JSON objects (map[string]any) are compared ignoring key order.
// JSON arrays are compared in order. Scalars use reflect.DeepEqual.
func jsonEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, va := range av {
			vb, exists := bv[k]
			if !exists || !jsonEqual(va, vb) {
				return false
			}
		}
		return true

	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true

	default:
		return reflect.DeepEqual(a, b)
	}
}
