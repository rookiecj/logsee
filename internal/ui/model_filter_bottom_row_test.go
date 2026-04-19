package ui

import (
	"strconv"
	"testing"

	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

// TestFilterNavDown_CursorStaysOnBottomRowWhenFilterSparse reproduces the
// user report "154 → 155 next match with filter level:D parks the cursor
// in the middle of the viewport".
//
// Pre-fix: after the filter top-up, the forward nav-advance branch set
// viewTopSeq = cursorSeq - (vh-1) in *file-line* space. With the filter
// dropping lines between `cursorSeq-(vh-1)` and `cursorSeq`, the computed
// top seq maps to a filtered index much closer to the cursor than vh-1
// rows, so syncIdxFromSeq placed the cursor mid-viewport.
//
// Post-fix: viewTopSeq is computed through fidx (the record (vh-1) rows
// above the cursor), so the cursor lands on the bottom row regardless of
// filter density.
func TestFilterNavDown_CursorStaysOnBottomRowWhenFilterSparse(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 4 {
		t.Fatalf("vh too small for scenario: %d", vh)
	}

	// Filter "match" — every `density`-th record passes. fidx is therefore
	// ~1/density as dense as Seq space, so the pre-fix Seq-arithmetic
	// viewTopSeq miscomputed the top row.
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	const density = 3
	// Initial window sized to stay under the default Ring(1000) capacity
	// so no eviction pollutes the scenario.
	const initialCount = 800
	initial := make([]domain.Line, 0, initialCount)
	for s := int64(1); s <= initialCount; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if s%density == 0 {
			text = "match " + strconv.FormatInt(s, 10)
		}
		initial = append(initial, domain.Line{Seq: s, Text: text})
	}
	m.buf.ReplaceRecords(initial)
	m.fileWinFirst = 1
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000

	fidx := m.filteredIndices()
	if len(fidx) < vh+1 {
		t.Fatalf("setup: need >vh matches, got %d (vh=%d)", len(fidx), vh)
	}

	// Cursor at the last visible filtered row (bottom of viewport).
	m.cursorIdx = len(fidx) - 1
	m.scrollTop = m.cursorIdx - vh + 1
	m.syncSeqFromIdx(fidx)
	oldCursorSeq := m.buf.At(fidx[m.cursorIdx]).Seq

	// j at the boundary triggers a filter scan with nav-advance=+1.
	cmd := m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)
	if cmd == nil {
		t.Fatal("expected filter scan cmd at boundary")
	}
	if m.filterTopupNavAdvance != +1 {
		t.Fatalf("want filterTopupNavAdvance=+1, got %d", m.filterTopupNavAdvance)
	}

	// Small forward scan: 100 fresh records so the merged ring keeps all
	// records (800 + 100 ≤ 1000 capacity) and the match immediately after
	// oldCursorSeq is well past the filtered position (vh-1) above it.
	firstLine := int64(initialCount + 1)
	const scanCount = 100
	newRecs := make([]domain.Line, 0, scanCount)
	for s := firstLine; s < firstLine+scanCount; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if s%density == 0 {
			text = "match " + strconv.FormatInt(s, 10)
		}
		newRecs = append(newRecs, domain.Line{Seq: s, Text: text})
	}
	next, _ := m.Update(FilterScanResultMsg{
		Records:    newRecs,
		FirstLine:  firstLine,
		Direction:  +1,
		ReachedEnd: false,
	})
	m2 := next.(*Model)
	fidx2 := m2.filteredIndices()

	// Cursor must sit on the bottom row, not mid-viewport.
	row := m2.cursorIdx - m2.scrollTop
	if row != vh-1 {
		t.Errorf("cursor row after filter nav-advance: got %d, want %d (bottom) — cursorIdx=%d scrollTop=%d fidxLen=%d",
			row, vh-1, m2.cursorIdx, m2.scrollTop, len(fidx2))
	}

	// Sanity: cursor advanced past the old cursor's seq and still on a match.
	if m2.cursorIdx < 0 || m2.cursorIdx >= len(fidx2) {
		t.Fatalf("cursorIdx %d out of fidx range %d", m2.cursorIdx, len(fidx2))
	}
	rec := m2.buf.At(fidx2[m2.cursorIdx])
	if rec.Seq <= oldCursorSeq {
		t.Errorf("cursor did not advance: cursorSeq=%d, old=%d", rec.Seq, oldCursorSeq)
	}
	if len(rec.Text) < 5 || rec.Text[:5] != "match" {
		t.Errorf("cursor landed on non-match text %q", rec.Text)
	}
}
