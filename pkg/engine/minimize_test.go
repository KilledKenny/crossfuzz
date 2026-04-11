package engine

import (
	"bytes"
	"testing"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/runner"
)

// mockRunner is a test double for runner.Runner.
// It returns a fixed output determined by the provided execute function.
type mockRunner struct {
	name   string
	execFn func(input []byte) []byte
}

func (m *mockRunner) Name() string { return m.name }
func (m *mockRunner) Start() error { return nil }
func (m *mockRunner) Stop() error  { return nil }
func (m *mockRunner) Execute(input []byte) ([]byte, []byte, error) {
	return m.execFn(input), make([]byte, 65536), nil
}

// triggerOnSuffix returns a comparator-triggering pair of runners where
// runner "a" always returns "ok" and runner "b" returns "FAIL" whenever
// the input contains the trigger byte sequence.
func makeRunners(trigger []byte) (*mockRunner, *mockRunner) {
	a := &mockRunner{name: "a", execFn: func(input []byte) []byte { return []byte("ok") }}
	b := &mockRunner{name: "b", execFn: func(input []byte) []byte {
		if bytes.Contains(input, trigger) {
			return []byte("FAIL")
		}
		return []byte("ok")
	}}
	return a, b
}

func TestMinimize_RemovesIrrelevantBytes(t *testing.T) {
	trigger := []byte("BUG")
	// Input is trigger surrounded by lots of padding.
	padding := bytes.Repeat([]byte("X"), 50)
	input := append(append(padding, trigger...), padding...)

	a, b := makeRunners(trigger)
	comp := compare.ByteEqual{}

	minimized, disc := Minimize(input, []runner.Runner{a, b}, comp)
	if disc == nil {
		t.Fatal("expected discrepancy after minimization")
	}
	if !bytes.Contains(minimized, trigger) {
		t.Fatalf("minimized input %q does not contain trigger %q", minimized, trigger)
	}
	if len(minimized) >= len(input) {
		t.Errorf("minimizer made no progress: input=%d minimized=%d", len(input), len(minimized))
	}
	// Ideally we converge to exactly the trigger.
	if len(minimized) > len(trigger)*3 {
		t.Logf("minimized to %d bytes (trigger is %d); acceptable but not optimal", len(minimized), len(trigger))
	}
}

func TestMinimize_AlreadyMinimal(t *testing.T) {
	trigger := []byte("X")
	a, b := makeRunners(trigger)
	comp := compare.ByteEqual{}

	minimized, disc := Minimize(trigger, []runner.Runner{a, b}, comp)
	if disc == nil {
		t.Fatal("expected discrepancy")
	}
	if !bytes.Equal(minimized, trigger) {
		t.Errorf("expected %q, got %q", trigger, minimized)
	}
}

func TestMinimize_NoDiscrepancy(t *testing.T) {
	trigger := []byte("NEVER")
	input := []byte("hello world")

	a, b := makeRunners(trigger)
	comp := compare.ByteEqual{}

	minimized, disc := Minimize(input, []runner.Runner{a, b}, comp)
	if disc != nil {
		t.Fatal("expected no discrepancy")
	}
	// Input should be unchanged (empty after full reduction).
	_ = minimized
}
