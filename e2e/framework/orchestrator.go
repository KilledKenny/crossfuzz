package framework

import (
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// Outcome classifies the result of one test run.
type Outcome int

const (
	Passed Outcome = iota
	Failed
	Skipped
	Panicked // panic that wasn't a fatalSignal / skipSignal
	Flaky    // failed initially but passed on a retry (--flaky N)
)

func (o Outcome) String() string {
	switch o {
	case Passed:
		return "ok"
	case Failed:
		return "FAIL"
	case Skipped:
		return "skip"
	case Panicked:
		return "PANIC"
	case Flaky:
		return "FLAKY"
	default:
		return "?"
	}
}

// Result captures everything observable about one test execution.
type Result struct {
	Test     Test
	Outcome  Outcome
	Reason   string        // skip reason or fail message
	Logs     []string      // collected via t.Logf
	Duration time.Duration
}

// Runner orchestrates execution of a slice of tests.
type Runner struct {
	Tests       []Test
	Parallel    int  // max concurrent tests; 1 = strictly serial
	Verbose     bool // stream per-test logs as they finish
	FailFast    bool // stop dispatching after the first failure
	StopOnPanic bool // treat panic as fatal to the whole run
	// Flaky, if > 0, rerun each failed/panicked test up to this many extra
	// times after the first pass. A test that passes any retry is reported
	// as Flaky (not Failed) so the user knows to investigate it as
	// instability rather than a real regression.
	Flaky   int
	Out     io.Writer // progress + summary output
	NowFunc func() time.Time
}

// Run executes every test in r.Tests, returning the results in the order they
// completed. Live progress (one line per test) is written to r.Out as each
// test finishes; a summary follows.
func (r *Runner) Run() []Result {
	if r.Out == nil {
		r.Out = io.Discard
	}
	if r.Parallel < 1 {
		r.Parallel = 1
	}
	if r.NowFunc == nil {
		r.NowFunc = time.Now
	}

	total := len(r.Tests)
	results := make([]Result, 0, total)
	var resMu sync.Mutex
	var done int
	var anyFailed bool

	startAll := r.NowFunc()

	sem := make(chan struct{}, r.Parallel)
	var wg sync.WaitGroup

	stopDispatch := make(chan struct{})
	stopOnce := sync.Once{}
	closeStop := func() { stopOnce.Do(func() { close(stopDispatch) }) }

	for i := range r.Tests {
		t := r.Tests[i]
		select {
		case <-stopDispatch:
			// FailFast: record the rest as skipped-with-reason for visibility.
			resMu.Lock()
			results = append(results, Result{Test: t, Outcome: Skipped, Reason: "skipped: prior failure (--failfast)"})
			done++
			resMu.Unlock()
			fmt.Fprintf(r.Out, "[%d/%d] %-50s skip (failfast)\n", done, total, t.Name)
			continue
		default:
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(t Test) {
			defer wg.Done()
			defer func() { <-sem }()

			res := r.runOne(t)

			resMu.Lock()
			results = append(results, res)
			done++
			n := done
			resMu.Unlock()

			r.printLine(n, total, res)
			if res.Outcome == Failed || res.Outcome == Panicked {
				resMu.Lock()
				anyFailed = true
				resMu.Unlock()
				if r.FailFast {
					closeStop()
				}
				if res.Outcome == Panicked && r.StopOnPanic {
					closeStop()
				}
			}
		}(t)
	}

	wg.Wait()

	if r.Flaky > 0 {
		r.retryFlaky(results)
	}

	r.printSummary(results, r.NowFunc().Sub(startAll))

	_ = anyFailed
	return results
}

// retryFlaky reruns each Failed/Panicked test up to r.Flaky additional times
// (sequentially, to keep output legible). Each retry bumps the framework
// DefaultSeed by +1 so the mutator explores a different input sequence —
// otherwise a retry with the same seed would be byte-for-byte identical and
// reveal nothing new. A test that passes any retry is rewritten in-place to
// Outcome=Flaky with a reason explaining what happened. Tests that fail every
// retry stay Failed/Panicked.
func (r *Runner) retryFlaky(results []Result) {
	baseSeed := DefaultSeed
	defer func() { DefaultSeed = baseSeed }()

	for i := range results {
		o := results[i].Outcome
		if o != Failed && o != Panicked {
			continue
		}
		origReason := results[i].Reason
		origOutcome := o
		for attempt := 1; attempt <= r.Flaky; attempt++ {
			DefaultSeed = baseSeed + int64(attempt)
			retry := r.runOne(results[i].Test)
			fmt.Fprintf(r.Out, "[retry %d/%d seed=%d] %-40s %s (%s)\n", attempt, r.Flaky, DefaultSeed, retry.Test.Name, retry.Outcome, retry.Duration.Truncate(10*time.Millisecond))
			if retry.Outcome == Passed {
				results[i].Outcome = Flaky
				results[i].Reason = fmt.Sprintf("initially %s on attempt 1 (seed=%d), passed on attempt %d/%d (seed=%d). Initial failure:\n%s", origOutcome, baseSeed, attempt+1, r.Flaky+1, DefaultSeed, origReason)
				results[i].Duration += retry.Duration
				break
			}
			results[i].Duration += retry.Duration
		}
	}
}

func (r *Runner) runOne(t Test) Result {
	ctx := newT(t.Name)
	res := Result{Test: t}
	start := r.NowFunc()

	func() {
		defer func() {
			ctx.runCleanups()
			if rec := recover(); rec != nil {
				switch rec.(type) {
				case fatalSignal:
					// Errorf already populated failMsg; nothing more to do.
				case skipSignal:
					// skipped state already set on ctx.
				default:
					ctx.mu.Lock()
					ctx.failed = true
					ctx.failMsg += fmt.Sprintf("panic: %v\n%s", rec, debug.Stack())
					ctx.mu.Unlock()
					res.Outcome = Panicked
				}
			}
		}()
		t.Func(ctx)
	}()
	res.Duration = r.NowFunc().Sub(start)

	failed, failMsg, skipped, logs := ctx.snapshot()
	res.Logs = logs
	switch {
	case skipped != "":
		res.Outcome = Skipped
		res.Reason = skipped
	case res.Outcome == Panicked:
		res.Reason = strings.TrimRight(failMsg, "\n")
	case failed:
		res.Outcome = Failed
		res.Reason = strings.TrimRight(failMsg, "\n")
	default:
		res.Outcome = Passed
	}
	return res
}

func (r *Runner) printLine(n, total int, res Result) {
	dur := res.Duration.Truncate(10 * time.Millisecond)
	status := res.Outcome.String()
	suffix := ""
	if res.Outcome == Skipped && res.Reason != "" {
		suffix = "  (" + firstLine(res.Reason) + ")"
	}
	fmt.Fprintf(r.Out, "[%d/%d] %-50s %s (%s)%s\n", n, total, res.Test.Name, status, dur, suffix)
	if r.Verbose && len(res.Logs) > 0 {
		for _, l := range res.Logs {
			fmt.Fprintf(r.Out, "    %s\n", l)
		}
	}
}

func (r *Runner) printSummary(results []Result, total time.Duration) {
	sort.SliceStable(results, func(i, j int) bool { return results[i].Test.Name < results[j].Test.Name })
	var pass, fail, skip, panicked, flaky int
	var failed []Result
	var skipped []Result
	var flakyList []Result
	for _, r := range results {
		switch r.Outcome {
		case Passed:
			pass++
		case Failed:
			fail++
			failed = append(failed, r)
		case Skipped:
			skip++
			skipped = append(skipped, r)
		case Panicked:
			panicked++
			failed = append(failed, r)
		case Flaky:
			flaky++
			flakyList = append(flakyList, r)
		}
	}

	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, "================================ summary ================================")
	fmt.Fprintf(r.Out, "  total:     %d\n", len(results))
	fmt.Fprintf(r.Out, "  passed:    %d\n", pass)
	fmt.Fprintf(r.Out, "  failed:    %d\n", fail)
	if panicked > 0 {
		fmt.Fprintf(r.Out, "  panicked:  %d\n", panicked)
	}
	if flaky > 0 {
		fmt.Fprintf(r.Out, "  flaky:     %d\n", flaky)
	}
	fmt.Fprintf(r.Out, "  skipped:   %d\n", skip)
	fmt.Fprintf(r.Out, "  duration:  %s\n", total.Truncate(10*time.Millisecond))

	if len(failed) > 0 {
		fmt.Fprintln(r.Out)
		fmt.Fprintln(r.Out, "failures:")
		for _, f := range failed {
			fmt.Fprintf(r.Out, "  %s (%s):\n", f.Test.Name, f.Outcome)
			for _, line := range strings.Split(f.Reason, "\n") {
				fmt.Fprintf(r.Out, "    %s\n", line)
			}
			if len(f.Logs) > 0 {
				fmt.Fprintln(r.Out, "    logs:")
				for _, l := range f.Logs {
					fmt.Fprintf(r.Out, "      %s\n", l)
				}
			}
		}
	}

	if len(flakyList) > 0 {
		fmt.Fprintln(r.Out)
		fmt.Fprintln(r.Out, "flaky (failed initially but passed on retry — please investigate):")
		for _, f := range flakyList {
			fmt.Fprintf(r.Out, "  %s: %s\n", f.Test.Name, firstLine(f.Reason))
		}
	}

	if len(skipped) > 0 {
		fmt.Fprintln(r.Out)
		fmt.Fprintln(r.Out, "skipped:")
		for _, s := range skipped {
			fmt.Fprintf(r.Out, "  %s: %s\n", s.Test.Name, firstLine(s.Reason))
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// AnyFailed reports whether any result is Failed or Panicked.
func AnyFailed(rs []Result) bool {
	for _, r := range rs {
		if r.Outcome == Failed || r.Outcome == Panicked {
			return true
		}
	}
	return false
}
