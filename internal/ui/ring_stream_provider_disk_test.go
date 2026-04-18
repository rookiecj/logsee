package ui

import (
	"os"
	"path/filepath"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/fileindex"
)

// setupDiskProvider wires a RingStreamProvider with a fresh --out file and an
// incremental index. The caller receives a "writer" closure that appends a
// line, flushes it to disk, pushes into the ring, and refreshes the index —
// mirroring the production applyIncomingLines flow.
func setupDiskProvider(t *testing.T, ringMax int) (*RingStreamProvider, func(string)) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create out: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	idx := fileindex.NewIncrementalOffsetIndex(path, 0)
	p := NewRingStreamProvider(buffer.NewRing(ringMax))
	p.SetDiskFallback(path, idx, 1)

	write := func(text string) {
		t.Helper()
		if _, err := f.WriteString(text + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := f.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		p.Push(text)
		st, err := f.Stat()
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if err := p.RefreshIndex(st.Size()); err != nil {
			t.Fatalf("refresh: %v", err)
		}
	}
	return p, write
}

func TestRingStreamProvider_DiskFallback_ServesEvictedSeqs(t *testing.T) {
	p, write := setupDiskProvider(t, 3) // tiny ring forces evictions
	for i := 1; i <= 10; i++ {
		write("line-" + string(rune('0'+i%10)))
	}

	// Seqs 1..7 have been evicted from the ring; the disk fallback must resolve them.
	recs, err := p.Fetch(1, 3)
	if err != nil {
		t.Fatalf("Fetch 1..3: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("len=%d, want 3", len(recs))
	}
	for i, rec := range recs {
		if rec.Seq != int64(i+1) {
			t.Errorf("recs[%d].Seq=%d, want %d", i, rec.Seq, i+1)
		}
	}

	// Live ring still serves recent seqs.
	recs, err = p.Fetch(8, 10)
	if err != nil {
		t.Fatalf("Fetch 8..10: %v", err)
	}
	if len(recs) != 3 || recs[0].Seq != 8 || recs[2].Seq != 10 {
		t.Fatalf("live fetch: %+v", recs)
	}
}

func TestRingStreamProvider_DiskFallback_StraddlesRingBoundary(t *testing.T) {
	p, write := setupDiskProvider(t, 3)
	for i := 1; i <= 10; i++ {
		write("x")
	}

	// Ring holds 8..10; request 6..10 should merge disk (6,7) + ring (8,9,10).
	recs, err := p.Fetch(6, 10)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(recs) != 5 {
		t.Fatalf("len=%d, want 5", len(recs))
	}
	for i, rec := range recs {
		if rec.Seq != int64(6+i) {
			t.Errorf("recs[%d].Seq=%d, want %d", i, rec.Seq, 6+i)
		}
	}
}

func TestRingStreamProvider_DiskFallback_RespectsHorizonAfterRotation(t *testing.T) {
	p, write := setupDiskProvider(t, 3)
	for i := 1; i <= 5; i++ {
		write("pre")
	}
	// Simulate rotation: lines 1..5 lost to a pre-rotation file. New file starts at seq 6.
	p.NoteRotation(6)
	// Fresh-file-backed index was reset; write post-rotation into a fresh file path.
	// For the test, reuse the same path: truncate and re-seed via index reset done in NoteRotation.
	// The test framework setup's write() appends to the original fd, so we mimic that by
	// truncating the file content and letting the index scan from 0 again.
	dir, _ := filepath.Split(p.outPath)
	newPath := filepath.Join(dir, "session.log")
	if err := os.WriteFile(newPath, nil, 0o644); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Reopen file for appending in the same writer helper semantics by calling write.
	// Our closure writes via fd opened in setup; truncation leaves fd position stale.
	// Instead, directly append via os helpers and refresh.
	f, err := os.OpenFile(newPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	for i := 6; i <= 10; i++ {
		if _, err := f.WriteString("post\n"); err != nil {
			t.Fatalf("write post: %v", err)
		}
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	p.Push("post") // seq 6
	p.Push("post") // seq 7
	p.Push("post") // seq 8
	p.Push("post") // seq 9
	p.Push("post") // seq 10
	st, err := f.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := p.RefreshIndex(st.Size()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Requesting pre-rotation seqs returns nothing (horizon guards).
	recs, err := p.Fetch(1, 5)
	if err != nil {
		t.Fatalf("Fetch pre: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("pre-rotation fetch len=%d, want 0", len(recs))
	}

	// Requesting post-rotation seqs that sit below the ring's live window comes off disk.
	recs, err = p.Fetch(6, 7)
	if err != nil {
		t.Fatalf("Fetch post: %v", err)
	}
	if len(recs) != 2 || recs[0].Seq != 6 || recs[1].Seq != 7 {
		t.Fatalf("post-rotation fetch: %+v", recs)
	}

	if got := p.Horizon(); got != 6 {
		t.Errorf("Horizon=%d, want 6", got)
	}
}

func TestRingStreamProvider_DiskFallback_PushUnderLock(t *testing.T) {
	// Regression check: Push must be safe against concurrent Fetch. Without the
	// internal mutex around buf.Push, the race detector would flag this test.
	p, _ := setupDiskProvider(t, 200)
	const N = 1000
	done := make(chan struct{})
	go func() {
		for i := range N {
			_, _ = p.Fetch(1, int64(i+1))
		}
		close(done)
	}()
	for range N {
		p.Push("x")
	}
	<-done
}
