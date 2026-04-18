package ui

import (
	"sync"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
)

func TestRingStreamProvider_FetchWithinRing(t *testing.T) {
	r := buffer.NewRing(100)
	p := NewRingStreamProvider(r)
	for i := 1; i <= 10; i++ {
		r.Push("line")
		p.NoteReceived(1)
	}

	recs, err := p.Fetch(3, 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := len(recs); got != 5 {
		t.Fatalf("len=%d, want 5", got)
	}
	for i, rec := range recs {
		wantSeq := int64(3 + i)
		if rec.Seq != wantSeq {
			t.Errorf("recs[%d].Seq=%d, want %d", i, rec.Seq, wantSeq)
		}
	}

	if got := p.TotalLines(); got != 10 {
		t.Errorf("TotalLines=%d, want 10", got)
	}
	if got := p.FileSize(); got != 0 {
		t.Errorf("FileSize=%d, want 0 (stream has no backing bytes)", got)
	}
	if got := p.EstimateBytes(1, 10); got != 0 {
		t.Errorf("EstimateBytes=%d, want 0 (no byte guardrail for stream)", got)
	}
}

func TestRingStreamProvider_FetchAfterEviction(t *testing.T) {
	// max=3 forces evictions; seqs 1..2 drop out once seq 4 arrives.
	r := buffer.NewRing(3)
	p := NewRingStreamProvider(r)
	for i := 1; i <= 10; i++ {
		r.Push("line")
		p.NoteReceived(1)
	}

	// Live ring now holds seqs 8,9,10.
	recs, err := p.Fetch(1, 5)
	if err != nil {
		t.Fatalf("Fetch evicted: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("evicted seqs fetched %d recs, want 0", len(recs))
	}

	recs, err = p.Fetch(8, 10)
	if err != nil {
		t.Fatalf("Fetch live: %v", err)
	}
	if got := len(recs); got != 3 {
		t.Fatalf("live len=%d, want 3", got)
	}
	if recs[0].Seq != 8 || recs[2].Seq != 10 {
		t.Errorf("live seqs=%d..%d, want 8..10", recs[0].Seq, recs[2].Seq)
	}

	// TotalLines reflects cumulative receive, not live ring size.
	if got := p.TotalLines(); got != 10 {
		t.Errorf("TotalLines after eviction=%d, want 10 (monotonic)", got)
	}
}

func TestRingStreamProvider_TotalLinesMonotonic(t *testing.T) {
	r := buffer.NewRing(2)
	p := NewRingStreamProvider(r)

	steps := []int{1, 1, 1, 5, 0, 3}
	want := int64(0)
	for _, n := range steps {
		for range n {
			r.Push("x")
		}
		p.NoteReceived(n)
		want += int64(n)
		if got := p.TotalLines(); got != want {
			t.Fatalf("after NoteReceived(%d): TotalLines=%d, want %d", n, got, want)
		}
	}

	// Negative/zero n must not decrement.
	p.NoteReceived(0)
	p.NoteReceived(-5)
	if got := p.TotalLines(); got != want {
		t.Errorf("TotalLines must be monotonic: got %d, want %d", got, want)
	}
}

func TestRingStreamProvider_ConcurrentPushFetch(t *testing.T) {
	// Race-detector coverage: one goroutine pushes + notes receives, another fetches.
	// The ring is not itself thread-safe (UI-thread only in production), so Push is
	// serialized via p.mu by gating it through a helper that takes the lock.
	r := buffer.NewRing(500)
	p := NewRingStreamProvider(r)

	pushUnderLock := func(text string) {
		p.mu.Lock()
		r.Push(text)
		p.mu.Unlock()
		p.NoteReceived(1)
	}

	const N = 2000
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range N {
			pushUnderLock("line")
		}
	}()

	go func() {
		defer wg.Done()
		for i := range N {
			_, _ = p.Fetch(1, int64(i+1))
		}
	}()

	wg.Wait()

	if got := p.TotalLines(); got != int64(N) {
		t.Errorf("TotalLines=%d, want %d", got, N)
	}
}
