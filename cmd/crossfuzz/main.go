package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/engine"
	"crossfuzz/pkg/runner"
)

// Persistent (root-level) flag values shared across all subcommands.
var (
	flagName      string
	flagTimeout   string
	flagMaxMemory string
)

var rootCmd = &cobra.Command{
	Use:   "crossfuzz",
	Short: "Coverage-guided differential fuzzer",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagName, "name", "", "Comma-separated list of target names to build/run (default: all)")
	rootCmd.PersistentFlags().StringVar(&flagTimeout, "timeout", "5s", "Per-execution timeout; target is killed and restarted on expiry (e.g. 5s, 500ms)")
	rootCmd.PersistentFlags().StringVar(&flagMaxMemory, "max-memory", "0", "Virtual memory limit per target process (e.g. 512M, 1G); 0 = no limit")

	rootCmd.AddCommand(buildCmd())
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(reduceCmd())
	rootCmd.AddCommand(analyzeCmd())
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// loadConfig loads the TOML config from args[0], applies the --name filter and
// --timeout override, and returns the config plus the parsed memory limit.
func loadConfig(cmd *cobra.Command, args []string) (*config.Config, uint64, error) {
	cfg, err := config.Load(args[0])
	if err != nil {
		return nil, 0, fmt.Errorf("error loading config: %w", err)
	}

	if flagName != "" {
		cfg.Targets = filterTargets(cfg.Targets, flagName)
		if len(cfg.Targets) == 0 {
			return nil, 0, fmt.Errorf("no targets matched --name=%s", flagName)
		}
	}

	execTimeout, err := time.ParseDuration(flagTimeout)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid --timeout %q: %w", flagTimeout, err)
	}
	cfg.Campaign.ExecTimeout.Duration = execTimeout

	memLimit, err := parseBytes(flagMaxMemory)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid --max-memory %q: %w", flagMaxMemory, err)
	}

	return cfg, memLimit, nil
}

func buildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build <config.toml>",
		Short: "Build all targets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig(cmd, args)
			if err != nil {
				return err
			}
			cmdBuild(cfg)
			return nil
		},
	}
}

func runCmd() *cobra.Command {
	var (
		build       bool
		warmup      int
		maxFindings int
		validate    int
		corpus      string
		findings    string
		debugEdge   bool
		logFile     string
		workers     int
	)

	cmd := &cobra.Command{
		Use:   "run <config.toml>",
		Short: "Run differential fuzzing campaign",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, memLimit, err := loadConfig(cmd, args)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("corpus") || cfg.Corpus.CacheDir == "" {
				cfg.Corpus.CacheDir = corpus
			}
			if cmd.Flags().Changed("findings") || cfg.Corpus.FindingsDir == "" {
				cfg.Corpus.FindingsDir = findings
			}
			cmdRun(cfg, warmup, validate, maxFindings, debugEdge, memLimit, workers, build, logFile)
			return nil
		},
	}

	cmd.Flags().BoolVar(&build, "build", false, "Build all targets before running")
	cmd.Flags().IntVar(&warmup, "warmup", 0, "Number of times to run the corpus before the main fuzzing loop")
	cmd.Flags().IntVar(&maxFindings, "max-findings", 10, "Stop after this many unique findings")
	cmd.Flags().IntVar(&validate, "validate", 0, "Re-execute each new input N times to confirm stable output; log unstable inputs with differing targets")
	cmd.Flags().StringVar(&corpus, "corpus", "corpus", "Directory for storing and loading corpus entries")
	cmd.Flags().StringVar(&findings, "findings", "findings", "Directory for saving findings")
	cmd.Flags().BoolVar(&debugEdge, "debug-edge", false, "Print per-target edge counts in status ticker")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Also write all stdout output to this file")
	cmd.Flags().IntVar(&workers, "workers", 1, "Number of parallel fuzzing workers, each with their own target processes")

	return cmd
}

