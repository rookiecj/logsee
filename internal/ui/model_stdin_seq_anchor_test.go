package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/filter"
)

// Phase 3 (docs/plans/stdin-fileprovider-unify-plan.md): after applyIncomingLines runs in the
// stdin path, cursorSeq / viewTopSeq must match the ring record referenced by cursorIdx /
// scrollTop. The file path maintains this invariant via syncSeqFromIdx; stdin should too, so
// that later phases can treat both paths uniformly.

func TestStdinSeqAnchor_FirstBatchSetsAnchors(t *testing.T) {
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 25

	m.applyIncomingLines([]string{"a", "b", "c"})

	fidx := m.filteredIndices()
	if len(fidx) != 3 {
		t.Fatalf("fidx len=%d, want 3", len(fidx))
	}
	wantCursor := m.buf.At(fidx[m.cursorIdx]).Seq
	if m.cursorSeq != wantCursor {
		t.Errorf("cursorSeq=%d, want %d (buf.At(fidx[cursorIdx]).Seq)", m.cursorSeq, wantCursor)
	}
	wantTop := m.buf.At(fidx[m.scrollTop]).Seq
	if m.viewTopSeq != wantTop {
		t.Errorf("viewTopSeq=%d, want %d", m.viewTopSeq, wantTop)
	}
}

func TestStdinSeqAnchor_InvariantAcrossBatches(t *testing.T) {
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 25

	batches := [][]string{
		{"a"},
		{"b", "c"},
		{"d", "e", "f"},
		{"g"},
	}
	for i, b := range batches {
		m.applyIncomingLines(b)
		fidx := m.filteredIndices()
		if len(fidx) == 0 {
			t.Fatalf("batch %d: empty fidx", i)
		}
		if got, want := m.cursorSeq, m.buf.At(fidx[m.cursorIdx]).Seq; got != want {
			t.Errorf("batch %d: cursorSeq=%d, want %d", i, got, want)
		}
		if got, want := m.viewTopSeq, m.buf.At(fidx[m.scrollTop]).Seq; got != want {
			t.Errorf("batch %d: viewTopSeq=%d, want %d", i, got, want)
		}
	}
}

func TestStdinSeqAnchor_SurvivesEviction(t *testing.T) {
	// max=3 forces eviction; Seq keeps incrementing so anchors must reference current ring tail.
	r := buffer.NewRing(3)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 25

	for i := range 10 {
		m.applyIncomingLines([]string{string(rune('a' + i))})
	}
	fidx := m.filteredIndices()
	if len(fidx) != 3 {
		t.Fatalf("fidx len=%d, want 3 (ring cap)", len(fidx))
	}

	wantCursor := m.buf.At(fidx[m.cursorIdx]).Seq
	if m.cursorSeq != wantCursor {
		t.Errorf("cursorSeq=%d, want %d", m.cursorSeq, wantCursor)
	}
	// After 10 pushes with cap 3, live seqs are 8,9,10. Tail-follow places cursor on last (Seq=10).
	if m.cursorSeq != 10 {
		t.Errorf("cursorSeq=%d, expected 10 (tail-follow)", m.cursorSeq)
	}
}

func TestStdinSeqAnchor_EmptyFidxClearsAnchors(t *testing.T) {
	// When filter hides every line, both anchors should be 0.
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 25

	m.applyIncomingLines([]string{"aaa"})
	// Apply a filter that rejects everything.
	prog, err := filter.Parse("bbb")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = prog
	m.appliedFilter = "bbb"

	fidx := m.filteredIndices()
	if len(fidx) != 0 {
		t.Fatalf("fidx len=%d, want 0 after non-matching filter", len(fidx))
	}
	// Trigger another batch to re-run the sync hook.
	m.applyIncomingLines([]string{"ccc"})
	fidx = m.filteredIndices()
	if len(fidx) != 0 {
		t.Fatalf("fidx len=%d, want 0 (filter still non-matching)", len(fidx))
	}
	if m.cursorSeq != 0 || m.viewTopSeq != 0 {
		t.Errorf("anchors should clear when fidx empty: cursorSeq=%d viewTopSeq=%d", m.cursorSeq, m.viewTopSeq)
	}
}
