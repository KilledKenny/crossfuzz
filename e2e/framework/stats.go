package framework

import (
	"regexp"
	"strconv"
	"strings"
)

// TickStats is one line of the live stats ticker (printed every ~3s).
type TickStats struct {
	Execs       int
	ExecsPerSec float64
	Rejected    int
	Duplicates  int
	Corpus      int
	Coverage    int // total coverage edges across all targets
	Findings    int
	Crashes     int
	Timeouts    int
	TargetEdges map[string]int // populated only when --debug-edge is set
}

// FinalStats is parsed from the "Campaign finished. ..." line printed at
// shutdown. It is the authoritative end-of-run summary.
type FinalStats struct {
	Found    bool // true if the "Campaign finished" line was present
	Execs    int
	Rejected int
	Corpus   int
	Findings int
	Crashes  int
	Timeouts int
}

var (
	tickRE = regexp.MustCompile(
		`execs:\s+(\d+)\s+\(([0-9.]+)/sec\)\s+\|\s+rejected:\s+(\d+)\s+\|\s+dup:\s+(\d+)\s+\|\s+corpus:\s+(\d+)\s+\|\s+coverage:\s+(\d+)\s+edges\s+\|\s+findings:\s+(\d+)\s+\|\s+crashes:\s+(\d+)\s+\|\s+timeouts:\s+(\d+)`,
	)
	finalRE = regexp.MustCompile(
		`Campaign finished\. Total execs:\s+(\d+),\s+Rejected:\s+(\d+),\s+Corpus:\s+(\d+),\s+Findings:\s+(\d+),\s+Crashes:\s+(\d+),\s+Timeouts:\s+(\d+)`,
	)
	debugEdgeRE = regexp.MustCompile(`\s+([A-Za-z0-9_]+):\s+(\d+)`)
)

// ParseOutput extracts every stats tick and the final summary line from
// captured stdout. Both may be absent (returns zero values).
func ParseOutput(stdout string) (FinalStats, []TickStats) {
	var ticks []TickStats
	for _, line := range strings.Split(stdout, "\n") {
		m := tickRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		t := TickStats{
			Execs:       atoi(m[1]),
			ExecsPerSec: atof(m[2]),
			Rejected:    atoi(m[3]),
			Duplicates:  atoi(m[4]),
			Corpus:      atoi(m[5]),
			Coverage:    atoi(m[6]),
			Findings:    atoi(m[7]),
			Crashes:     atoi(m[8]),
			Timeouts:    atoi(m[9]),
		}
		tail := line[strings.Index(line, "timeouts:"):]
		if pipe := strings.Index(tail, " | "); pipe >= 0 {
			t.TargetEdges = map[string]int{}
			for _, em := range debugEdgeRE.FindAllStringSubmatch(tail[pipe:], -1) {
				t.TargetEdges[em[1]] = atoi(em[2])
			}
		}
		ticks = append(ticks, t)
	}

	var final FinalStats
	if m := finalRE.FindStringSubmatch(stdout); m != nil {
		final.Found = true
		final.Execs = atoi(m[1])
		final.Rejected = atoi(m[2])
		final.Corpus = atoi(m[3])
		final.Findings = atoi(m[4])
		final.Crashes = atoi(m[5])
		final.Timeouts = atoi(m[6])
	}
	return final, ticks
}

func atoi(s string) int     { n, _ := strconv.Atoi(s); return n }
func atof(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }
