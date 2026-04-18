package loginput

import (
	"io"
	"strings"
	"testing"
)

func TestScanLines_CRLF(t *testing.T) {
	// Given: CRLF terminated line
	// When: ScanLines
	// Then: one line without CR or LF
	var got []string
	err := ScanLines(strings.NewReader("hello\r\n"), func(s string) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("got %#v", got)
	}
}

func TestScanLines_CRResetsUntilLF(t *testing.T) {
	// Given: CR overwrites prefix before LF
	// When: ScanLines
	// Then: only text after last CR before LF
	var got []string
	err := ScanLines(strings.NewReader("foo\rbar\n"), func(s string) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "bar" {
		t.Fatalf("got %#v", got)
	}
}

func TestScanLines_EmptyLines(t *testing.T) {
	var got []string
	err := ScanLines(strings.NewReader("\n\na\n"), func(s string) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "" || got[1] != "" || got[2] != "a" {
		t.Fatalf("got %#v", got)
	}
}

func TestScanLines_MacCREnd(t *testing.T) {
	// Given: lone CR at end of file (Mac line end)
	// When: ScanLines
	// Then: one line emitted
	var got []string
	err := ScanLines(strings.NewReader("only\r"), func(s string) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "only" {
		t.Fatalf("got %#v", got)
	}
}

func TestLineStartOffsets_matchesScanLineCount(t *testing.T) {
	inputs := []string{
		"",
		"a",
		"a\n",
		"\n",
		"a\nb",
		"foo\rbar\n",
		"only\r",
		"\n\na\n",
	}
	for _, s := range inputs {
		var lines []string
		err := ScanLines(strings.NewReader(s), func(x string) error {
			lines = append(lines, x)
			return nil
		})
		if err != nil {
			t.Fatalf("input %q ScanLines: %v", s, err)
		}
		off, err := LineStartOffsets(strings.NewReader(s))
		if err != nil {
			t.Fatalf("input %q LineStartOffsets: %v", s, err)
		}
		if len(off) != len(lines) {
			t.Fatalf("input %q: want %d offsets, got %d (lines %#v)", s, len(lines), len(off), lines)
		}
	}
}

func TestScanLines_SplitCRLFAcrossRead(t *testing.T) {
	// Given: \r and \n split across small reads
	// When: ScanLines
	// Then: single logical line
	r := &chunkReader{chunks: [][]byte{[]byte("ab\r"), []byte("\ncd\n")}}
	var got []string
	err := ScanLines(r, func(s string) error {
		got = append(got, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "ab" || got[1] != "cd" {
		t.Fatalf("got %#v", got)
	}
}

type chunkReader struct {
	chunks [][]byte
	i      int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.i >= len(c.chunks) {
		return 0, io.EOF
	}
	b := c.chunks[c.i]
	c.i++
	n := copy(p, b)
	if n < len(b) {
		// simplify test: chunks fit in p
		panic("chunk too large for buffer")
	}
	return n, nil
}
