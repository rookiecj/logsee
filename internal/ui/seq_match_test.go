package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

func newSeqMatchModel(t *testing.T) *Model {
	t.Helper()
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 25
	return m
}

func TestNextMatchIdxInFidx_forwardFindsFirstStrictlyGreater(t *testing.T) {
	m := newSeqMatchModel(t)
	recs := []domain.Line{
		{Seq: 10, Text: "a"},
		{Seq: 20, Text: "b"},
		{Seq: 30, Text: "c"},
		{Seq: 40, Text: "d"},
	}
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()

	idx := m.nextMatchIdxInFidx(fidx, 20, +1, nil)
	if idx != 2 {
		t.Fatalf("forward from 20: want idx=2 (seq 30), got %d", idx)
	}
	// Strictly greater: from 30, the next is 40 (idx 3).
	idx = m.nextMatchIdxInFidx(fidx, 30, +1, nil)
	if idx != 3 {
		t.Fatalf("forward from 30: want idx=3 (seq 40), got %d", idx)
	}
	// No more: from 40 forward → -1.
	idx = m.nextMatchIdxInFidx(fidx, 40, +1, nil)
	if idx != -1 {
		t.Fatalf("forward from 40 (last): want -1, got %d", idx)
	}
}

func TestNextMatchIdxInFidx_backwardFindsLastStrictlyLess(t *testing.T) {
	m := newSeqMatchModel(t)
	recs := []domain.Line{
		{Seq: 10, Text: "a"},
		{Seq: 20, Text: "b"},
		{Seq: 30, Text: "c"},
	}
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()

	idx := m.nextMatchIdxInFidx(fidx, 30, -1, nil)
	if idx != 1 {
		t.Fatalf("backward from 30: want idx=1 (seq 20), got %d", idx)
	}
	idx = m.nextMatchIdxInFidx(fidx, 10, -1, nil)
	if idx != -1 {
		t.Fatalf("backward from 10 (first): want -1, got %d", idx)
	}
}

func TestNextMatchIdxInFidx_predicateFiltersRecords(t *testing.T) {
	m := newSeqMatchModel(t)
	recs := []domain.Line{
		{Seq: 1, Text: "alpha"},
		{Seq: 2, Text: "beta"},
		{Seq: 3, Text: "alpha beta"},
		{Seq: 4, Text: "gamma"},
		{Seq: 5, Text: "beta"},
	}
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()

	// Predicate: text contains "beta".
	betaPred := func(rec domain.Line) bool {
		return containsBeta(rec.Text)
	}
	// Forward from seq=2: next beta is seq=3 (idx 2).
	idx := m.nextMatchIdxInFidx(fidx, 2, +1, betaPred)
	if idx != 2 {
		t.Fatalf("forward beta from 2: want idx=2 (seq 3), got %d", idx)
	}
	// Backward from seq=4: previous beta is seq=3 (idx 2).
	idx = m.nextMatchIdxInFidx(fidx, 4, -1, betaPred)
	if idx != 2 {
		t.Fatalf("backward beta from 4: want idx=2 (seq 3), got %d", idx)
	}
}

func TestNextMatchIdxInFidx_emptyFidxReturnsNegOne(t *testing.T) {
	m := newSeqMatchModel(t)
	idx := m.nextMatchIdxInFidx(nil, 10, +1, nil)
	if idx != -1 {
		t.Fatalf("empty fidx: want -1, got %d", idx)
	}
}

func TestNextMatchIdxInFidx_respectsFilterProjectionInFidx(t *testing.T) {
	// fidx only contains filter-passing rows → helper with pred=nil works over the projection.
	m := newSeqMatchModel(t)
	p, err := filter.Parse("keep")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "keep"
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "drop"},
		{Seq: 2, Text: "keep"},
		{Seq: 3, Text: "drop"},
		{Seq: 4, Text: "keep"},
		{Seq: 5, Text: "drop"},
		{Seq: 6, Text: "keep"},
	})
	fidx := m.filteredIndices()
	if len(fidx) != 3 {
		t.Fatalf("setup: want 3 filter-passing rows, got %d", len(fidx))
	}

	idx := m.nextMatchIdxInFidx(fidx, 2, +1, nil)
	if idx != 1 {
		t.Fatalf("forward over filtered fidx from seq 2: want idx=1 (seq 4), got %d", idx)
	}
	idx = m.nextMatchIdxInFidx(fidx, 4, -1, nil)
	if idx != 0 {
		t.Fatalf("backward over filtered fidx from seq 4: want idx=0 (seq 2), got %d", idx)
	}
}

func TestSearchPredicate_matchesCommittedQuery(t *testing.T) {
	m := newSeqMatchModel(t)
	m.searchBuf = "error"
	pred := m.searchPredicate()
	cases := []struct {
		text string
		want bool
	}{
		{"an error occurred", true},
		{"ERROR (case-sensitive)", false},
		{"no match here", false},
	}
	for _, c := range cases {
		got := pred(domain.Line{Seq: 1, Text: c.text})
		if got != c.want {
			t.Errorf("searchPredicate(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestFilterPredicate_matchesCurrentProgram(t *testing.T) {
	m := newSeqMatchModel(t)
	p, err := filter.Parse("warn")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "warn"
	pred := m.filterPredicate()
	if !pred(domain.Line{Seq: 1, Text: "warning: disk full"}) {
		t.Fatal("warn predicate: want true for 'warning: disk full'")
	}
	if pred(domain.Line{Seq: 1, Text: "info: ok"}) {
		t.Fatal("warn predicate: want false for 'info: ok'")
	}
}

func containsBeta(s string) bool {
	for i := 0; i+4 <= len(s); i++ {
		if s[i:i+4] == "beta" {
			return true
		}
	}
	return false
}
