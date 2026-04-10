package engine

import (
	"math/rand"
)

// Mutator generates new inputs by applying random mutations.
type Mutator struct {
	rng          *rand.Rand
	maxInputSize int
	strategies   []func([]byte) []byte
}

// NewMutator creates a mutator with the given PRNG seed and max input size.
func NewMutator(seed int64, maxInputSize int) *Mutator {
	m := &Mutator{
		rng:          rand.New(rand.NewSource(seed)),
		maxInputSize: maxInputSize,
	}
	m.strategies = []func([]byte) []byte{
		m.bitFlip,
		m.byteFlip,
		m.arithmeticInc,
		m.arithmeticDec,
		m.interestingValue,
		m.randomByte,
		m.insertBytes,
		m.deleteBytes,
		m.duplicateChunk,
	}
	return m
}

// Mutate returns a mutated copy of input.
func (m *Mutator) Mutate(input []byte) []byte {
	if len(input) == 0 {
		n := m.rng.Intn(64) + 1
		out := make([]byte, n)
		for i := range out {
			out[i] = byte(m.rng.Intn(256))
		}
		return out
	}

	result := make([]byte, len(input))
	copy(result, input)

	numMutations := m.rng.Intn(3) + 1
	for i := 0; i < numMutations; i++ {
		result = m.strategies[m.rng.Intn(len(m.strategies))](result)
		if len(result) > m.maxInputSize {
			result = result[:m.maxInputSize]
		}
	}
	return result
}

// Splice combines a prefix of a with a suffix of b at a random crossover point.
func (m *Mutator) Splice(a, b []byte) []byte {
	if len(a) == 0 || len(b) == 0 {
		if len(a) > 0 {
			return a
		}
		return b
	}
	splitA := m.rng.Intn(len(a))
	splitB := m.rng.Intn(len(b))
	out := make([]byte, 0, splitA+len(b)-splitB)
	out = append(out, a[:splitA]...)
	out = append(out, b[splitB:]...)
	if len(out) > m.maxInputSize {
		out = out[:m.maxInputSize]
	}
	return out
}
