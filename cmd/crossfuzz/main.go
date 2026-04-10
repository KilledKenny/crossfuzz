package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"crossfuzz/pkg/compare"
	"crossfuzz/pkg/config"
	"crossfuzz/pkg/engine"
	"crossfuzz/pkg/runner"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: crossfuzz <command> <config.toml>\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  build   Build all targets\n")
		fmt.Fprintf(os.Stderr, "  run     Run differential fuzzing campaign\n")
		os.Exit(1)
	}

	cfg, err := config.Load(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		cmdBuild(cfg)
	case "run":
		cmdRun(cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
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
