package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/runner"

	"golang.org/x/time/rate"
)

// WorkerRunners bundles the per-worker resources for a single parallel fuzzing
// worker. Each worker gets its own independent set of target processes, its own
// comparator, and its own input filter: a harness comparator reads the
// shared-memory regions of this worker's targets rather than another worker's,
// and a per-worker filter keeps workers from serialising through one filter
// process. Filter may be nil if no input filter is configured.
type WorkerRunners struct {
	Harness    []runner.Runner
	Servers    []*runner.ServerProcess
	Comparator compare.Comparator
	Filter     *runner.FilterProcess
}

// workerState holds per-worker mutable state that must not be shared between goroutines.
// comparator and filter are per-worker: a harness comparator is bound to a
// specific worker's target SHM regions, and a per-worker filter avoids
// serialising every worker through a single filter process. filter may be nil.
type workerState struct {
	id            int
	runners       []runner.Runner
	serverRunners []*runner.ServerProcess
	comparator    compare.Comparator
	filter        *runner.FilterProcess
	mutator       *Mutator
	rng           *rand.Rand
	// dedup skips inputs that this worker has already executed. False
	// positives are tolerable; they cost at most a few re-executions.
	dedup *bloom
	// stuckExecs counts iterations since this worker last added a new
	// corpus entry. Used to ramp up the splice rate when havoc stops
	// finding new edges.
	stuckExecs int
}

// Coordinator drives the fuzzing campaign.
type Coordinator struct {
	cfg     *config.Config
	workers []workerState
	corpus  *Corpus
	stats   *Stats

	covMu        sync.Mutex // protects globalCov and perTargetCov
	globalCov    []byte
	perTargetCov map[string][]byte

	findingMu     sync.Mutex // protects findingCovs and findingsCount
	findingCovs   map[[32]byte]bool
	findingsCount int

	warmupRounds   int
	validateRounds int
	maxFindings    int

	// stopAfterExecs caps the number of fuzz inputs each parallel worker will
	// execute before it returns. Zero means no per-worker cap. The counter is
	// strictly per-worker (no shared lock); total execs across the campaign
	// are roughly stopAfterExecs * numWorkers.
	stopAfterExecs int
	// stopAfterDuration wall-clocks the entire campaign on top of any
	// configured [campaign].timeout. Zero means no extra cap.
	stopAfterDuration time.Duration

	// seed is the base value used to derive every worker's mutator and rng
	// seeds. Defaults to wall-clock time; SetSeed overrides it for
	// reproducible runs (tests, bug repros).
	seed int64
}

// NewCoordinator creates a coordinator for the given config and worker sets.
// Each WorkerRunners in workerSets carries its own isolated set of target
// processes, its own comparator, and its own input filter (which may be nil);
// the worker runs the fuzzing loop concurrently with the others, sharing the
// corpus and global coverage bitmap.
func NewCoordinator(cfg *config.Config, workerSets []WorkerRunners) (*Coordinator, error) {
	seed := time.Now().UnixNano()

	dict, err := buildDict(cfg)
	if err != nil {
		return nil, err
	}

	workers := make([]workerState, len(workerSets))
	for i, ws := range workerSets {
		workers[i] = workerState{
			id:            i,
			runners:       ws.Harness,
			serverRunners: ws.Servers,
			comparator:    ws.Comparator,
			filter:        ws.Filter,
			mutator:       NewMutator(seed+int64(i), cfg.Campaign.MaxInputSize, dict),
			rng:           rand.New(rand.NewSource(seed + int64(i) + 1)),
			dedup:         newBloom(),
		}
	}
	return &Coordinator{
		cfg:          cfg,
		workers:      workers,
		corpus:       NewCorpus(cfg.Corpus.SeedDir, cfg.Corpus.CorpusDir),
		stats:        NewStats(),
		globalCov:    make([]byte, coverage.BitmapSize),
		perTargetCov: make(map[string][]byte),
		findingCovs:  make(map[[32]byte]bool),
		seed:         seed,
	}, nil
}

