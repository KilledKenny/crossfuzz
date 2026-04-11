package engine

import (
	"context"
	"crypto/sha256"
	"fmt"

	"crossfuzz/pkg/config"
	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/runner"
)

// ReduceResult holds the output of a corpus reduction pass.
type ReduceResult struct {
	// Kept is the deduplicated set of inputs, one per unique coverage profile.
	Kept [][]byte
	// Total is the number of corpus entries evaluated.
	Total int
}

// Reduce loads the corpus, executes every entry through the targets, and
// deduplicates by coverage profile: when two inputs produce identical
// bucketized coverage bitmaps the shorter one is kept. Returns the reduced
// set of inputs and a count of how many were evaluated.
func Reduce(ctx context.Context, cfg *config.Config, runners []runner.Runner) (*ReduceResult, error) {
	corpus := NewCorpus(cfg.Corpus.SeedDir, cfg.Corpus.CacheDir)
	if err := corpus.Load(); err != nil {
		return nil, fmt.Errorf("load corpus: %w", err)
	}

	entries := corpus.All()
	// key: SHA-256 of bucketized combined bitmap → smallest input with that profile
	best := make(map[[32]byte][]byte, len(entries))

	for i, input := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		combined := make([]byte, coverage.BitmapSize)
		ok := true
		for _, r := range runners {
			_, cov, err := r.Execute(input)
			if err != nil {
				fmt.Printf("  [%d/%d] skipped (exec error: %v)\n", i+1, len(entries), err)
				ok = false
				break
			}
			coverage.Merge(combined, cov)
		}
		if !ok {
			continue
		}

		coverage.Bucketize(combined)
		key := sha256.Sum256(combined)

		if existing, seen := best[key]; !seen || len(input) < len(existing) {
			best[key] = input
		}
	}

	kept := make([][]byte, 0, len(best))
	for _, input := range best {
		kept = append(kept, input)
	}

	return &ReduceResult{Kept: kept, Total: len(entries)}, nil
}
