package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaderSource_EmitsLinesWithMonotonicSeq(t *testing.T) {
	rs := NewReader(strings.NewReader("a\nb\nc\n")).WithChannelCapacity(2)
	ch, err := rs.Lines(context.Background())
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	defer rs.Close()

	var got []string
	var lastSeq int64
	for line := range ch {
		if line.Seq <= lastSeq {
			t.Errorf("Seq not monotonic: %d after %d", line.Seq, lastSeq)
		}
		lastSeq = line.Seq
		got = append(got, line.Text)
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReaderSource_HonorsContextCancellation(t *testing.T) {
	// Build input large enough that not everything fits the channel
	// buffer before we cancel.
	rs := NewReader(strings.NewReader(strings.Repeat("x\n", 10_000))).WithChannelCapacity(1)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := rs.Lines(ctx)
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	// Read a couple of lines then cancel.
	<-ch
	<-ch
	cancel()
	// Drain — producer should close the channel once it notices ctx.Done.
	for range ch {
	}
	if err := rs.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestFileSource_ReadsFileByLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.log")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs := NewFile(path).WithChannelCapacity(2)
	ch, err := fs.Lines(context.Background())
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	defer fs.Close()

	var got []string
	for line := range ch {
		got = append(got, line.Text)
	}
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestFileSource_MissingFileReturnsError(t *testing.T) {
	fs := NewFile(filepath.Join(t.TempDir(), "nope.log"))
	_, err := fs.Lines(context.Background())
	if err == nil {
		t.Fatal("expected open error for missing file")
	}
}

func TestFileSource_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	_ = os.WriteFile(path, []byte("a\n"), 0o644)
	fs := NewFile(path)
	if _, err := fs.Lines(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := fs.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Errorf("second Close should be no-op: %v", err)
	}
}
