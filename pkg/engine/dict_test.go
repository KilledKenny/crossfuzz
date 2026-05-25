package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDictLine(t *testing.T) {
	cases := []struct {
		in   string
		want []byte
	}{
		{`"abc"`, []byte("abc")},
		{`name="abc"`, []byte("abc")},
		{`name@1="abc"`, []byte("abc")},
		{`"\x00\x01\xff"`, []byte{0x00, 0x01, 0xff}},
		{`"a\\b\"c"`, []byte(`a\b"c`)},
	}
	for _, c := range cases {
		got, err := parseDictLine(c.in)
		if err != nil {
			t.Fatalf("parseDictLine(%q): %v", c.in, err)
		}
		if !bytes.Equal(got, c.want) {
			t.Errorf("parseDictLine(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDictLineErrors(t *testing.T) {
	bad := []string{
		`abc`,           // no quotes
		`"abc`,          // unterminated
		`"\xZZ"`,        // bad hex
		`"\q"`,          // unknown escape
		`"\`,            // dangling
	}
	for _, b := range bad {
		if _, err := parseDictLine(b); err == nil {
			t.Errorf("parseDictLine(%q): expected error", b)
		}
	}
}

func TestDictLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.txt")
	body := "# comment\n" +
		"\n" +
		"\"{\"\n" +
		"name=\"foo\"\n" +
		"a@2=\"\\x00\\xff\"\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	d := NewDict()
	if err := d.LoadFile(path); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if d.Len() != 3 {
		t.Fatalf("Len = %d, want 3", d.Len())
	}
}

func TestDictDefaultForComparator(t *testing.T) {
	if DefaultDictForComparator("byte_equal").Len() != 0 {
		t.Error("byte_equal should have no defaults")
	}
	if DefaultDictForComparator("json_structural").Len() < 10 {
		t.Error("json_structural should have many defaults")
	}
	if DefaultDictForComparator("numeric").Len() < 10 {
		t.Error("numeric should have many defaults")
	}
}
