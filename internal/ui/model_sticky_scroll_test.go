package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

// Phase 4 replaces the sticky-pin mechanism with seq-anchor driven scroll derivation. These tests
// exercise the same regressions (cursor mid-viewport after boundary reload / filter prepend) but
// assert on the seq-anchor flow instead of a flag.

func TestApplyFileWindowLoaded_seqAnchorPinsCursorTopRowAfterNavUp(t *testing.T) {
	// Given: async window load after k at top edge; seq anchors were set by maybeFileLoadAfterNavUp
	// (cursorSeq=G-1, viewTopSeq=G-1). Stale scrollTop from the prior buffer must not leak through.
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/a.log", "", nil, nil, nil)
	m.width, m.height = 80, 30
	m.lineWrap = false
	m.filePartial = true
	m.scrollTop = 100 // stale, pre-reload
	vh := m.viewportH()
	// Seq anchors set by maybeFileLoadAfterNavUp style: cursorSeq and viewTopSeq both = G-1 = 5.
	m.cursorSeq = 5
	m.viewTopSeq = 5
	n := vh + 8
	recs := make([]domain.Record, n)
	for i := range recs {
		recs[i] = domain.Record{Seq: int64(i + 1), Text: "line"}
	}
	m.applyFileWindowLoaded(recs, 1)

	fidx := m.filteredIndices()
	if len(fidx) <= vh {
		t.Fatalf("Given: want L>vh, got %d vh=%d", len(fidx), vh)
	}
	if m.cursorIdx != 4 {
		t.Fatalf("Then: cursorIdx for seq 5 want 4, got %d", m.cursorIdx)
	}
	if m.scrollTop != m.cursorIdx {
		t.Fatalf("Then: want cursor on viewport row 0 (scrollTop=%d==cursorIdx=%d), got scrollTop=%d",
			m.cursorIdx, m.cursorIdx, m.scrollTop)
	}
}

func TestFilterScanResultMsg_backwardPrepend_keepsCursorOnFocusSeqTopRow(t *testing.T) {
	// Given: filter-topup backward scan prepended records; cursor was on the old first match.
	// Phase 4: handler re-anchors cursorSeq/viewTopSeq and runs syncIdxFromSeq to pin top row.
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.lineWrap = false
	m.filePartial = true
	vh := m.viewportH()
	p, err := filter.Parse("x")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "x"
	m.filterTopupActive = true
	m.filterTopupDir = -1

	prepN := vh + 3
	oldN := vh + 5
	recs := make([]domain.Record, oldN)
	for i := range recs {
		recs[i] = domain.Record{Seq: int64(prepN + i + 1), Text: "x"}
	}
	m.buf.ReplaceRecords(recs)
	m.cursorIdx = 0
	m.scrollTop = 0
	m.fileWinFirst = int64(prepN + 1)
	focusSeq := m.buf.At(m.filteredIndices()[m.cursorIdx]).Seq

	pre := make([]domain.Record, prepN)
	for i := range pre {
		pre[i] = domain.Record{Seq: int64(i + 1), Text: "x"}
	}
	next, _ := m.Update(FilterScanResultMsg{
		Records:    pre,
		FirstLine:  1,
		Direction:  -1,
		ReachedEnd: false,
	})
	m2 := next.(*Model)
	fidx := m2.filteredIndices()
	if len(fidx) <= vh {
		t.Fatalf("Given: need len(fidx)>vh, got %d vh=%d", len(fidx), vh)
	}
	// Non-nav topup keeps cursor on focusSeq via remap.
	if m2.buf.At(fidx[m2.cursorIdx]).Seq != focusSeq {
		t.Fatalf("Then: want cursor still on seq %d", focusSeq)
	}
}

func TestMaybeFileLoadAfterNavUp_unfilteredUsesAroundGMinus1(t *testing.T) {
	// Unfiltered k at top: single physical-step reload via Around(G-1); cursor seq anchor advances to G-1.
	r := buffer.NewRing(10000)
	m := NewModel(r, nil, false, false, "", "/tmp/a.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/a.log"
	m.width = 80
	m.height = 25
	m.fileOffsets = make([]int64, 6000)
	m.fileTotalLines = 6000
	m.fileWinFirst = 2000
	recs := make([]domain.Record, 50)
	for i := range recs {
		recs[i] = domain.Record{Seq: int64(2000 + i), Text: "line"}
	}
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()
	m.cursorIdx = 0
	cmd := m.maybeFileLoadAfterNavUp(fidx, 0)
	if cmd == nil {
		t.Fatal("Then: expected load cmd")
	}
	if m.cursorSeq != 1999 {
		t.Fatalf("Then: want Around(G-1) cursorSeq=1999, got %d", m.cursorSeq)
	}
}

func TestMaybeFileLoadAfterNavUp_filteredDelegatesToFilterScan(t *testing.T) {
	r := buffer.NewRing(10000)
	m := NewModel(r, nil, false, false, "", "/tmp/a.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/a.log"
	m.width = 80
	m.height = 25
	m.fileOffsets = make([]int64, 6000)
	m.fileTotalLines = 6000
	m.fileWinFirst = 2000
	recs := make([]domain.Record, 50)
	for i := range recs {
		recs[i] = domain.Record{Seq: int64(2000 + i), Text: "x"}
	}
	p, err := filter.Parse("x")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "x"
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()
	m.cursorIdx = 0
	cmd := m.maybeFileLoadAfterNavUp(fidx, 0)
	if cmd == nil {
		t.Fatal("Then: expected filter scan cmd")
	}
	if m.filterTopupNavAdvance != -1 {
		t.Fatalf("Then: want filterTopupNavAdvance=-1, got %d", m.filterTopupNavAdvance)
	}
	if m.filterTopupDir != -1 {
		t.Fatalf("Then: want filterTopupDir=-1, got %d", m.filterTopupDir)
	}
	if !m.filterTopupActive {
		t.Fatal("Then: want filterTopupActive=true")
	}
	// Filter-scan path does not set cursorSeq directly; it is updated by the async result handler.
	// Here we just assert the delegation state is correct.
	_ = cmd
}

func TestMaybeFileLoadAfterNavDown_filteredDelegatesToFilterScan(t *testing.T) {
	r := buffer.NewRing(10000)
	m := NewModel(r, nil, false, false, "", "/tmp/a.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/a.log"
	m.width = 80
	m.height = 25
	m.fileOffsets = make([]int64, 6000)
	m.fileTotalLines = 6000
	m.fileWinFirst = 2000
	recs := make([]domain.Record, 50)
	for i := range recs {
		recs[i] = domain.Record{Seq: int64(2000 + i), Text: "x"}
	}
	p, err := filter.Parse("x")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "x"
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	cmd := m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)
	if cmd == nil {
		t.Fatal("Then: expected filter scan cmd")
	}
	if m.filterTopupNavAdvance != +1 {
		t.Fatalf("Then: want filterTopupNavAdvance=+1, got %d", m.filterTopupNavAdvance)
	}
	if m.filterTopupDir != +1 {
		t.Fatalf("Then: want filterTopupDir=+1, got %d", m.filterTopupDir)
	}
	if !m.filterTopupActive {
		t.Fatal("Then: want filterTopupActive=true")
	}
	// Filter-scan path does not set cursorSeq directly; it is updated by the async result handler.
	// Here we just assert the delegation state is correct.
	_ = cmd
}

var _ = domain.Record{} // keep import anchored after Phase 4 deletions
