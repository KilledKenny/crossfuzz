package engine

import (
	"bytes"
	"testing"
)

func TestMutatorDictStrategiesRegistered(t *testing.T) {
	noDict := NewMutator(1, 1024, nil)
	withDict := NewMutator(1, 1024, func() *Dict { d := NewDict(); d.Add([]byte("foo")); return d }())
	if withDict.NumStrategies() != noDict.NumStrategies()+2 {
		t.Fatalf("expected dict mutator to add 2 strategies, got %d vs %d",
			withDict.NumStrategies(), noDict.NumStrategies())
	}
}

func TestMutatorMutateReturnsMutation(t *testing.T) {
	d := NewDict()
	d.Add([]byte("xx"))
	m := NewMutator(42, 64, d)

	input := []byte("hello world")
	differ := 0
	for i := 0; i < 100; i++ {
		out := m.Mutate(input)
		if !bytes.Equal(out, input) {
			differ++
		}
		if len(m.LastOps()) == 0 {
			t.Fatalf("LastOps empty after Mutate")
		}
	}
	if differ < 80 {
		t.Errorf("only %d/100 mutations differed from input", differ)
	}
}

func TestMutatorRewardUpdatesArms(t *testing.T) {
	m := NewMutator(7, 64, nil)
	// All arms must have pulls > 0 after the warmup phase plus a bit more
	// so UCB1 has data to pick from.
	for i := 0; i < mutWarmupPulls+200; i++ {
		_ = m.Mutate([]byte("abc"))
		ops := m.LastOps()
		// Pretend strategy 0 always pays off.
		reward := 0.0
		for _, op := range ops {
			if op == 0 {
				reward = 1.0
			}
		}
		m.Reward(ops, reward)
	}
	if m.arms[0].pulls == 0 {
		t.Fatal("arm 0 was never pulled")
	}
	if m.arms[0].rewards == 0 {
		t.Fatal("arm 0 was rewarded but rewards is 0")
	}
}

func TestMutatorEmptyInputProducesData(t *testing.T) {
	m := NewMutator(3, 64, nil)
	out := m.Mutate(nil)
	if len(out) == 0 {
		t.Fatal("Mutate(nil) returned empty")
	}
}

func TestMutatorMaxInputSizeRespected(t *testing.T) {
	m := NewMutator(5, 16, nil)
	input := bytes.Repeat([]byte("a"), 16)
	for i := 0; i < 200; i++ {
		out := m.Mutate(input)
		if len(out) > 16 {
			t.Fatalf("output len %d exceeds max 16", len(out))
		}
	}
}
