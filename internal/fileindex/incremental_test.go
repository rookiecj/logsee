package fileindex

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAppend(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestIncrementalOffsetIndex_GrowsOneByteAtATime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}
	idx := NewIncrementalOffsetIndex(path, 0)

	// Empty refresh is a no-op.
	if err := idx.RefreshTo(0); err != nil {
		t.Fatalf("refresh empty: %v", err)
	}
	if got := idx.Len(); got != 0 {
		t.Fatalf("empty: Len=%d, want 0", got)
	}

	// "a\n" → one line at offset 0.
	writeAppend(t, path, "a\n")
	if err := idx.RefreshTo(2); err != nil {
		t.Fatalf("refresh 2: %v", err)
	}
	if got := idx.Snapshot(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("after a\\n: offsets=%v, want [0]", got)
	}

	// Append "b\n" → two lines.
	writeAppend(t, path, "b\n")
	if err := idx.RefreshTo(4); err != nil {
		t.Fatalf("refresh 4: %v", err)
	}
	if got := idx.Snapshot(); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("after b\\n: offsets=%v, want [0 2]", got)
	}

	// Partial line "cc" — no trailing newline — line 3 is started but open.
	writeAppend(t, path, "cc")
	if err := idx.RefreshTo(6); err != nil {
		t.Fatalf("refresh 6: %v", err)
	}
	if got := idx.Snapshot(); len(got) != 3 || got[2] != 4 {
		t.Fatalf("after partial cc: offsets=%v, want [0 2 4]", got)
	}

	// Complete line 3 with "\n" — offsets unchanged; pendingStart advances.
	writeAppend(t, path, "\n")
	if err := idx.RefreshTo(7); err != nil {
		t.Fatalf("refresh 7: %v", err)
	}
	if got := idx.Snapshot(); len(got) != 3 {
		t.Fatalf("after completing line 3: offsets=%v, want unchanged [0 2 4]", got)
	}

	// Line 4.
	writeAppend(t, path, "d\n")
	if err := idx.RefreshTo(9); err != nil {
		t.Fatalf("refresh 9: %v", err)
	}
	if got := idx.Snapshot(); len(got) != 4 || got[3] != 7 {
		t.Fatalf("after d\\n: offsets=%v, want [0 2 4 7]", got)
	}
}

func TestIncrementalOffsetIndex_SkipsStartBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	// Pre-existing content before the indexed window.
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}
	idx := NewIncrementalOffsetIndex(path, 2)

	writeAppend(t, path, "bb\ncc\n")
	if err := idx.RefreshTo(8); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	got := idx.Snapshot()
	if len(got) != 2 || got[0] != 2 || got[1] != 5 {
		t.Fatalf("offsets=%v, want [2 5]", got)
	}
}

func TestIncrementalOffsetIndex_RefreshIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}
	idx := NewIncrementalOffsetIndex(path, 0)
	for range 3 {
		if err := idx.RefreshTo(12); err != nil {
			t.Fatalf("refresh: %v", err)
		}
	}
	got := idx.Snapshot()
	if len(got) != 2 || got[0] != 0 || got[1] != 6 {
		t.Fatalf("offsets=%v, want [0 6]", got)
	}
}

func TestIncrementalOffsetIndex_ResetFollowsRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}
	idx := NewIncrementalOffsetIndex(path, 0)
	if err := idx.RefreshTo(4); err != nil {
		t.Fatalf("refresh pre: %v", err)
	}
	// Simulate rotation: truncate file and re-index from 0.
	if err := os.WriteFile(path, []byte("x\ny\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	idx.Reset(0)
	if err := idx.RefreshTo(4); err != nil {
		t.Fatalf("refresh post: %v", err)
	}
	got := idx.Snapshot()
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("offsets=%v, want [0 2]", got)
	}
}
