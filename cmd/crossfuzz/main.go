package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  --name=fuzz1,fuzz2   Comma-separated list of target names to build/run (default: all)\n")
}

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	nameFlag := fs.String("name", "", "Comma-separated list of target names to build/run (default: all)")
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
		cmdRun(cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
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

func cmdRun(cfg *config.Config) {
	var runners []runner.Runner
	for _, tc := range cfg.Targets {
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
		runners = append(runners, r)
	}

	var comp compare.Comparator
	switch cfg.Comparator.Type {
	case "byte_equal", "":
		comp = compare.ByteEqual{}
	default:
		fmt.Fprintf(os.Stderr, "Unknown comparator type: %s\n", cfg.Comparator.Type)
		os.Exit(1)
	}

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
	defer func() {
		for _, r := range runners {
			r.Stop()
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	coord := engine.NewCoordinator(cfg, runners, comp)
	if err := coord.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Campaign error: %v\n", err)
		os.Exit(1)
	}
}
