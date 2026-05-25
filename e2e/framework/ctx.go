package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// T is the per-test context handed to every test function. It mimics enough
// of *testing.T's surface (Errorf, Fatalf, Skipf, Logf, Cleanup, TempDir,
// Helper, Parallel) that the existing helpers ported from go test compile
// with minimal changes, but it is otherwise independent of the testing
// package — see orchestrator.go for the runner.
type T struct {
	name string

	mu       sync.Mutex
	failed   bool
	failMsg  string
	skipped  string
	logs     []string
	cleanups []func()
	tempIdx  uint32
	tempDir  string // lazily created per-test root
}

func newT(name string) *T { return &T{name: name} }

// Sentinel panic types thrown by Fatalf / Skipf to abort the running test
// cleanly. The orchestrator's deferred recover distinguishes them by type.
type fatalSignal struct{}
type skipSignal struct{}

// Name returns the test name (e.g. "cli.MaxFindings").
func (t *T) Name() string { return t.name }

// Helper is a no-op kept for API compatibility — we don't track caller depth.
func (t *T) Helper() {}

// Parallel is a no-op. The orchestrator manages parallelism; tests do not
// opt in individually.
func (t *T) Parallel() {}

// Errorf records a failure but lets the test continue.
func (t *T) Errorf(format string, args ...any) {
	t.mu.Lock()
	t.failed = true
	t.failMsg += fmt.Sprintf(format, args...) + "\n"
	t.mu.Unlock()
}

// Fatalf records a failure and aborts the current test via panic.
func (t *T) Fatalf(format string, args ...any) {
	t.Errorf(format, args...)
	panic(fatalSignal{})
}

// Errorln/Error are kept minimal — most callers use Errorf/Fatalf.
func (t *T) Error(args ...any) { t.Errorf("%s", fmt.Sprint(args...)) }
func (t *T) Fatal(args ...any) { t.Fatalf("%s", fmt.Sprint(args...)) }

// Skipf marks the test as skipped and aborts it.
func (t *T) Skipf(format string, args ...any) {
	t.mu.Lock()
	t.skipped = fmt.Sprintf(format, args...)
	t.mu.Unlock()
	panic(skipSignal{})
}

func (t *T) Skip(args ...any) { t.Skipf("%s", fmt.Sprint(args...)) }

// Logf records a log line; the orchestrator prints them in verbose mode
// and always after a failure.
func (t *T) Logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	t.mu.Lock()
	t.logs = append(t.logs, line)
	t.mu.Unlock()
}

func (t *T) Log(args ...any) { t.Logf("%s", fmt.Sprint(args...)) }

// Cleanup registers a function to run after the test, in LIFO order.
func (t *T) Cleanup(f func()) {
	t.mu.Lock()
	t.cleanups = append(t.cleanups, f)
	t.mu.Unlock()
}

// TempDir returns a fresh directory under a per-test root. The root is
// removed (along with every subdirectory created via TempDir) when the
// test finishes.
func (t *T) TempDir() string {
	t.mu.Lock()
	if t.tempDir == "" {
		root, err := os.MkdirTemp("", "e2e-"+sanitize(t.name)+"-")
		if err != nil {
			t.mu.Unlock()
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.tempDir = root
		t.cleanups = append(t.cleanups, func() { _ = os.RemoveAll(root) })
	}
	idx := atomic.AddUint32(&t.tempIdx, 1)
	t.mu.Unlock()
	sub := filepath.Join(t.tempDir, fmt.Sprintf("%03d", idx))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir tempdir: %v", err)
	}
	return sub
}

// runCleanups runs every registered Cleanup in LIFO order.
func (t *T) runCleanups() {
	t.mu.Lock()
	fs := t.cleanups
	t.cleanups = nil
	t.mu.Unlock()
	for i := len(fs) - 1; i >= 0; i-- {
		func(f func()) {
			defer func() {
				_ = recover() // best effort; cleanup must not abort other cleanups
			}()
			f()
		}(fs[i])
	}
}

// snapshot returns a stable view of the current state for the orchestrator.
func (t *T) snapshot() (failed bool, failMsg, skipped string, logs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cpy := make([]string, len(t.logs))
	copy(cpy, t.logs)
	return t.failed, t.failMsg, t.skipped, cpy
}

// Duration is set by the orchestrator after the test runs; tests don't use it.
var _ = time.Duration(0)

// sanitize makes a test name safe for a filesystem path component.
func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return string(out)
}

// strconv used at compile-time check only.
var _ = strconv.Itoa