// buildDict assembles the mutator dictionary from three sources, in order:
// comparator-derived defaults, the optional [campaign] dict_file, and inline
// [campaign] dicts entries. Returns a non-nil *Dict (possibly empty).
func buildDict(cfg *config.Config) (*Dict, error) {
	d := DefaultDictForComparator(cfg.Comparator.Type)
	if cfg.Campaign.DictFile != "" {
		if err := d.LoadFile(cfg.Campaign.DictFile); err != nil {
			return nil, fmt.Errorf("dict: %w", err)
		}
	}
	for _, tok := range cfg.Campaign.Dicts {
		d.Add([]byte(tok))
	}
	return d, nil
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

// SetDebugEdge enables per-target edge counts in the status ticker output.
func (c *Coordinator) SetDebugEdge(enabled bool) {
	c.stats.SetDebugEdge(enabled)
}

// SetStopAfter configures early-termination conditions for the campaign.
// execsPerWorker > 0 caps each parallel worker at that many executed inputs
// (the counter is per-worker, with no shared synchronisation, so the total
// across N workers is roughly execsPerWorker * N). duration > 0 layers an
// additional wall-clock timeout on top of [campaign].timeout — whichever
// signal fires first wins. Either or both may be zero.
func (c *Coordinator) SetStopAfter(execsPerWorker int, duration time.Duration) {
	c.stopAfterExecs = execsPerWorker
	c.stopAfterDuration = duration
}

// SetSeed overrides the base seed used to derive each worker's mutator and
// rng. Intended for tests and bug reproduction; in normal use the wall-clock
// default chosen in NewCoordinator should stand.
func (c *Coordinator) SetSeed(seed int64) {
	c.seed = seed
	dict := c.workers[0].mutator.dict
	for i := range c.workers {
		c.workers[i].mutator = NewMutator(seed+int64(i), c.cfg.Campaign.MaxInputSize, dict)
		c.workers[i].rng = rand.New(rand.NewSource(seed + int64(i) + 1))
	}
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

// Warmup runs every corpus entry rounds times through worker 0's runners to
// pre-seed the global coverage bitmap before the main fuzzing loop begins.
func (c *Coordinator) Warmup(ctx context.Context, rounds int) error {
	return c.warmupWorker(ctx, &c.workers[0], rounds)
}

func (c *Coordinator) warmupWorker(ctx context.Context, w *workerState, rounds int) error {
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
			_, _, cov, err := c.executeAll(w, input)
			if err != nil {
				return fmt.Errorf("warmup exec: %w", err)
			}
			coverage.Bucketize(cov)
			c.covMu.Lock()
			coverage.Merge(c.globalCov, cov)
			c.covMu.Unlock()
		}
	}
	c.covMu.Lock()
	bits := coverage.CountBits(c.globalCov)
	c.covMu.Unlock()
	fmt.Printf("Warmup complete. Coverage bits: %d\n", bits)
	return nil
}

// Run executes the fuzzing campaign until the context is cancelled or timeout.
// It spawns one goroutine per worker, all sharing the same corpus and global coverage bitmap.
func (c *Coordinator) Run(ctx context.Context) error {
	if err := c.corpus.Load(); err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}
	if err := os.MkdirAll(c.cfg.Corpus.FindingsDir, 0755); err != nil {
		return fmt.Errorf("create findings dir: %w", err)
	}

	if c.corpus.Len() == 0 {
		c.corpus.Add([]byte(""))
	}

	numWorkers := len(c.workers)
	fmt.Printf("Starting campaign %q with %d worker(s), %d harness + %d server targets each, %d seed inputs\n",
		c.cfg.Campaign.Name, numWorkers,
		len(c.workers[0].runners), len(c.workers[0].serverRunners),
		c.corpus.Len())
	fmt.Printf("Seed: %d\n", c.seed)

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
	// --stop-after duration: layered on top of [campaign].timeout so the
	// earliest signal wins.
	if c.stopAfterDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.stopAfterDuration)
		defer cancel()
	}

	// workerCtx lets any worker cancel all others (e.g. on max-findings).
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)
	for i := range c.workers {
		wg.Add(1)
		go func(w *workerState) {
			defer wg.Done()
			if err := c.runWorker(workerCtx, cancelWorkers, w); err != nil {
				errCh <- err
			}
		}(&c.workers[i])
	}

	wg.Wait()
	close(errCh)

	snap := c.stats.Snapshot()
	fmt.Printf("\n\nCampaign finished. Total execs: %d, Rejected: %d, Corpus: %d, Findings: %d, Crashes: %d, Timeouts: %d\n",
		snap.TotalExecs, snap.Rejected, c.corpus.Len(), c.findingsCount, snap.Crashes, snap.Timeouts)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// runWorker is the inner fuzzing loop executed by a single worker goroutine.
