package engine

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
)

// Seed is one corpus entry plus the metadata the power schedule uses to
// allocate energy. All fields are guarded by Corpus.mu.
type Seed struct {
	Data            []byte
	NewEdges        int     // edges this seed first revealed when added
	Hits            int     // times this seed has been picked as mutation base
	ExecTimeNs      int64   // mean per-target exec time at insertion (rough)
	Skew            float64 // stddev/mean of per-target edge counts at insertion
	DivergenceScore float64 // bumped when a child input triggers a finding
	AddedAt         int     // monotonic generation counter (0 = seed/initial)
}

// Corpus manages the set of fuzz inputs and the power-schedule weights over them.
type Corpus struct {
	mu        sync.RWMutex
	entries   []*Seed
	hashes    map[[32]byte]bool
	gen       int // incremented on every successful Add — used as Seed.AddedAt
	seedDir   string
	corpusDir string
}

// NewCorpus creates a corpus backed by the given directories.
func NewCorpus(seedDir, corpusDir string) *Corpus {
	return &Corpus{
		hashes:    make(map[[32]byte]bool),
		seedDir:   seedDir,
		corpusDir: corpusDir,
	}
}

// Load reads seed and cached inputs from disk.
func (c *Corpus) Load() error {
	for _, dir := range []string{c.seedDir, c.corpusDir} {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			c.Add(data)
		}
	}
	return nil
}

// Add inserts an input into the corpus if it's not a duplicate. The returned
// *Seed (nil if duplicate) lets the caller annotate the entry with edge/exec
// metadata after the fact.
func (c *Corpus) Add(input []byte) *Seed {
	h := sha256.Sum256(input)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hashes[h] {
		return nil
	}
	cp := make([]byte, len(input))
	copy(cp, input)
	c.hashes[h] = true
	c.gen++
	s := &Seed{Data: cp, AddedAt: c.gen}
	c.entries = append(c.entries, s)
	return s
}

// Annotate updates the per-seed metadata used by the power schedule.
// Safe to call on a *Seed returned by Add.
func (c *Corpus) Annotate(s *Seed, newEdges int, execTimeNs int64, skew float64) {
	if s == nil {
		return
	}
	c.mu.Lock()
	s.NewEdges = newEdges
	s.ExecTimeNs = execTimeNs
	s.Skew = skew
	c.mu.Unlock()
}

// BumpDivergence increases a seed's divergence multiplier (called when a
// child input it produced triggered a comparator finding).
func (c *Corpus) BumpDivergence(s *Seed, delta float64) {
	if s == nil {
		return
	}
	c.mu.Lock()
	s.DivergenceScore += delta
	c.mu.Unlock()
}

// Save persists an input to the corpus directory.
func (c *Corpus) Save(input []byte) error {
	if c.corpusDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.corpusDir, 0755); err != nil {
		return fmt.Errorf("create corpus dir: %w", err)
	}
	h := sha256.Sum256(input)
	name := fmt.Sprintf("%x", h[:8])
	return os.WriteFile(filepath.Join(c.corpusDir, name), input, 0644)
}

// PickWeighted returns a seed chosen with probability proportional to its
// power-schedule weight. Picking increments Seed.Hits. Returns nil on empty
// corpus.
//
// Weight blends four signals:
//  1. Recency: newer seeds get more attention while their basin is fresh.
//  2. Productivity: seeds that revealed many new edges on insertion stay hot.
//  3. Cost: faster-executing seeds are preferred (1/sqrt(exec_time)).
//  4. Divergence: explicit multiplier from BumpDivergence + per-target skew.
//
// The hit penalty (1/(Hits+1)) prevents any one seed from monopolising work.
func (c *Corpus) PickWeighted(rng *rand.Rand) *Seed {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) == 0 {
		return nil
	}
	weights := make([]float64, len(c.entries))
	var total float64
	maxGen := c.gen
	for i, s := range c.entries {
		w := float64(s.NewEdges+1)
		// Recency: tail of half-life decay so older but still-productive
		// seeds keep some residual weight.
		age := maxGen - s.AddedAt
		w *= 1.0 + math.Exp(-float64(age)/64.0)
		// Hit penalty.
		w /= math.Sqrt(float64(s.Hits) + 1)
		// Exec cost — prefer faster seeds. Saturate at 1ms.
		t := float64(s.ExecTimeNs)
		if t < 1e3 {
			t = 1e3
		}
		w *= 1.0 / math.Sqrt(t/1e6+1) // normalise to ms
		// Differential signals.
		w *= 1.0 + s.Skew
		w *= 1.0 + s.DivergenceScore
		if w < 1e-6 {
			w = 1e-6
		}
		weights[i] = w
		total += w
	}
	r := rng.Float64() * total
	for i, w := range weights {
		r -= w
		if r <= 0 {
			c.entries[i].Hits++
			return c.entries[i]
		}
	}
	// Fall-through on floating-point rounding.
	last := c.entries[len(c.entries)-1]
	last.Hits++
	return last
}

// PickRandom returns a uniformly-random seed. Used as splice's second draw,
// where biasing would defeat the point.
func (c *Corpus) PickRandom(rng *rand.Rand) *Seed {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) == 0 {
		return nil
	}
	return c.entries[rng.Intn(len(c.entries))]
}

// Len returns the number of entries in the corpus.
func (c *Corpus) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// All returns a snapshot of all corpus entry payloads.
func (c *Corpus) All() [][]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([][]byte, len(c.entries))
	for i, s := range c.entries {
		result[i] = s.Data
	}
	return result
}