func reduceCmd() *cobra.Command {
	var (
		build         bool
		corpus        string
		corpusReduced string
		validate      int
	)

	cmd := &cobra.Command{
		Use:   "reduce <config.toml>",
		Short: "Deduplicate corpus by coverage profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig(cmd, args)
			if err != nil {
				return err
			}
			if build {
				cmdBuild(cfg)
			}
			if cmd.Flags().Changed("corpus") || cfg.Corpus.CacheDir == "" {
				cfg.Corpus.CacheDir = corpus
			}
			cmdReduce(cfg, corpusReduced, validate)
			return nil
		},
	}

	cmd.Flags().BoolVar(&build, "build", false, "Build all targets before reducing")
	cmd.Flags().StringVar(&corpus, "corpus", "corpus", "Directory for loading corpus entries")
	cmd.Flags().StringVar(&corpusReduced, "corpus-reduced", "corpus-reduced", "Output directory for reduced corpus")
	cmd.Flags().IntVar(&validate, "validate", 0, "Re-execute each new input N times to confirm stable output; log unstable inputs with differing targets")

	return cmd
}

func analyzeCmd() *cobra.Command {
	var (
		build       bool
		payload     string
		payloadPath string
	)

	cmd := &cobra.Command{
		Use:   "analyze <config.toml>",
		Short: "Run a payload against all targets and print hex output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig(cmd, args)
			if err != nil {
				return err
			}
			if build {
				cmdBuild(cfg)
			}
			if payload == "" && payloadPath == "" {
				return fmt.Errorf("analyze requires --payload or --payload-path")
			}
			cmdAnalyze(cfg, payload, payloadPath)
			return nil
		},
	}

	cmd.Flags().BoolVar(&build, "build", false, "Build all targets before analyzing")
	cmd.Flags().StringVar(&payload, "payload", "", "Payload string to send to all targets")
	cmd.Flags().StringVar(&payloadPath, "payload-path", "", "File or directory of payloads to send")

	return cmd
}

