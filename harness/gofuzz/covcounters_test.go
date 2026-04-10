package gofuzz

import (
	"bytes"
	"runtime/coverage"
	"strings"
	"testing"
)

// exerciseCoverage calls into enough instrumented code that a snapshot
// taken afterwards will contain non-zero counter values.
func exerciseCoverage() int {
	s := strings.Repeat("ab", 32)
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 'a' {
			n++
		} else {
			n--
		}
	}
	return n
}

func TestCovReaderParsesSelfSnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := coverage.WriteCounters(&buf); err != nil {
		t.Skipf("runtime/coverage unavailable (need -cover -covermode=atomic): %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("WriteCounters produced zero bytes")
	}

	// Make sure we have some counter activity to look at.
	_ = exerciseCoverage()

	// Take a second snapshot after exercising code.
	buf.Reset()
	if err := coverage.WriteCounters(&buf); err != nil {
		t.Fatalf("second WriteCounters: %v", err)
	}

	var r covReader
	funcs, err := r.parse(buf.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(funcs) == 0 {
		t.Fatal("parser returned zero function entries")
	}

	var total, nonZero int
	for _, f := range funcs {
		for _, c := range f.counters {
			total++
			if c > 0 {
				nonZero++
			}
		}
	}
	if total == 0 {
		t.Fatal("no counter values across any function")
	}
	if nonZero == 0 {
		t.Fatal("every counter was zero; parser or exerciseCoverage is broken")
	}
	t.Logf("parsed %d functions, %d/%d counters non-zero",
		len(funcs), nonZero, total)
}

func TestCovReaderRejectsBadMagic(t *testing.T) {
	var r covReader
	bad := make([]byte, covHeaderSize)
	bad[0] = 0xFF
	if _, err := r.parse(bad); err == nil {
		t.Fatal("expected error on bad magic, got nil")
	}
}

func TestCovReaderRejectsShortInput(t *testing.T) {
	var r covReader
	if _, err := r.parse([]byte{0x00, 0x63}); err == nil {
		t.Fatal("expected error on short input, got nil")
	}
}

func TestCovReaderRejectsUnknownFlavor(t *testing.T) {
	var r covReader
	buf := make([]byte, covHeaderSize)
	copy(buf[:4], covMagic[:])
	// version = 1
	buf[4] = 1
	// flavor at offset 24 set to an invalid value
	buf[24] = 99
	if _, err := r.parse(buf); err == nil {
		t.Fatal("expected error on unknown flavor, got nil")
	}
}

// realSample is a 155-byte covcounters stream captured from a real
// `go build -cover -covermode=atomic` helper program that ran a tight
// loop incrementing/decrementing counters. Flavor is CtrULeb128. There
// are two function entries with known counter values.
var realSample = []byte{
	0x00, 0x63, 0x77, 0x6d, 0x01, 0x00, 0x00, 0x00, 0xec, 0xbe, 0x71, 0x39, 0x7a, 0xfa, 0xdd, 0x16,
	0xd3, 0xb6, 0x06, 0xa2, 0xd1, 0x5d, 0xe7, 0xa6, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x41, 0x00, 0x00, 0x00, 0x0b, 0x00, 0x00, 0x00,
	0x09, 0x00, 0x05, 0x61, 0x72, 0x67, 0x76, 0x30, 0x19, 0x2f, 0x74, 0x6d, 0x70, 0x2f, 0x63, 0x6c,
	0x61, 0x75, 0x64, 0x65, 0x2f, 0x63, 0x6f, 0x76, 0x67, 0x65, 0x6e, 0x2f, 0x63, 0x6f, 0x76, 0x67,
	0x65, 0x6e, 0x04, 0x47, 0x4f, 0x4f, 0x53, 0x05, 0x6c, 0x69, 0x6e, 0x75, 0x78, 0x06, 0x47, 0x4f,
	0x41, 0x52, 0x43, 0x48, 0x05, 0x61, 0x6d, 0x64, 0x36, 0x34, 0x04, 0x61, 0x72, 0x67, 0x63, 0x01,
	0x31, 0x04, 0x05, 0x06, 0x03, 0x04, 0x07, 0x08, 0x01, 0x02, 0x00, 0x00, 0x04, 0x00, 0x01, 0x01,
	0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x01, 0x01, 0x64, 0x32, 0x32, 0x00, 0x63, 0x77, 0x6d, 0x00,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

func TestCovReaderParsesRealSample(t *testing.T) {
	var r covReader
	funcs, err := r.parse(realSample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}

	// Function 0: NumCtrs=4, PkgID=0, FuncID=1, counters=[1,0,0,0]
	// (this was the helper's work() function — only one basic block hit
	// since work() was called once).
	got0 := funcs[0]
	if got0.pkgID != 0 || got0.funcID != 1 {
		t.Errorf("func 0 ids: got pkg=%d func=%d, want pkg=0 func=1",
			got0.pkgID, got0.funcID)
	}
	want0 := []uint32{1, 0, 0, 0}
	if !counterSliceEqual(got0.counters, want0) {
		t.Errorf("func 0 counters: got %v, want %v", got0.counters, want0)
	}

	// Function 1: NumCtrs=5, PkgID=0, FuncID=0, counters=[1,1,100,50,50]
	// (main + the 100-iteration loop → 100 total, 50 even branches,
	// 50 odd branches).
	got1 := funcs[1]
	if got1.pkgID != 0 || got1.funcID != 0 {
		t.Errorf("func 1 ids: got pkg=%d func=%d, want pkg=0 func=0",
			got1.pkgID, got1.funcID)
	}
	want1 := []uint32{1, 1, 100, 50, 50}
	if !counterSliceEqual(got1.counters, want1) {
		t.Errorf("func 1 counters: got %v, want %v", got1.counters, want1)
	}
}

func counterSliceEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCovReaderParsesDiffSnapshot(t *testing.T) {
	var buf1 bytes.Buffer
	if err := coverage.WriteCounters(&buf1); err != nil {
		t.Skipf("runtime/coverage unavailable (need -cover -covermode=atomic): %v", err)
	}
	if buf1.Len() == 0 {
		t.Fatal("WriteCounters produced zero bytes")
	}

	// Make sure we have some counter activity to look at.
	_ = exerciseCoverage()

	// Take a second snapshot after exercising code.
	var buf2 bytes.Buffer
	if err := coverage.WriteCounters(&buf2); err != nil {
		t.Fatalf("second WriteCounters: %v", err)
	}
	if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatalf("Buff 1 and 2 match")
	}

	var r covReader
	funcs, err := r.parse(buf2.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(funcs) == 0 {
		t.Fatal("parser returned zero function entries")
	}

	var total, nonZero int
	for _, f := range funcs {
		for _, c := range f.counters {
			total++
			if c > 0 {
				nonZero++
			}
		}
	}
	if total == 0 {
		t.Fatal("no counter values across any function")
	}
	if nonZero == 0 {
		t.Fatal("every counter was zero; parser or exerciseCoverage is broken")
	}
	t.Logf("parsed %d functions, %d/%d counters non-zero",
		len(funcs), nonZero, total)
}
