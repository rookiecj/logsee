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

// TestSearchNavNext_CursorStaysOnBottomRowWhenFilterSparse reproduces the
// user's follow-up report: with a filter like `level:D` and a highlight
// query committed, continuous `n` (next highlight hit) lands the cursor
// mid-viewport after a disk-scan jump.
//
// Pre-fix: cmdLoadFileWindowAroundBottom set viewTopSeq = globalLine -
// (vh-1) in raw file-line Seq space. applyFileWindowLoaded then fed
// that to syncIdxFromSeq, which — because fidx is sparser than Seq
// space under a filter — resolved the seq to an idx just a few
// filtered rows above the cursor, anchoring the viewport top too high
// and leaving the cursor in the middle.
//
// Post-fix: applyFileWindowLoaded re-anchors viewTopSeq through fidx
// when preferBottom is set.
func TestSearchNavNext_CursorStaysOnBottomRowWhenFilterSparse(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 4 {
		t.Fatalf("vh too small for scenario: %d", vh)
	}

	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	// 1..900 records, every 3rd is a filtered-in "match". Dense enough
	// that one viewport of matches occupies >> vh file lines, so raw-seq
	// arithmetic and fidx arithmetic diverge sharply.
	const density = 3
	const total = 900
	recs := make([]domain.Line, 0, total)
	for s := int64(1); s <= total; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if s%density == 0 {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Line{Seq: s, Text: text})
	}
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000

	// Simulate SearchScanResult arriving with a found seq deep in the
	// file: the handler invokes cmdLoadFileWindowAroundBottom, which
	// pins cursor to that seq on the viewport's bottom row.
	const foundSeq = int64(600) // must be a match (600 % 3 == 0)
	m.cursorSeq = foundSeq
	top := foundSeq - int64(vh-1)
	if top < 1 {
		top = 1
	}
	m.viewTopSeq = top

	// Window load result arrives: applyFileWindowLoaded merges and
	// resolves scroll state from the cursor/view anchors.
	m.applyFileWindowLoaded(recs, 1)

	fidx := m.filteredIndices()
	row := m.cursorIdx - m.scrollTop
	if row != vh-1 {
		t.Errorf("cursor row after search nav-next disk load: got %d, want %d (bottom) — cursorIdx=%d scrollTop=%d fidxLen=%d",
			row, vh-1, m.cursorIdx, m.scrollTop, len(fidx))
	}
	if m.buf.At(fidx[m.cursorIdx]).Seq != foundSeq {
		t.Errorf("cursor should land on foundSeq=%d, got %d", foundSeq, m.buf.At(fidx[m.cursorIdx]).Seq)
	}
}