func filterTargets(targets []config.TargetConfig, nameList string) []config.TargetConfig {
	names := make(map[string]bool)
	for _, n := range strings.Split(nameList, ",") {
		if t := strings.TrimSpace(n); t != "" {
			names[t] = true
		}
	}
	var filtered []config.TargetConfig
	for _, tc := range targets {
		if names[tc.Name] {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

func cmdBuild(cfg *config.Config) {
	for _, tc := range cfg.Targets {
		if tc.BuildCmd == "" {
			fmt.Printf("Skipping %s (no build_cmd)\n", tc.Name)
			continue
		}
		fmt.Printf("Building %s: %s\n", tc.Name, tc.BuildCmd)
		cmd := exec.Command("sh", "-c", tc.BuildCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed for %s: %v\n", tc.Name, err)
			os.Exit(1)
		}
	}
	if cfg.InputFilter.BuildCmd != "" {
		fmt.Printf("Building input_filter: %s\n", cfg.InputFilter.BuildCmd)
		cmd := exec.Command("sh", "-c", cfg.InputFilter.BuildCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed for input_filter: %v\n", err)
			os.Exit(1)
		}
	}
	if cfg.Comparator.BuildCmd != "" {
		fmt.Printf("Building comparator: %s\n", cfg.Comparator.BuildCmd)
		cmd := exec.Command("sh", "-c", cfg.Comparator.BuildCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed for comparator: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("Build complete.")
}

func buildRunners(cfg *config.Config, memLimit uint64, workerID int) ([]runner.Runner, []*runner.ServerProcess) {
	workerEnv := []string{fmt.Sprintf("CROSSFUZZ_ID=%d", workerID)}
	var harness []runner.Runner
	var servers []*runner.ServerProcess
	for _, tc := range cfg.Targets {
		if tc.IsServer() {
			r, err := runner.NewServerProcess(runner.ProcessConfig{
				Name:   tc.Name,
				Binary: tc.Binary,
				Args:   tc.Args,
				Env:    append(workerEnv, tc.Env...),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating server runner %s: %v\n", tc.Name, err)
				os.Exit(1)
			}
			servers = append(servers, r)
		} else {
			r, err := runner.NewProcess(runner.ProcessConfig{
				Name:          tc.Name,
				Binary:        tc.Binary,
				Args:          tc.Args,
				Env:           append(workerEnv, tc.Env...),
				Timeout:       cfg.Campaign.ExecTimeout.Duration,
				MemLimitBytes: memLimit,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating runner %s: %v\n", tc.Name, err)
				os.Exit(1)
			}
			harness = append(harness, r)
		}
	}
	return harness, servers
}

// allRunners combines harness and server runners into a single slice for
// use with startRunners/stopRunners, which only need the Runner interface.
func allRunners(harness []runner.Runner, servers []*runner.ServerProcess) []runner.Runner {
	all := make([]runner.Runner, len(harness), len(harness)+len(servers))
	copy(all, harness)
	for _, s := range servers {
		all = append(all, s)
	}
	return all
}

func startRunners(runners []runner.Runner) {
	for _, r := range runners {
		if err := r.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting %s: %v\n", r.Name(), err)
			for _, r2 := range runners {
				r2.Stop()
			}
			os.Exit(1)
		}
		fmt.Printf("Started target: %s\n", r.Name())
	}
}

func stopRunners(runners []runner.Runner) {
	for _, r := range runners {
		r.Stop()
	}
}

// setupLogFile redirects os.Stdout to a tee that writes to both the original
// stdout and the named file. The returned cleanup function must be called
// (typically via defer) to flush and close the log file.
func setupLogFile(path string) func() {
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file %q: %v\n", path, err)
		os.Exit(1)
	}
	r, w, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating log pipe: %v\n", err)
		os.Exit(1)
	}
	origStdout := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(io.MultiWriter(origStdout, f), r) //nolint:errcheck
	}()
	return func() {
		w.Close()
		<-done
		r.Close()
		f.Close()
	}
}

func cmdRun(cfg *config.Config, warmup int, validate int, maxFindings int, debugEdge bool, memLimit uint64, numWorkers int, build bool, logFile string) {
	if numWorkers < 1 {
		fmt.Fprintf(os.Stderr, "--workers must be at least 1\n")
		os.Exit(1)
	}

	cleanup := setupLogFile(logFile)
	defer cleanup()

	// Build targets once before spawning any workers.
	if build {
		cmdBuild(cfg)
	}

	// Build one independent set of target processes per worker.
	workerSets := make([]engine.WorkerRunners, numWorkers)
	var allFlat []runner.Runner
	for i := range workerSets {
		harness, servers := buildRunners(cfg, memLimit, i)
		workerSets[i] = engine.WorkerRunners{Harness: harness, Servers: servers}
		allFlat = append(allFlat, allRunners(harness, servers)...)
	}

	startRunners(allFlat)
	defer stopRunners(allFlat)

	// Collect SHM paths from the first worker set (all workers have the same
	// target names; the comparator only needs one set of paths).
	targetSHMs := make(map[string]string)
	if len(workerSets) > 0 {
		ws := workerSets[0]
		for _, r := range ws.Harness {
			if p := r.SHMPath(); p != "" {
				targetSHMs[r.Name()] = p
			}
		}
		for _, s := range ws.Servers {
			if p := s.SHMPath(); p != "" {
				targetSHMs[s.Name()] = p
			}
		}
	}

	var comp compare.Comparator
	switch cfg.Comparator.Type {
	case "byte_equal", "":
		comp = compare.ByteEqual{}
	case "json_structural":
		comp = compare.JSONStructural{}
	case "numeric":
		comp = compare.Numeric{}
	case "numeric_relative":
		comp = compare.Numeric{Relative: true}
	case "none":
		comp = compare.NoOp{}
	case "custom":
		if cfg.Comparator.Script == "" {
			fmt.Fprintf(os.Stderr, "Comparator type 'custom' requires a script path\n")
			os.Exit(1)
		}
		comp = compare.Custom{Script: cfg.Comparator.Script}
	case "harness":
		if cfg.Comparator.Binary == "" {
			fmt.Fprintf(os.Stderr, "Comparator type 'harness' requires a binary path\n")
			os.Exit(1)
		}
		cmpProc, err := runner.NewCompareProcess(runner.ProcessConfig{
			Name:   "comparator",
			Binary: cfg.Comparator.Binary,
			Args:   cfg.Comparator.Args,
			Env:    cfg.Comparator.Env,
		}, targetSHMs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating comparator: %v\n", err)
			os.Exit(1)
		}
		if err := cmpProc.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting comparator: %v\n", err)
			os.Exit(1)
		}
		defer cmpProc.Stop()
		fmt.Println("Started comparator harness.")
		comp = compare.Harness{Proc: cmpProc}
	default:
		fmt.Fprintf(os.Stderr, "Unknown comparator type: %s\n", cfg.Comparator.Type)
		os.Exit(1)
	}

	// Start the input filter process if configured.
	var filter *runner.FilterProcess
	if cfg.InputFilter.Binary != "" {
		var err error
		filter, err = runner.NewFilterProcess(runner.ProcessConfig{
			Name:   "input_filter",
			Binary: cfg.InputFilter.Binary,
			Args:   cfg.InputFilter.Args,
			Env:    cfg.InputFilter.Env,
		}, cfg.InputFilter.Transform)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating input filter: %v\n", err)
			os.Exit(1)
		}
		if err := filter.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting input filter: %v\n", err)
			os.Exit(1)
		}
		defer filter.Stop()
		fmt.Println("Started input filter.")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	coord := engine.NewCoordinator(cfg, workerSets, comp, filter)
	coord.SetWarmupRounds(warmup)
	coord.SetValidateRounds(validate)
	coord.SetMaxFindings(maxFindings)
	coord.SetDebugEdge(debugEdge)
	if err := coord.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Campaign error: %v\n", err)
		os.Exit(1)
	}
}

