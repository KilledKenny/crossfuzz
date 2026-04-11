package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/runner"

	"golang.org/x/time/rate"
)

// Coordinator drives the fuzzing campaign.
type Coordinator struct {
	cfg            *config.Config
	runners        []runner.Runner
	corpus         *Corpus
	mutator        *Mutator
	comparator     compare.Comparator
	stats          *Stats
	globalCov      []byte
	rng            *rand.Rand
	warmupRounds   int
	validateRounds int
	maxFindings    int
	findingCovs    map[[32]byte]bool
}

// NewCoordinator creates a coordinator for the given config and runners.
func NewCoordinator(cfg *config.Config, runners []runner.Runner, comp compare.Comparator) *Coordinator {
	seed := time.Now().UnixNano()
	return &Coordinator{
		cfg:         cfg,
		runners:     runners,
		corpus:      NewCorpus(cfg.Corpus.SeedDir, cfg.Corpus.CacheDir),
		mutator:     NewMutator(seed, cfg.Campaign.MaxInputSize),
		comparator:  comp,
		stats:       NewStats(),
		globalCov:   make([]byte, coverage.BitmapSize),
		rng:         rand.New(rand.NewSource(seed + 1)),
		findingCovs: make(map[[32]byte]bool),
	}
}

// SetWarmupRounds configures the number of warmup rounds to run before the
// main fuzzing loop. Each corpus entry is executed this many times to
// pre-seed the global coverage bitmap.
func (c *Coordinator) SetWarmupRounds(n int) {
	c.warmupRounds = n
}

// SetValidateRounds configures how many extra times each new interesting input
// is re-executed to confirm it is stable before being added to the corpus.
func (c *Coordinator) SetValidateRounds(n int) {
	c.validateRounds = n
}

// SetMaxFindings configures the maximum number of unique findings before the
// campaign stops. A value of 0 means no limit.
func (c *Coordinator) SetMaxFindings(n int) {
	c.maxFindings = n
}

// validateStability runs input through all runners n times and checks whether
// each target produces identical output or coverage on every run. Returns the
// names of any targets whose output or coverage changed across runs (sorted).
// An empty slice means the input is stable. n <= 0 always returns stable.
func validateStability(runners []runner.Runner, input []byte, n int) []string {
	if n <= 0 {
		return nil
	}
	firstOutput := make(map[string][]byte, len(runners))
	firstCoverage := make(map[string][]byte, len(runners))
	unstable := make(map[string]bool)

	for i := 0; i < n; i++ {
		for _, r := range runners {
			if unstable[r.Name()] {
				continue
			}
			output, coverage, err := r.Execute(input)
			if err != nil {
				continue
			}
			if i == 0 {
				firstOutput[r.Name()] = output
				firstCoverage[r.Name()] = coverage
			} else if !bytes.Equal(firstOutput[r.Name()], output) {
				unstable[r.Name()] = true
			} else if !bytes.Equal(firstCoverage[r.Name()], coverage) {
				unstable[r.Name()] = true
			}
		}
	}

	if len(unstable) == 0 {
		return nil
	}
	names := make([]string, 0, len(unstable))
	for name := range unstable {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var logCovSometimes = rate.Sometimes{First: 10, Interval: time.Second}

// Warmup runs every corpus entry rounds times to pre-seed the global coverage
// bitmap before the main fuzzing loop begins.
func (c *Coordinator) Warmup(ctx context.Context, rounds int) error {
	if rounds <= 0 {
		return nil
	}
	entries := c.corpus.All()
	fmt.Printf("Warmup: running %d corpus entries × %d rounds\n", len(entries), rounds)
	for round := 0; round < rounds; round++ {
		for _, input := range entries {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			_, cov, err := c.executeAll(input)
			if err != nil {
				return fmt.Errorf("warmup exec: %w", err)
			}
			coverage.Bucketize(cov)
			coverage.Merge(c.globalCov, cov)
		}
	}
	fmt.Printf("Warmup complete. Coverage bits: %d\n", coverage.CountBits(c.globalCov))
	return nil
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

	if c.warmupRounds > 0 {
		if err := c.Warmup(ctx, c.warmupRounds); err != nil {
			return fmt.Errorf("warmup: %w", err)
		}
	}

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

		// Check for new coverage. Re-run to filter out flaky edges —
		// Go's runtime/coverage instrumentation still emits a small
		// amount of noise on GC/scheduler paths even after the
		// harness-side noise mask, so we accept only bits that show up
		// in every verification run before claiming new coverage.
		coverage.Bucketize(combinedCov)
		if coverage.HasNewBits(c.globalCov, combinedCov) {

			stable := true
			if c.validateRounds > 0 {
				if unstable := validateStability(c.runners, input, c.validateRounds); len(unstable) > 0 {
					fmt.Printf("\n[UNSTABLE] input (%d bytes) discarded — targets with non-deterministic output: %v\n",
						len(input), unstable)
					stable = false
				}
			}
			if stable {
				coverage.Merge(c.globalCov, combinedCov)
				if c.corpus.Add(input) {
					c.corpus.Save(input)
				}
			}
		}

		// Compare outputs across targets.
		if disc := c.comparator.Compare(input, outputs); disc != nil {
			covKey := sha256.Sum256(combinedCov)
			if c.findingCovs[covKey] {
				continue
			}
			c.findingCovs[covKey] = true
			findings++
			minimized, minDisc := Minimize(disc.Input, c.runners, c.comparator)
			if minDisc != nil {
				disc = minDisc
			} else {
				disc.Input = minimized
			}
			c.saveFinding(disc, findings)
			fmt.Printf("\n[FINDING #%d] %s (input: %d bytes)\n", findings, disc.Description, len(disc.Input))
			if c.maxFindings > 0 && findings >= c.maxFindings {
				fmt.Printf("\nMax findings (%d) reached. Stopping.\n", c.maxFindings)
				return nil
			}
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
		ID          int            `json:"id"`
		Hash        string         `json:"hash"`
		Comparator  string         `json:"comparator"`
		Description string         `json:"description"`
		InputLen    int            `json:"input_len"`
		OutputLens  map[string]int `json:"output_lens"`
		Timestamp   string         `json:"timestamp"`
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
