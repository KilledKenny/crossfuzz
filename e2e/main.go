// crossfuzz-e2e drives end-to-end tests of the crossfuzz coordinator and
// language harnesses. It supersedes the earlier `go test`-based suite.
//
// Tests register themselves via init() in subpackages under tests/. This
// binary imports tests/all.go for side effects, which transitively imports
// every test package so the registry is populated before main() runs.
//
// Usage:
//
//	bin/crossfuzz-e2e [-run REGEX] [-tag TAG]... [-parallel N] [-v] [-failfast] [-list]
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"

	"github.com/KilledKenny/crossfuzz/e2e/framework"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests"
)

func main() {
	var (
		runRE     = flag.String("run", "", "regex matched against test names; empty = run all")
		tagFlag   tagList
		parallel  = flag.Int("parallel", runtime.NumCPU(), "max concurrent tests")
		verbose   = flag.Bool("v", false, "stream each test's log lines after it finishes")
		failFast  = flag.Bool("failfast", false, "stop dispatching new tests after the first failure")
		list      = flag.Bool("list", false, "print matching test names (and tags) and exit")
		stopPanic = flag.Bool("stop-on-panic", false, "treat a test panic as fatal to the entire run")
		seed      = flag.Int64("seed", framework.DefaultSeed, "default --seed value injected into every `crossfuzz run` invocation. Pass 0 for wall-clock (non-deterministic).")
		flaky     = flag.Int("flaky", 3, "after the first pass, rerun each failed test up to N times (incrementing the seed each retry); any test that passes a retry is reported as flaky, not failed. Pass 0 to disable.")
	)
	flag.Var(&tagFlag, "tag", "only run tests carrying this tag (repeatable). Combined with -run by AND.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	framework.DefaultSeed = *seed

	tests := framework.Tests()
	var re *regexp.Regexp
	if *runRE != "" {
		var err error
		re, err = regexp.Compile(*runRE)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -run regex %q: %v\n", *runRE, err)
			os.Exit(2)
		}
	}

	filtered := tests[:0:0]
	for _, t := range tests {
		if re != nil && !re.MatchString(t.Name) {
			continue
		}
		if len(tagFlag) > 0 {
			matched := true
			for _, tag := range tagFlag {
				if !t.HasTag(tag) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, t)
	}

	if len(filtered) == 0 {
		fmt.Fprintln(os.Stderr, "no tests matched the filters")
		os.Exit(2)
	}

	if *list {
		for _, t := range filtered {
			tags := ""
			if len(t.Tags) > 0 {
				tags = fmt.Sprintf("  [%s]", joinComma(t.Tags))
			}
			fmt.Printf("%s%s\n", t.Name, tags)
		}
		return
	}

	r := framework.Runner{
		Tests:       filtered,
		Parallel:    *parallel,
		Verbose:     *verbose,
		FailFast:    *failFast,
		StopOnPanic: *stopPanic,
		Flaky:       *flaky,
		Out:         os.Stdout,
	}
	fmt.Fprintf(os.Stdout, "running %d tests with up to %d in parallel\n\n", len(filtered), *parallel)
	results := r.Run()
	if framework.AnyFailed(results) {
		os.Exit(1)
	}
}

// tagList is a flag.Value that collects repeated -tag flags.
type tagList []string

func (l *tagList) String() string     { return joinComma(*l) }
func (l *tagList) Set(s string) error { *l = append(*l, s); return nil }

func joinComma(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}