// It shares the corpus and globalCov with sibling workers via the coordinator.
func (c *Coordinator) runWorker(ctx context.Context, cancel context.CancelFunc, w *workerState) error {
	const (
		spliceRateHot   = 15 // probability denom while havoc still produces new edges
		spliceRateStuck = 5  // … when we have not added a corpus entry for a while
		stuckThreshold  = 5000
	)

	// Per-worker exec cap from --stop-after <N>. Strictly local — no shared
	// counter, no lock — so N workers run roughly N*cap total inputs.
	var execsThisWorker int

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		// --stop-after <N>: every continue path (dedup hit, filter reject,
		// finding-cov dedup) bypasses any check at the bottom of the loop,
		// so the cap is enforced at the top before each new iteration.
		if c.stopAfterExecs > 0 && execsThisWorker >= c.stopAfterExecs {
			return nil
		}

		// Generate input: pick a base via the power schedule, mostly mutate,
		// occasionally splice. spliceRate ramps up once the worker has not
		// added a corpus entry in a while — havoc has plateaued, so cross
		// pollination from another seed is more likely to break out.
		spliceRate := spliceRateHot
		if w.stuckExecs > stuckThreshold {
			spliceRate = spliceRateStuck
		}

		var input []byte
		var fromSplice bool
		parent := c.corpus.PickWeighted(w.rng)
		var base []byte
		if parent != nil {
			base = parent.Data
		}
		if c.corpus.Len() > 1 && w.rng.Intn(spliceRate) == 0 {
			other := c.corpus.PickRandom(w.rng)
			var otherData []byte
			if other != nil {
				otherData = other.Data
			}
			input = w.mutator.Splice(base, otherData)
			fromSplice = true
		} else {
			input = w.mutator.Mutate(base)
		}

		// Skip inputs this worker has already executed. Splice products
		// bypass the check — they are more likely to be genuinely novel,
		// and getting them in front of the comparator early matters more
		// than saving a duplicate exec.
		if !fromSplice && w.dedup.CheckAndAdd(input) {
			c.stats.RecordDuplicate()
			continue
		}

		// Run this worker's input filter (if configured) before sending to targets.
		if w.filter != nil {
			accepted, transformed, err := w.filter.Filter(input)
			if err != nil {
				fmt.Printf("\nFilter error: %v\n", err)
				continue
			}
			if !accepted {
				c.stats.RecordRejected()
				continue
			}
			if transformed != nil {
				input = transformed
			}
		}

		// Snapshot the mutation ops fired by this iteration before any
		// subsequent mutator call (e.g. via Splice in another loop) clobbers
		// them. Splice clears LastOps, so for splice products this is empty
		// and the bandit gets no signal.
		opsThisIter := append([]int(nil), w.mutator.LastOps()...)

		// Execute on all targets and time the round. Per-target timings are
		// folded into a mean exec time used by the power schedule.
		execStart := time.Now()
		outputs, perTargetCov, combinedCov, execErr := c.executeAll(w, input)
		execElapsed := time.Since(execStart)
		if execErr != nil {
			if !errors.Is(execErr, errSkipIteration) {
				fmt.Printf("\nExec error: %v\n", execErr)
			}
			continue
		}

		c.stats.RecordExec()
		w.stuckExecs++
		execsThisWorker++

		// Check for new coverage. Re-run to filter out flaky edges —
		// Go's runtime/coverage instrumentation still emits a small
		// amount of noise on GC/scheduler paths even after the
		// harness-side noise mask, so we accept only bits that show up
		// in every verification run before claiming new coverage.
		coverage.Bucketize(combinedCov)
		c.covMu.Lock()
		isNew := coverage.HasNewBits(c.globalCov, combinedCov)
		c.covMu.Unlock()

		// Credit the bandit. New-edge → reward 1, otherwise 0.
		if len(opsThisIter) > 0 {
			reward := 0.0
			if isNew {
				reward = 1.0
			}
			w.mutator.Reward(opsThisIter, reward)
		}

		if isNew {
			stable := true
			if c.validateRounds > 0 {
				if unstable := validateStability(w.runners, input, c.validateRounds); len(unstable) > 0 {
					fmt.Printf("\n[UNSTABLE] input (%d bytes) discarded — targets with non-deterministic output: %v\n",
						len(input), unstable)
					stable = false
				}
			}
			if stable {
				c.covMu.Lock()
				newEdges := coverage.CountBits(combinedCov)
				coverage.Merge(c.globalCov, combinedCov)
				c.covMu.Unlock()

				if s := c.corpus.Add(input); s != nil {
					// Skew = stddev/mean of per-target edge counts.
					// Inputs with high skew exercise some implementations
					// much more than others — exactly the inputs whose
					// neighbourhood we want to re-explore for diff bugs.
					c.corpus.Annotate(s, newEdges, execElapsed.Nanoseconds(),
						perTargetSkew(perTargetCov))
					w.stuckExecs = 0
					if err := c.corpus.Save(input); err != nil {
						fmt.Printf("\n[WARN] failed to save corpus entry: %v\n", err)
					}
				}
			}
		}

		// Compare outputs across targets using this worker's comparator.
		if disc := w.comparator.Compare(input, outputs); disc != nil {
			covKey := sha256.Sum256(combinedCov)
			c.findingMu.Lock()
			if c.findingCovs[covKey] {
				c.findingMu.Unlock()
				continue
			}
			c.findingCovs[covKey] = true
			c.findingsCount++
			findingID := c.findingsCount
			shouldStop := c.maxFindings > 0 && c.findingsCount >= c.maxFindings
			c.findingMu.Unlock()

			// Parent seed produced an oracle-positive child — boost its
			// energy so the schedule re-explores its neighbourhood.
			c.corpus.BumpDivergence(parent, 1.0)

			minimized, minDisc := Minimize(disc.Input, w.runners, w.comparator)
			if minDisc != nil {
				disc = minDisc
			} else {
				disc.Input = minimized
			}
			if err := c.saveFinding(disc, findingID); err != nil {
				fmt.Printf("\n[WARN] failed to save finding #%d: %v\n", findingID, err)
			}
			fmt.Printf("\n[FINDING #%d] %s (input: %d bytes)\n", findingID, disc.Description, len(disc.Input))
			if shouldStop {
				fmt.Printf("\nMax findings (%d) reached. Stopping.\n", c.maxFindings)
				cancel()
				return nil
			}
		}

		// Update per-target coverage accumulator and refresh stats.
		c.covMu.Lock()
		for name, cov := range perTargetCov {
			if acc, ok := c.perTargetCov[name]; ok {
				coverage.Merge(acc, cov)
			} else {
				acc := make([]byte, coverage.BitmapSize)
				coverage.Merge(acc, cov)
				c.perTargetCov[name] = acc
			}
		}
		covBits := coverage.CountBits(c.globalCov)
		targetEdges := make(map[string]int, len(c.perTargetCov))
		for name, acc := range c.perTargetCov {
			targetEdges[name] = coverage.CountBits(acc)
		}
		c.covMu.Unlock()

		c.findingMu.Lock()
		findingsSnapshot := c.findingsCount
		c.findingMu.Unlock()

		c.stats.Update(c.corpus.Len(), covBits, findingsSnapshot, targetEdges)
		c.stats.PrintIfDue()
	}
}

