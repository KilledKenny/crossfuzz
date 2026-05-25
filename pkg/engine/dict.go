package engine

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
)

// Dict is a collection of byte tokens used by the dictOverwrite/dictInsert
// mutation strategies. Entries shorter than 1 byte or longer than 64 bytes
// are dropped on load to keep mutations bounded.
type Dict struct {
	entries [][]byte
}

// NewDict creates an empty dictionary.
func NewDict() *Dict { return &Dict{} }

// Len returns the number of entries.
func (d *Dict) Len() int {
	if d == nil {
		return 0
	}
	return len(d.entries)
}

// Add appends a token. Tokens of length 0 or >64 are ignored.
func (d *Dict) Add(token []byte) {
	if len(token) == 0 || len(token) > 64 {
		return
	}
	cp := make([]byte, len(token))
	copy(cp, token)
	d.entries = append(d.entries, cp)
}

// Pick returns a random token. Caller must check Len() > 0.
func (d *Dict) Pick(r interface{ Intn(int) int }) []byte {
	return d.entries[r.Intn(len(d.entries))]
}

// LoadFile reads tokens in AFL-compatible dictionary syntax.
// Each non-comment, non-blank line is either:
//
//	"\xNNfoo\x00"
//	name="\xNNfoo\x00"
//	name@level="..."
//
// Levels are ignored. # starts a comment. Escapes: \\ \" \xNN.
func (d *Dict) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read dict %s: %w", path, err)
	}
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tok, err := parseDictLine(line)
		if err != nil {
			return fmt.Errorf("dict %s:%d: %w", path, i+1, err)
		}
		d.Add(tok)
	}
	return nil
}

// parseDictLine extracts the quoted token from one AFL-style dictionary entry.
func parseDictLine(line string) ([]byte, error) {
	// Strip optional `name=` or `name@level=` prefix.
	if eq := strings.Index(line, "="); eq != -1 && !strings.HasPrefix(line, "\"") {
		line = strings.TrimSpace(line[eq+1:])
	}
	if len(line) < 2 || line[0] != '"' || line[len(line)-1] != '"' {
		return nil, fmt.Errorf("expected quoted token, got %q", line)
	}
	body := line[1 : len(line)-1]
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		if i+1 >= len(body) {
			return nil, fmt.Errorf("dangling backslash")
		}
		i++
		switch body[i] {
		case '\\':
			out = append(out, '\\')
		case '"':
			out = append(out, '"')
		case 'x':
			if i+2 >= len(body) {
				return nil, fmt.Errorf("bad \\x escape")
			}
			var v byte
			for j := 0; j < 2; j++ {
				v <<= 4
				h := body[i+1+j]
				switch {
				case h >= '0' && h <= '9':
					v |= h - '0'
				case h >= 'a' && h <= 'f':
					v |= h - 'a' + 10
				case h >= 'A' && h <= 'F':
					v |= h - 'A' + 10
				default:
					return nil, fmt.Errorf("bad hex digit %q", h)
				}
			}
			out = append(out, v)
			i += 2
		default:
			return nil, fmt.Errorf("unknown escape \\%c", body[i])
		}
	}
	return out, nil
}

// DefaultDictForComparator returns a built-in set of tokens useful for the
// given comparator type. Returns an empty dict for comparators with no useful
// defaults (byte_equal, none, custom, harness).
func DefaultDictForComparator(name string) *Dict {
	d := NewDict()
	switch name {
	case "json_structural":
		for _, s := range []string{
			"{", "}", "[", "]", ",", ":", " ", "\t", "\n",
			"null", "true", "false",
			"\"\"", "\"a\"", "\" \"", "\"\\u0000\"", "\"\\\\\"", "\"\\\"\"",
			"0", "-0", "1", "-1", "0.0", "1e0", "1e308", "-1e308",
			"1e-308", "0.1", "-9223372036854775808", "9223372036854775807",
		} {
			d.Add([]byte(s))
		}
	case "numeric":
		add64 := func(u uint64) {
			le := make([]byte, 8)
			be := make([]byte, 8)
			binary.LittleEndian.PutUint64(le, u)
			binary.BigEndian.PutUint64(be, u)
			d.Add(le)
			d.Add(be)
		}
		add64(0)
		add64(1)
		add64(^uint64(0))
		add64(uint64(math.MaxInt64))
		add64(uint64(1) << 63) // bit pattern of int64 MinInt64
		add64(math.Float64bits(math.NaN()))
		add64(math.Float64bits(math.Inf(1)))
		add64(math.Float64bits(math.Inf(-1)))
		for _, s := range []string{"0", "-1", "1", "NaN", "Infinity", "-Infinity", "1e308", "1e-308"} {
			d.Add([]byte(s))
		}
	}
	return d
}
