package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/runner"

	"golang.org/x/time/rate"
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

var logCovSometimes = rate.Sometimes{First: 10, Interval: time.Second}

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
		outputs, combinedCov, execErr := c.executeAll(input)
		if execErr != nil {
			fmt.Printf("\nExec error: %v\n", execErr)
			continue
		}

		c.stats.RecordExec()

		// Check for new coverage. Re-run once to filter out flaky
		// edges — Go's runtime/coverage instrumentation still emits a
		// small amount of noise on GC/scheduler paths even after the
		// harness-side noise mask, so we accept only bits that show up
		// in BOTH runs before claiming new coverage.
		coverage.Bucketize(combinedCov)
		if coverage.HasNewBits(c.globalCov, combinedCov) {
			_, verifyCov, verifyErr := c.executeAll(input)
			if verifyErr == nil {
				coverage.Bucketize(verifyCov)
				for i := range combinedCov {
					combinedCov[i] &= verifyCov[i]
				}
				if coverage.HasNewBits(c.globalCov, combinedCov) {
					coverage.Merge(c.globalCov, combinedCov)
					if c.corpus.Add(input) {
						c.corpus.Save(input)
					}
				}
			}
		}

		// Compare outputs across targets.
		if disc := c.comparator.Compare(input, outputs); disc != nil {
			findings++
			minimized, minDisc := Minimize(disc.Input, c.runners, c.comparator)
			if minDisc != nil {
				disc = minDisc
			} else {
				disc.Input = minimized
			}
			c.saveFinding(disc, findings)
			fmt.Printf("\n[FINDING #%d] %s (input: %d bytes)\n", findings, disc.Description, len(disc.Input))
		}

		c.stats.Update(c.corpus.Len(), coverage.CountBits(c.globalCov), findings)
		c.stats.PrintIfDue()
	}
}

// executeAll runs input through every configured target and returns
// their outputs plus the merged raw (un-bucketized) coverage bitmap.
func (c *Coordinator) executeAll(input []byte) (map[string][]byte, []byte, error) {
	outputs := make(map[string][]byte, len(c.runners))
	combined := make([]byte, coverage.BitmapSize)
	for _, r := range c.runners {
		output, cov, err := r.Execute(input)
		if err != nil {
			return nil, nil, fmt.Errorf("target %s: %w", r.Name(), err)
		}
		outputs[r.Name()] = output
		coverage.Merge(combined, cov)
	}
	return outputs, combined, nil
}

func (c *Coordinator) saveFinding(disc *compare.Discrepancy, id int) {
	h := sha256.Sum256(disc.Input)
	dirName := fmt.Sprintf("%x", h[:8])
	dir := filepath.Join(c.cfg.Corpus.FindingsDir, dirName)
	os.MkdirAll(dir, 0755)

	os.WriteFile(filepath.Join(dir, "input.bin"), disc.Input, 0644)
	for name, output := range disc.Outputs {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("output_%s.bin", name)), output, 0644)
	}

	type metadata struct {
		ID          int               `json:"id"`
		Hash        string            `json:"hash"`
		Comparator  string            `json:"comparator"`
		Description string            `json:"description"`
		InputLen    int               `json:"input_len"`
		OutputLens  map[string]int    `json:"output_lens"`
		Timestamp   string            `json:"timestamp"`
	}
	lens := make(map[string]int, len(disc.Outputs))
	for name, out := range disc.Outputs {
		lens[name] = len(out)
	}
	meta := metadata{
		ID:          id,
		Hash:        fmt.Sprintf("%x", h),
		Comparator:  disc.Comparator,
		Description: disc.Description,
		InputLen:    len(disc.Input),
		OutputLens:  lens,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	if data, err := json.MarshalIndent(meta, "", "  "); err == nil {
		os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0644)
	}
}
