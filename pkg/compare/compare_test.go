package compare

import (
	"math"
	"testing"
)

// ---- JSONStructural --------------------------------------------------------

func TestJSONStructural_Equal(t *testing.T) {
	comp := JSONStructural{}
	outputs := map[string][]byte{
		"a": []byte(`{"x":1,"y":2}`),
		"b": []byte(`{"y":2,"x":1}`),
	}
	if d := comp.Compare([]byte("input"), outputs); d != nil {
		t.Fatalf("expected no discrepancy, got: %s", d.Description)
	}
}

func TestJSONStructural_Mismatch(t *testing.T) {
	comp := JSONStructural{}
	outputs := map[string][]byte{
		"a": []byte(`{"x":1}`),
		"b": []byte(`{"x":2}`),
	}
	d := comp.Compare([]byte("input"), outputs)
	if d == nil {
		t.Fatal("expected discrepancy, got nil")
	}
	if d.Comparator != "json_structural" {
		t.Errorf("wrong comparator name: %s", d.Comparator)
	}
}

func TestJSONStructural_OneParseError(t *testing.T) {
	comp := JSONStructural{}
	outputs := map[string][]byte{
		"a": []byte(`{"x":1}`),
		"b": []byte(`not json`),
	}
	d := comp.Compare([]byte("input"), outputs)
	if d == nil {
		t.Fatal("expected discrepancy for parse error mismatch")
	}
}

func TestJSONStructural_BothParseError_Same(t *testing.T) {
	comp := JSONStructural{}
	// Both produce invalid JSON — same error, no discrepancy.
	outputs := map[string][]byte{
		"a": []byte(`not json`),
		"b": []byte(`not json`),
	}
	// Different parse errors on same invalid input are still equal if the
	// json package returns the same error string.
	_ = comp.Compare([]byte("input"), outputs)
	// We just verify it doesn't panic.
}

func TestJSONStructural_NestedObjects(t *testing.T) {
	comp := JSONStructural{}
	outputs := map[string][]byte{
		"a": []byte(`{"a":{"b":{"c":42}}}`),
		"b": []byte(`{"a":{"b":{"c":42}}}`),
	}
	if d := comp.Compare([]byte("in"), outputs); d != nil {
		t.Fatalf("unexpected discrepancy: %s", d.Description)
	}
}

func TestJSONStructural_Arrays(t *testing.T) {
	comp := JSONStructural{}
	// Arrays are order-sensitive.
	outputs := map[string][]byte{
		"a": []byte(`[1,2,3]`),
		"b": []byte(`[3,2,1]`),
	}
	if d := comp.Compare([]byte("in"), outputs); d == nil {
		t.Fatal("expected discrepancy for reordered array")
	}
}

// ---- Numeric ---------------------------------------------------------------

func TestNumeric_Equal(t *testing.T) {
	comp := Numeric{Epsilon: 1e-6}
	outputs := map[string][]byte{
		"a": []byte("3.14159265"),
		"b": []byte("3.14159265"),
	}
	if d := comp.Compare([]byte("in"), outputs); d != nil {
		t.Fatalf("unexpected discrepancy: %s", d.Description)
	}
}

func TestNumeric_WithinEpsilon(t *testing.T) {
	comp := Numeric{Epsilon: 0.01}
	outputs := map[string][]byte{
		"a": []byte("1.000"),
		"b": []byte("1.005"),
	}
	if d := comp.Compare([]byte("in"), outputs); d != nil {
		t.Fatalf("unexpected discrepancy within epsilon: %s", d.Description)
	}
}

func TestNumeric_ExceedsEpsilon(t *testing.T) {
	comp := Numeric{Epsilon: 0.001}
	outputs := map[string][]byte{
		"a": []byte("1.0"),
		"b": []byte("1.1"),
	}
	if d := comp.Compare([]byte("in"), outputs); d == nil {
		t.Fatal("expected discrepancy exceeding epsilon")
	}
}

func TestNumeric_NaN(t *testing.T) {
	comp := Numeric{}
	// Both NaN → equal.
	outputs := map[string][]byte{
		"a": []byte("NaN"),
		"b": []byte("NaN"),
	}
	if d := comp.Compare([]byte("in"), outputs); d != nil {
		t.Fatalf("NaN == NaN should not be a discrepancy: %s", d.Description)
	}
}

func TestNumeric_NaN_vs_Number(t *testing.T) {
	comp := Numeric{}
	outputs := map[string][]byte{
		"a": []byte("NaN"),
		"b": []byte("0"),
	}
	if d := comp.Compare([]byte("in"), outputs); d == nil {
		t.Fatal("expected discrepancy: NaN vs 0")
	}
}

func TestNumeric_Inf(t *testing.T) {
	comp := Numeric{}
	outputs := map[string][]byte{
		"a": []byte("+Inf"),
		"b": []byte("+Inf"),
	}
	if d := comp.Compare([]byte("in"), outputs); d != nil {
		t.Fatalf("unexpected discrepancy for equal Inf: %s", d.Description)
	}
}

func TestNumeric_Relative(t *testing.T) {
	comp := Numeric{Epsilon: 0.01, Relative: true}
	outputs := map[string][]byte{
		"a": []byte("1000000.0"),
		"b": []byte("1000001.0"),
	}
	// Relative diff = 1e-6 < 0.01.
	if d := comp.Compare([]byte("in"), outputs); d != nil {
		t.Fatalf("unexpected discrepancy with relative epsilon: %s", d.Description)
	}
}

func TestNumericEqual_Helpers(t *testing.T) {
	if !numericsEqual(math.NaN(), math.NaN(), 1e-9, false) {
		t.Error("NaN == NaN should be true")
	}
	if numericsEqual(math.NaN(), 0, 1e-9, false) {
		t.Error("NaN == 0 should be false")
	}
	if !numericsEqual(math.Inf(1), math.Inf(1), 1e-9, false) {
		t.Error("+Inf == +Inf should be true")
	}
	if numericsEqual(math.Inf(1), math.Inf(-1), 1e-9, false) {
		t.Error("+Inf == -Inf should be false")
	}
}