// errSkipIteration is returned by executeAll when a crash or timeout was
// detected, saved, and the iteration should be skipped without aborting the campaign.
var errSkipIteration = errors.New("skip iteration")

// perTargetSkew returns the coefficient of variation (stddev / mean) of edge
// counts across targets. Zero when all targets exercise the same number of
// edges; rises as one implementation hits paths the others miss. Used as a
// differential-fuzzing energy multiplier in the power schedule.
func perTargetSkew(perTargetCov map[string][]byte) float64 {
	if len(perTargetCov) < 2 {
		return 0
	}
	counts := make([]float64, 0, len(perTargetCov))
	for _, cov := range perTargetCov {
		counts = append(counts, float64(coverage.CountBits(cov)))
	}
	var sum float64
	for _, v := range counts {
		sum += v
	}
	mean := sum / float64(len(counts))
	if mean <= 0 {
		return 0
	}
	var ss float64
	for _, v := range counts {
		d := v - mean
		ss += d * d
	}
	std := math.Sqrt(ss / float64(len(counts)))
	return std / mean
}

// executeAll runs input through all harness targets and collects coverage
// from server targets. Returns outputs from harness runners, a per-target
// coverage map, and the merged raw (un-bucketized) coverage bitmap from all targets.
// On crash or timeout, saves a finding and returns errSkipIteration.
func (c *Coordinator) executeAll(w *workerState, input []byte) (map[string][]byte, map[string][]byte, []byte, error) {
	// Reset server coverage bitmaps before the harness runs so we only
	// capture edges from this iteration. Harness runners reset themselves
	// inside Execute().
	for _, s := range w.serverRunners {
		s.ResetCoverage()
	}

	// Execute harness runners via the pipe protocol.
	outputs := make(map[string][]byte, len(w.runners))
	perTargetCov := make(map[string][]byte, len(w.runners)+len(w.serverRunners))
	combined := make([]byte, coverage.BitmapSize)
	for _, r := range w.runners {
		output, cov, err := r.Execute(input)
		if err != nil {
			var te *runner.TimeoutError
			var ce *runner.CrashError
			switch {
			case errors.As(err, &te):
				c.stats.RecordTimeout()
				fmt.Printf("\n[TIMEOUT] %s (%d-byte input)\n", te.TargetName, len(input))
				if saveErr := c.saveSpecialFinding("timeout", te.TargetName, input); saveErr != nil {
					fmt.Printf("[WARN] failed to save timeout finding: %v\n", saveErr)
				}
				return nil, nil, nil, errSkipIteration
			case errors.As(err, &ce):
				c.stats.RecordCrash()
				fmt.Printf("\n[CRASH] %s (%d-byte input): %v\n", ce.TargetName, len(input), ce.ExitState)
				if saveErr := c.saveSpecialFinding("crash", ce.TargetName, input); saveErr != nil {
					fmt.Printf("[WARN] failed to save crash finding: %v\n", saveErr)
				}
				return nil, nil, nil, errSkipIteration
			default:
				return nil, nil, nil, fmt.Errorf("target %s: %w", r.Name(), err)
			}
		}
		outputs[r.Name()] = output
		perTargetCov[r.Name()] = cov
		coverage.Merge(combined, cov)
	}

	// Read coverage accumulated by server targets while the harness ran.
	for _, s := range w.serverRunners {
		_, cov, err := s.Execute(input)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("server target %s: %w", s.Name(), err)
		}
		perTargetCov[s.Name()] = cov
		coverage.Merge(combined, cov)
	}

	return outputs, perTargetCov, combined, nil
}

