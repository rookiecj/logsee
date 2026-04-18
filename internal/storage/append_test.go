package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLineAppender_rotateBeforeNextLineExceedsMax(t *testing.T) {
	// Given: small max size and lines that would push the file over the limit
	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")
	a, err := newLineAppender(path, 0, 256, 3)
	if err != nil {
		t.Fatalf("newLineAppender: %v", err)
	}
	line := strings.Repeat("x", 200)
	// When: two lines (each ~201 bytes on disk); second would exceed 256
	if err := a.WriteLine(line); err != nil {
		t.Fatalf("WriteLine 1: %v", err)
	}
	if err := a.Flush(); err != nil {
		t.Fatalf("Flush 1: %v", err)
	}
	if err := a.WriteLine(line); err != nil {
		t.Fatalf("WriteLine 2: %v", err)
	}
	if err := a.Flush(); err != nil {
		t.Fatalf("Flush 2: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Then: previous segment is path.1; active file holds only the second line
	b1, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read .1: %v", err)
	}
	if got, want := strings.TrimSuffix(string(b1), "\n"), line; got != want {
		t.Fatalf(".1 content: got len %d want len %d", len(got), len(want))
	}
	b0, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if got, want := strings.TrimSuffix(string(b0), "\n"), line; got != want {
		t.Fatalf("active file: got %q want %q", got, want)
	}
}

func TestLineAppender_maxBytesZeroNoRotation(t *testing.T) {
	// Given: rotation disabled
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	a, err := newLineAppender(path, 0, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("y", 500)
	// When
	for i := 0; i < 5; i++ {
		if err := a.WriteLine(line); err != nil {
			t.Fatal(err)
		}
		if err := a.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	// Then: no .1 file; single file grows past small hypothetical limit
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected no %q", path+".1")
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() < 2500 {
		t.Fatalf("expected large file, size %d", st.Size())
	}
}
