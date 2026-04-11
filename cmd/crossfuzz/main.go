package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/engine"
	"crossfuzz/pkg/runner"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: crossfuzz <command> <config.toml> [flags]\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  build   Build all targets\n")
	fmt.Fprintf(os.Stderr, "  run     Run differential fuzzing campaign\n")
	fmt.Fprintf(os.Stderr, "  reduce  Deduplicate corpus by coverage profile\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  --name=fuzz1,fuzz2      Comma-separated list of target names to build/run (default: all)\n")
	fmt.Fprintf(os.Stderr, "  --build                 Build all targets before running (run command only)\n")
	fmt.Fprintf(os.Stderr, "  --warmup=N              Run corpus N times before the main fuzzing loop (run command only)\n")
	fmt.Fprintf(os.Stderr, "  --max-findings=N        Stop after N unique findings (run command only, default: 10)\n")
	fmt.Fprintf(os.Stderr, "  --validate=N            Re-execute each new input N times; log unstable inputs and which targets differ\n")
	fmt.Fprintf(os.Stderr, "  --corpus=DIR            Directory for corpus entries (default: corpus)\n")
	fmt.Fprintf(os.Stderr, "  --findings=DIR          Directory for saving findings (default: findings)\n")
	fmt.Fprintf(os.Stderr, "  --corpus-reduced=DIR    Output directory for reduced corpus (reduce command only, default: corpus-reduced)\n")
	fmt.Fprintf(os.Stderr, "  --debug-edge            Print per-target edge counts in status ticker (run command only)\n")
}

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	nameFlag := fs.String("name", "", "Comma-separated list of target names to build/run (default: all)")
	buildFlag := fs.Bool("build", false, "Build all targets before running (run command only)")
	warmupFlag := fs.Int("warmup", 0, "Number of times to run the corpus before the main fuzzing loop (run command only)")
	maxFindingsFlag := fs.Int("max-findings", 10, "Stop after this many unique findings (run command only)")
	validateFlag := fs.Int("validate", 0, "Re-execute each new input N times to confirm stable output; log unstable inputs with differing targets")
	corpusFlag := fs.String("corpus", "corpus", "Directory for storing and loading corpus entries (run command only)")
	findingsFlag := fs.String("findings", "findings", "Directory for saving findings (run command only)")
	corpusReducedFlag := fs.String("corpus-reduced", "corpus-reduced", "Output directory for reduced corpus (reduce command only)")
	debugEdgeFlag := fs.Bool("debug-edge", false, "Print per-target edge counts in status ticker (run command only)")
	fs.Usage = usage

	// os.Args[2] is the config file; flags follow after
	if err := fs.Parse(os.Args[3:]); err != nil {
		os.Exit(1)
	}

	cfg, err := config.Load(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *nameFlag != "" {
		cfg.Targets = filterTargets(cfg.Targets, *nameFlag)
		if len(cfg.Targets) == 0 {
			fmt.Fprintf(os.Stderr, "No targets matched --name=%s\n", *nameFlag)
			os.Exit(1)
		}
	}

	switch command {
	case "build":
		cmdBuild(cfg)
	case "run":
		if isFlagSet(fs, "corpus") || cfg.Corpus.CacheDir == "" {
			cfg.Corpus.CacheDir = *corpusFlag
		}
		if isFlagSet(fs, "findings") || cfg.Corpus.FindingsDir == "" {
			cfg.Corpus.FindingsDir = *findingsFlag
		}
		if *buildFlag {
			cmdBuild(cfg)
		}
		cmdRun(cfg, *warmupFlag, *validateFlag, *maxFindingsFlag, *debugEdgeFlag)
	case "reduce":
		if isFlagSet(fs, "corpus") || cfg.Corpus.CacheDir == "" {
			cfg.Corpus.CacheDir = *corpusFlag
		}
		cmdReduce(cfg, *corpusReducedFlag, *validateFlag)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
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
	fmt.Println("Build complete.")
}

func buildRunners(cfg *config.Config) ([]runner.Runner, []*runner.ServerProcess) {
	var harness []runner.Runner
	var servers []*runner.ServerProcess
	for _, tc := range cfg.Targets {
		if tc.IsServer() {
			r, err := runner.NewServerProcess(runner.ProcessConfig{
				Name:   tc.Name,
				Binary: tc.Binary,
				Args:   tc.Args,
				Env:    tc.Env,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating server runner %s: %v\n", tc.Name, err)
				os.Exit(1)
			}
			servers = append(servers, r)
		} else {
			r, err := runner.NewProcess(runner.ProcessConfig{
				Name:    tc.Name,
				Binary:  tc.Binary,
				Args:    tc.Args,
				Env:     tc.Env,
				Timeout: cfg.Campaign.ExecTimeout.Duration,
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

func cmdRun(cfg *config.Config, warmup int, validate int, maxFindings int, debugEdge bool) {
	harness, servers := buildRunners(cfg)

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
	default:
		fmt.Fprintf(os.Stderr, "Unknown comparator type: %s\n", cfg.Comparator.Type)
		os.Exit(1)
	}

	all := allRunners(harness, servers)
	startRunners(all)
	defer stopRunners(all)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	coord := engine.NewCoordinator(cfg, harness, servers, comp)
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
	harness, servers := buildRunners(cfg)
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