func cmdReduce(cfg *config.Config, outDir string, validate int) {
	harness, servers := buildRunners(cfg, 0, 0)
	all := allRunners(harness, servers)
	startRunners(all)
	defer stopRunners(all)
	runners := all

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("Reducing corpus in %q...\n", cfg.Corpus.CacheDir)
	result, err := engine.Reduce(ctx, cfg, runners, validate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reduce error: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}

	for _, input := range result.Kept {
		h := sha256.Sum256(input)
		name := fmt.Sprintf("%x", h[:8])
		if err := os.WriteFile(filepath.Join(outDir, name), input, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing corpus entry: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Reduced %d → %d entries (saved to %q)\n", result.Total, len(result.Kept), outDir)
}

func cmdAnalyze(cfg *config.Config, payload string, payloadPath string) {
	harness, servers := buildRunners(cfg, 0, 0)
	all := allRunners(harness, servers)
	startRunners(all)
	defer stopRunners(all)

	type namedPayload struct {
		name string
		data []byte
	}
	var payloads []namedPayload

	if payload != "" {
		payloads = append(payloads, namedPayload{name: "<payload>", data: []byte(payload)})
	}

	if payloadPath != "" {
		info, err := os.Stat(payloadPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error accessing payload path: %v\n", err)
			os.Exit(1)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(payloadPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading payload directory: %v\n", err)
				os.Exit(1)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				p := filepath.Join(payloadPath, entry.Name())
				data, err := os.ReadFile(p)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", p, err)
					os.Exit(1)
				}
				payloads = append(payloads, namedPayload{name: entry.Name(), data: data})
			}
		} else {
			data, err := os.ReadFile(payloadPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading payload file: %v\n", err)
				os.Exit(1)
			}
			payloads = append(payloads, namedPayload{name: filepath.Base(payloadPath), data: data})
		}
	}

	if len(payloads) == 0 {
		fmt.Fprintln(os.Stderr, "No payloads to run.")
		os.Exit(1)
	}

	type result struct {
		name   string
		output []byte
		err    error
	}

	for _, p := range payloads {
		fmt.Printf("=== Payload: %s (%d bytes) ===\n", p.name, len(p.data))
		fmt.Printf("Input:\n%s\n", hex.Dump(p.data))

		// Collect all outputs before printing so we can compute the diff mask.
		results := make([]result, len(all))
		for i, r := range all {
			out, _, err := r.Execute(p.data)
			results[i] = result{name: r.Name(), output: out, err: err}
		}

		// Build mask over successful outputs only.
		var successful [][]byte
		for _, res := range results {
			if res.err == nil {
				successful = append(successful, res.output)
			}
		}
		mask := diffMask(successful)

		for _, res := range results {
			fmt.Printf("--- Target: %s ---\n", res.name)
			if res.err != nil {
				fmt.Printf("Error: %v\n", res.err)
			} else {
				fmt.Print(colorHexDump(res.output, mask))
			}
		}
		fmt.Println()
	}
}

// parseBytes parses a human-readable byte count with optional suffix:
// K/k = kibibytes, M/m = mebibytes, G/g = gibibytes. Returns 0 for "0" or "".
func parseBytes(s string) (uint64, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	mult := uint64(1)
	trimmed := s
	switch last := s[len(s)-1]; last {
	case 'K', 'k':
		mult = 1 << 10
		trimmed = s[:len(s)-1]
	case 'M', 'm':
		mult = 1 << 20
		trimmed = s[:len(s)-1]
	case 'G', 'g':
		mult = 1 << 30
		trimmed = s[:len(s)-1]
	}
	n, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: must be a number with optional K/M/G suffix", s)
	}
	return n * mult, nil
}
