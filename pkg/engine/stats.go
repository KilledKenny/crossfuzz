package engine

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Stats tracks live fuzzing statistics.
type Stats struct {
	mu          sync.Mutex
	startTime   time.Time
	totalExecs  uint64
	corpusSize  int
	coverBits   int
	findings    int
	lastPrint   time.Time
	debugEdge   bool
	targetEdges map[string]int
}

// NewStats creates a stats tracker.
func NewStats() *Stats {
	now := time.Now().Add(-time.Duration(time.Second * 5))
	return &Stats{startTime: now, lastPrint: now}
}

// SetDebugEdge enables per-target edge counts in the ticker output.
func (s *Stats) SetDebugEdge(enabled bool) {
	s.mu.Lock()
	s.debugEdge = enabled
	s.mu.Unlock()
}

func (s *Stats) RecordExec() {
	s.mu.Lock()
	s.totalExecs++
	s.mu.Unlock()
}

func (s *Stats) Update(corpusSize, coverBits, findings int, targetEdges map[string]int) {
	s.mu.Lock()
	s.corpusSize = corpusSize
	s.coverBits = coverBits
	s.findings = findings
	s.targetEdges = targetEdges
	s.mu.Unlock()
}

// PrintIfDue prints stats to stderr at most every 3 seconds.
func (s *Stats) PrintIfDue() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Since(s.lastPrint) < 3*time.Second {
		return
	}

	elapsed := time.Since(s.startTime)
	execsPerSec := float64(s.totalExecs) / elapsed.Seconds()

	line := fmt.Sprintf("\r\033[2K[%s] execs: %d (%.0f/sec) | corpus: %d | coverage: %d edges | findings: %d",
		elapsed.Truncate(time.Second), s.totalExecs, execsPerSec,
		s.corpusSize, s.coverBits, s.findings)

	if s.debugEdge && len(s.targetEdges) > 0 {
		names := make([]string, 0, len(s.targetEdges))
		for name := range s.targetEdges {
			names = append(names, name)
		}
		sort.Strings(names)
		line += " |"
		for _, name := range names {
			line += fmt.Sprintf(" %s: %d", name, s.targetEdges[name])
		}
	}

	fmt.Println(line)

	s.lastPrint = time.Now()
}
