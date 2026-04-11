package engine

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

// Corpus manages the set of fuzz inputs.
type Corpus struct {
	entries  [][]byte
	hashes   map[[32]byte]bool
	seedDir  string
	cacheDir string
}

// NewCorpus creates a corpus backed by the given directories.
func NewCorpus(seedDir, cacheDir string) *Corpus {
	return &Corpus{
		hashes:   make(map[[32]byte]bool),
		seedDir:  seedDir,
		cacheDir: cacheDir,
	}
}

// Load reads seed and cached inputs from disk.
func (c *Corpus) Load() error {
	for _, dir := range []string{c.seedDir, c.cacheDir} {
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

// Add inserts an input into the corpus if it's not a duplicate.
// Returns true if the input was new.
func (c *Corpus) Add(input []byte) bool {
	h := sha256.Sum256(input)
	if c.hashes[h] {
		return false
	}
	cp := make([]byte, len(input))
	copy(cp, input)
	c.hashes[h] = true
	c.entries = append(c.entries, cp)
	return true
}

// Save persists an input to the cache directory.
func (c *Corpus) Save(input []byte) error {
	if c.cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	h := sha256.Sum256(input)
	name := fmt.Sprintf("%x", h[:8])
	return os.WriteFile(filepath.Join(c.cacheDir, name), input, 0644)
}

// Pick returns a random corpus entry.
func (c *Corpus) Pick(rng *rand.Rand) []byte {
	if len(c.entries) == 0 {
		return nil
	}
	return c.entries[rng.Intn(len(c.entries))]
}

// Len returns the number of entries in the corpus.
func (c *Corpus) Len() int {
	return len(c.entries)
}

// All returns a snapshot of all corpus entries.
func (c *Corpus) All() [][]byte {
	result := make([][]byte, len(c.entries))
	copy(result, c.entries)
	return result
}
