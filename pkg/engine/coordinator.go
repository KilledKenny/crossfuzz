package engine

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/runner"
)

// Coordinator drives the fuzzing campaign.
type Coordinator struct {
	cfg        *config.Config
	runners    []runner.Runner
	corpus     *Corpus
	mutator    *Mutator
	comparator compare.Comparator
	stats      *Stats
	globalCov  []byte
	rng        *rand.Rand
}

// NewCoordinator creates a coordinator for the given config and runners.
func NewCoordinator(cfg *config.Config, runners []runner.Runner, comp compare.Comparator) *Coordinator {
	seed := time.Now().UnixNano()
	return &Coordinator{
		cfg:        cfg,
		runners:    runners,
		corpus:     NewCorpus(cfg.Corpus.SeedDir, cfg.Corpus.CacheDir),
		mutator:    NewMutator(seed, cfg.Campaign.MaxInputSize),
		comparator: comp,
		stats:      NewStats(),
		globalCov:  make([]byte, coverage.BitmapSize),
		rng:        rand.New(rand.NewSource(seed + 1)),
	}
}

// Run executes the fuzzing campaign until the context is cancelled or timeout.
func (c *Coordinator) Run(ctx context.Context) error {
	if err := c.corpus.Load(); err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}
	os.MkdirAll(c.cfg.Corpus.FindingsDir, 0755)

	if c.corpus.Len() == 0 {
		c.corpus.Add([]byte(""))
	}

	fmt.Printf("Starting campaign %q with %d targets, %d seed inputs\n",
		c.cfg.Campaign.Name, len(c.runners), c.corpus.Len())

	if c.cfg.Campaign.Timeout.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.Campaign.Timeout.Duration)
		defer cancel()
	}

	findings := 0
	spliceRate := 10 // 1 in N iterations is a splice instead of mutation

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\n\nCampaign finished. Total execs: %d, Corpus: %d, Findings: %d\n",
				c.stats.totalExecs, c.corpus.Len(), findings)
			return nil
		default:
		}

		// Generate input: mostly mutate, occasionally splice.
		var input []byte
		base := c.corpus.Pick(c.rng)
		if c.corpus.Len() > 1 && c.rng.Intn(spliceRate) == 0 {
			other := c.corpus.Pick(c.rng)
			input = c.mutator.Splice(base, other)
		} else {
			input = c.mutator.Mutate(base)
		}

		// Execute on all targets.
		outputs := make(map[string][]byte, len(c.runners))
		combinedCov := make([]byte, coverage.BitmapSize)
		var execErr error

		for _, r := range c.runners {
			output, cov, err := r.Execute(input)
			if err != nil {
				execErr = fmt.Errorf("target %s: %w", r.Name(), err)
				break
			}
			outputs[r.Name()] = output
			coverage.Merge(combinedCov, cov)
		}

		if execErr != nil {
			fmt.Printf("\nExec error: %v\n", execErr)
			continue
		}

		c.stats.RecordExec()

		// Check for new coverage.
		coverage.Bucketize(combinedCov)
		if coverage.HasNewBits(c.globalCov, combinedCov) {
			coverage.Merge(c.globalCov, combinedCov)
			if c.corpus.Add(input) {
				c.corpus.Save(input)
			}
		}

		// Compare outputs across targets.
		if disc := c.comparator.Compare(input, outputs); disc != nil {
			findings++
			c.saveFinding(disc, findings)
			fmt.Printf("\n[FINDING #%d] %s (input: %d bytes)\n", findings, disc.Description, len(input))
			return nil
		}

		c.stats.Update(c.corpus.Len(), coverage.CountBits(c.globalCov), findings)
		c.stats.PrintIfDue()
	}
}

func (c *Coordinator) saveFinding(disc *compare.Discrepancy, id int) {
	dir := filepath.Join(c.cfg.Corpus.FindingsDir, fmt.Sprintf("finding_%04d", id))
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "input.bin"), disc.Input, 0644)
	for name, output := range disc.Outputs {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("output_%s.bin", name)), output, 0644)
	}
	os.WriteFile(filepath.Join(dir, "description.txt"), []byte(disc.Description), 0644)
}
