package fileindex

import (
	"os"
	"path/filepath"
	"testing"

	"git.inpt.fr/42dottools/log/internal/loginput"
)

func TestReadWindowRecords_roundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	content := "a\nb\nc\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	off, err := loginput.LineStartOffsets(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(off) != 3 {
		t.Fatalf("offsets: want 3 got %d", len(off))
	}
	recs, err := ReadWindowRecords(path, off, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Seq != 2 || recs[0].Text != "b" || recs[1].Seq != 3 || recs[1].Text != "c" {
		t.Fatalf("records: %#v", recs)
	}
}

func TestReadFirstNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	if err := os.WriteFile(path, []byte("x\ny\nz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadFirstNLines(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "x" || lines[1] != "y" {
		t.Fatalf("got %#v", lines)
	}
}
