package engine

import "testing"

func TestBloomNoFalseNegatives(t *testing.T) {
	b := newBloom()
	for i := 0; i < 10000; i++ {
		x := []byte{byte(i), byte(i >> 8)}
		if b.CheckAndAdd(x) {
			t.Fatalf("first insert reported as seen: i=%d", i)
		}
		if !b.CheckAndAdd(x) {
			t.Fatalf("immediate re-insert reported as new: i=%d", i)
		}
	}
}

func TestBloomFalsePositiveRateBounded(t *testing.T) {
	b := newBloom()
	for i := 0; i < 50000; i++ {
		x := []byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}
		b.CheckAndAdd(x)
	}
	// Probe with a disjoint key space and count collisions.
	collisions := 0
	probes := 5000
	for i := 0; i < probes; i++ {
		x := []byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24), 0xee}
		if b.CheckAndAdd(x) {
			collisions++
		}
	}
	if rate := float64(collisions) / float64(probes); rate > 0.05 {
		t.Fatalf("false-positive rate too high: %.3f", rate)
	}
}
