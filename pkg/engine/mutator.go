package engine

import (
	"math"
	"math/rand"
)

// mutateOp is one byte-level mutation strategy. It mutates its argument in
// place when possible and returns the (possibly resized) slice.
type mutateOp func([]byte) []byte

// banditArm tracks per-strategy MAB statistics for UCB1 selection.
type banditArm struct {
	pulls   int
	rewards float64
}

// Mutator generates new inputs by stacking random byte-level mutations.
//
// Strategy selection is a UCB1 multi-armed bandit over the registered ops:
// each Mutate call records which arms fired, and Reward credits all of them
// with the same {0,1} payoff after the coordinator sees whether new edges
// appeared. Before warmupPulls total pulls have been observed, selection
// falls back to uniform random so every arm gets baseline data.
//
// All Mutator methods are safe to call from one goroutine; the coordinator
// uses one Mutator per worker.
type Mutator struct {
	rng          *rand.Rand
	maxInputSize int

	dict       *Dict
	ops        []mutateOp
	opNames    []string
	arms       []banditArm
	totalPulls int

	lastOps []int // strategy indices used in the most recent Mutate call
}

const (
	mutWarmupPulls = 500
	mutUCBConstant = 1.4
	mutMaxStack    = 7 // stacks are 1<<rand.Intn(mutMaxStack), so up to 64
)

// NewMutator builds a mutator. dict may be nil or empty; dict-dependent
// strategies are registered only when at least one entry is available.
func NewMutator(seed int64, maxInputSize int, dict *Dict) *Mutator {
	m := &Mutator{
		rng:          rand.New(rand.NewSource(seed)),
		maxInputSize: maxInputSize,
		dict:         dict,
	}
	m.register("bit_flip", m.bitFlip)
	m.register("byte_flip", m.byteFlip)
	m.register("arith_inc", m.arithmeticInc)
	m.register("arith_dec", m.arithmeticDec)
	m.register("interesting", m.interestingValue)
	m.register("random_byte", m.randomByte)
	m.register("insert_bytes", m.insertBytes)
	m.register("delete_bytes", m.deleteBytes)
	m.register("dup_chunk", m.duplicateChunk)
	if dict != nil && dict.Len() > 0 {
		m.register("dict_overwrite", m.dictOverwrite)
		m.register("dict_insert", m.dictInsert)
	}
	m.arms = make([]banditArm, len(m.ops))
	return m
}

func (m *Mutator) register(name string, op mutateOp) {
	m.ops = append(m.ops, op)
	m.opNames = append(m.opNames, name)
}

// NumStrategies returns how many bandit arms are registered.
func (m *Mutator) NumStrategies() int { return len(m.ops) }

// LastOps returns the strategy indices fired by the most recent Mutate call.
// The slice is owned by the Mutator and overwritten on the next call.
func (m *Mutator) LastOps() []int { return m.lastOps }

// StrategyName returns the human-readable name for a strategy index.
func (m *Mutator) StrategyName(i int) string { return m.opNames[i] }

// Reward credits each strategy index with a reward in [0,1]. Call this after
// the coordinator decides whether the input produced by Mutate was interesting
// (new coverage = 1, otherwise = 0).
func (m *Mutator) Reward(ops []int, reward float64) {
	for _, i := range ops {
		if i < 0 || i >= len(m.arms) {
			continue
		}
		m.arms[i].pulls++
		m.arms[i].rewards += reward
		m.totalPulls++
	}
}

// pickOp selects a strategy index using UCB1 (or uniform during warmup).
func (m *Mutator) pickOp() int {
	if m.totalPulls < mutWarmupPulls {
		return m.rng.Intn(len(m.ops))
	}
	// Pull any untouched arm first.
	for i, a := range m.arms {
		if a.pulls == 0 {
			return i
		}
	}
	logN := math.Log(float64(m.totalPulls))
	best := -1
	bestScore := math.Inf(-1)
	for i, a := range m.arms {
		mean := a.rewards / float64(a.pulls)
		bonus := mutUCBConstant * math.Sqrt(2*logN/float64(a.pulls))
		s := mean + bonus
		if s > bestScore {
			bestScore = s
			best = i
		}
	}
	if best < 0 {
		best = m.rng.Intn(len(m.ops))
	}
	return best
}

// Mutate returns a mutated copy of input. The strategies that fired are
// available via LastOps until the next Mutate call.
func (m *Mutator) Mutate(input []byte) []byte {
	if len(input) == 0 {
		n := m.rng.Intn(64) + 1
		out := make([]byte, n)
		for i := range out {
			out[i] = byte(m.rng.Intn(256))
		}
		m.lastOps = m.lastOps[:0]
		return out
	}

	result := make([]byte, len(input))
	copy(result, input)

	// Power-of-two stacking: 1, 2, 4, 8, 16, 32, 64. This roughly matches
	// AFL's havoc behaviour where most calls do a few mutations and a long
	// tail does many — small inputs still see small stacks, but a large
	// input occasionally gets reshaped aggressively in one call.
	stack := 1 << m.rng.Intn(mutMaxStack)

	if cap(m.lastOps) < stack {
		m.lastOps = make([]int, 0, stack)
	} else {
		m.lastOps = m.lastOps[:0]
	}

	for i := 0; i < stack; i++ {
		idx := m.pickOp()
		result = m.ops[idx](result)
		m.lastOps = append(m.lastOps, idx)
		if len(result) > m.maxInputSize {
			result = result[:m.maxInputSize]
		}
		if len(result) == 0 {
			// Avoid feeding zero-length intermediates back into ops that
			// short-circuit on empty input; reseed with one random byte.
			result = append(result, byte(m.rng.Intn(256)))
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
	m.lastOps = m.lastOps[:0]
	return out
}