// saveSpecialFinding writes a crash or timeout input to the findings directory.
func (c *Coordinator) saveSpecialFinding(kind, targetName string, input []byte) error {
	h := sha256.Sum256(input)
	dirName := fmt.Sprintf("%s_%s_%x", kind, targetName, h[:6])
	dir := filepath.Join(c.cfg.Corpus.FindingsDir, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create finding dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "input.bin"), input, 0644); err != nil {
		return fmt.Errorf("write input.bin: %w", err)
	}
	type metadata struct {
		Kind      string `json:"kind"`
		Target    string `json:"target"`
		Hash      string `json:"hash"`
		InputLen  int    `json:"input_len"`
		Timestamp string `json:"timestamp"`
	}
	meta := metadata{
		Kind:      kind,
		Target:    targetName,
		Hash:      fmt.Sprintf("%x", h),
		InputLen:  len(input),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0644)
}

func (c *Coordinator) saveFinding(disc *compare.Discrepancy, id int) error {
	h := sha256.Sum256(disc.Input)
	dirName := fmt.Sprintf("%x", h[:8])
	dir := filepath.Join(c.cfg.Corpus.FindingsDir, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create finding dir %s: %w", dir, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "input.bin"), disc.Input, 0644); err != nil {
		return fmt.Errorf("write input.bin: %w", err)
	}
	for name, output := range disc.Outputs {
		path := filepath.Join(dir, fmt.Sprintf("output_%s.bin", name))
		if err := os.WriteFile(path, output, 0644); err != nil {
			return fmt.Errorf("write output for %s: %w", name, err)
		}
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
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0644); err != nil {
		return fmt.Errorf("write metadata.json: %w", err)
	}
	return nil
}
