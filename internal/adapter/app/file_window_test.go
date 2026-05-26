package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtendFileLineIndexAppendsOnlyNewBytes(t *testing.T) {
	// given
	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")
	if err := os.WriteFile(path, []byte("line-1\nline-2\n"), 0o644); err != nil {
		t.Fatalf("write initial SOT: %v", err)
	}
	initial, err := buildFileLineIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("build initial index: %v", err)
	}
	if err := os.WriteFile(path, []byte("line-1\nline-2\nline-3\nline-4\n"), 0o644); err != nil {
		t.Fatalf("append to SOT: %v", err)
	}

	// when
	extended, err := extendFileLineIndex(context.Background(), path, initial)

	// then
	if err != nil {
		t.Fatalf("extend file index: %v", err)
	}
	if got, want := extended.totalLines(), 4; got != want {
		t.Fatalf("total lines = %d, want %d", got, want)
	}
	if got, want := len(extended.offsets), len(initial.offsets)+2; got != want {
		t.Fatalf("offset count = %d, want %d", got, want)
	}
}
