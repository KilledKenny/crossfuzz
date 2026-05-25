package engine

import (
	"math/rand"
	"testing"
)

func TestCorpusAddDeduplicates(t *testing.T) {
	c := NewCorpus("", "")
	if s := c.Add([]byte("hello")); s == nil {
		t.Fatal("first Add returned nil")
	}
	if s := c.Add([]byte("hello")); s != nil {
		t.Fatal("duplicate Add should return nil")
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
}

func TestPickWeightedFavorsHighEdgeSeeds(t *testing.T) {
	c := NewCorpus("", "")
	low := c.Add([]byte("low"))
	high := c.Add([]byte("high"))
	c.Annotate(low, 1, 1_000_000, 0)
	c.Annotate(high, 100, 1_000_000, 0)

	rng := rand.New(rand.NewSource(1))
	wins := map[string]int{}
	for i := 0; i < 2000; i++ {
		s := c.PickWeighted(rng)
		wins[string(s.Data)]++
	}
	if wins["high"] <= wins["low"] {
		t.Fatalf("high-edge seed should win more often: low=%d high=%d", wins["low"], wins["high"])
	}
}

func TestPickWeightedFavorsLowExecTime(t *testing.T) {
	c := NewCorpus("", "")
	fast := c.Add([]byte("fast"))
	slow := c.Add([]byte("slow"))
	c.Annotate(fast, 1, 100_000, 0)        // 0.1ms
	c.Annotate(slow, 1, 1_000_000_000, 0)  // 1s

	rng := rand.New(rand.NewSource(2))
	wins := map[string]int{}
	for i := 0; i < 2000; i++ {
		s := c.PickWeighted(rng)
		wins[string(s.Data)]++
	}
	if wins["fast"] <= wins["slow"] {
		t.Fatalf("fast seed should win more often: fast=%d slow=%d", wins["fast"], wins["slow"])
	}
}

func TestBumpDivergenceMakesSeedHotter(t *testing.T) {
	c := NewCorpus("", "")
	a := c.Add([]byte("a"))
	b := c.Add([]byte("b"))
	c.Annotate(a, 1, 1_000_000, 0)
	c.Annotate(b, 1, 1_000_000, 0)
	c.BumpDivergence(b, 10)

	rng := rand.New(rand.NewSource(3))
	wins := map[string]int{}
	for i := 0; i < 2000; i++ {
		s := c.PickWeighted(rng)
		wins[string(s.Data)]++
	}
	if wins["b"] <= wins["a"] {
		t.Fatalf("divergent seed should win more often: a=%d b=%d", wins["a"], wins["b"])
	}
}
