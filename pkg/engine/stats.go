package engine

import (
	"fmt"
	"sync"
	"time"
)

// Stats tracks live fuzzing statistics.
type Stats struct {
	mu         sync.Mutex
	startTime  time.Time
	totalExecs uint64
	corpusSize int
	coverBits  int
	findings   int
	lastPrint  time.Time
}

// NewStats creates a stats tracker.
func NewStats() *Stats {
	now := time.Now().Add(-time.Duration(time.Second * 5))
	return &Stats{startTime: now, lastPrint: now}
}

func (s *Stats) RecordExec() {
	s.mu.Lock()
	s.totalExecs++
	s.mu.Unlock()
}

func (s *Stats) Update(corpusSize, coverBits, findings int) {
	s.mu.Lock()
	s.corpusSize = corpusSize
	s.coverBits = coverBits
	s.findings = findings
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

	fmt.Printf("\r\033[2K[%s] execs: %d (%.0f/sec) | corpus: %d | coverage: %d edges | findings: %d\n",
		elapsed.Truncate(time.Second), s.totalExecs, execsPerSec,
		s.corpusSize, s.coverBits, s.findings)

	s.lastPrint = time.Now()
}
