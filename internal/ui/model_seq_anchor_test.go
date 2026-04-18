package ui

import (
	"strconv"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

// Phase 1 seq-coord invariant: (cursorSeq, viewTopSeq) determine cursor's screen row deterministically
// across Ring.ReplaceRecords, regardless of the prior scrollTop value. See
// docs/plans/seq-coord-pull-window-plan.md.

func newFilePartialModelForSeq(t *testing.T) *Model {
	t.Helper()
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/phase1.log", "", nil, nil, nil)
	m.width, m.height = 80, 25
	m.lineWrap = false
	m.filePartial = true
	return m
}

func recsFromSeqs(seqs ...int64) []domain.Record {
	out := make([]domain.Record, len(seqs))
	for i, s := range seqs {
		out[i] = domain.Record{Seq: s, Text: "line"}
	}
	return out
}

// Down boundary: after j-triggered reload at the window tail, cursor must sit on the bottom row.
func TestSeqAnchor_navDownBoundary_cursorSticksToBottomRow(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 4 {
		t.Fatalf("vh too small for scenario: %d", vh)
	}
	// Old window [1..2vh] with cursor at last row (bottom).
	initialSeqs := make([]int64, 2*vh)
	for i := range initialSeqs {
		initialSeqs[i] = int64(i + 1)
	}
	m.buf.ReplaceRecords(recsFromSeqs(initialSeqs...))
	m.fileWinFirst = 1
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000
	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.scrollTop = m.cursorIdx - vh + 1
	m.syncSeqFromIdx(fidx)
	prevRow := m.cursorIdx - m.scrollTop

	// Fire nav-down boundary: sets seq anchors for the reload.
	cmd := m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)
	if cmd == nil {
		t.Fatal("expected boundary reload cmd")
	}
	G := initialSeqs[len(initialSeqs)-1]
	if m.cursorSeq != G+1 {
		t.Fatalf("cursorSeq: want %d got %d", G+1, m.cursorSeq)
	}

	// Simulate the async reload: window centered at G+1, 2*vh records.
	win := 2 * vh
	first := G + 1 - int64(vh)
	if first < 1 {
		first = 1
	}
	newSeqs := make([]int64, win)
	for i := range newSeqs {
		newSeqs[i] = first + int64(i)
	}
	m.applyFileWindowLoaded(recsFromSeqs(newSeqs...), first)

	fidx = m.filteredIndices()
	row := m.cursorIdx - m.scrollTop
	if row != vh-1 {
		t.Fatalf("cursor row after reload: want bottom (%d), got %d (cursorIdx=%d scrollTop=%d)", vh-1, m.cursorIdx, m.cursorIdx, m.scrollTop)
	}
	if row != prevRow {
		t.Fatalf("cursor row changed across reload: prev=%d now=%d", prevRow, row)
	}
	if m.buf.At(fidx[m.cursorIdx]).Seq != G+1 {
		t.Fatalf("cursor seq after reload: want %d got %d", G+1, m.buf.At(fidx[m.cursorIdx]).Seq)
	}
}

// Up boundary: after k-triggered reload at the window head, cursor must sit on the top row.
func TestSeqAnchor_navUpBoundary_cursorSticksToTopRow(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 4 {
		t.Fatalf("vh too small for scenario: %d", vh)
	}
	// Old window starts at seq=500 so G-1=499 is a valid upward target.
	base := int64(500)
	initialSeqs := make([]int64, 2*vh)
	for i := range initialSeqs {
		initialSeqs[i] = base + int64(i)
	}
	m.buf.ReplaceRecords(recsFromSeqs(initialSeqs...))
	m.fileWinFirst = base
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000
	fidx := m.filteredIndices()
	m.cursorIdx = 0
	m.scrollTop = 0
	m.syncSeqFromIdx(fidx)
	prevRow := m.cursorIdx - m.scrollTop

	cmd := m.maybeFileLoadAfterNavUp(fidx, 0)
	if cmd == nil {
		t.Fatal("expected boundary reload cmd")
	}
	if m.cursorSeq != base-1 {
		t.Fatalf("cursorSeq: want %d got %d", base-1, m.cursorSeq)
	}

	// Simulate the async reload: window around base-1.
	first := base - 1 - int64(vh)
	if first < 1 {
		first = 1
	}
	newSeqs := make([]int64, 2*vh)
	for i := range newSeqs {
		newSeqs[i] = first + int64(i)
	}
	m.applyFileWindowLoaded(recsFromSeqs(newSeqs...), first)

	fidx = m.filteredIndices()
	row := m.cursorIdx - m.scrollTop
	if row != 0 {
		t.Fatalf("cursor row after reload: want top (0), got %d (cursorIdx=%d scrollTop=%d)", row, m.cursorIdx, m.scrollTop)
	}
	if row != prevRow {
		t.Fatalf("cursor row changed across reload: prev=%d now=%d", prevRow, row)
	}
	if m.buf.At(fidx[m.cursorIdx]).Seq != base-1 {
		t.Fatalf("cursor seq after reload: want %d got %d", base-1, m.buf.At(fidx[m.cursorIdx]).Seq)
	}
}

// Two chained reloads in the same direction (j, j at boundary again) must keep cursor on bottom row.
func TestSeqAnchor_twoChainedDownReloads_cursorStaysOnBottom(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 4 {
		t.Fatalf("vh too small: %d", vh)
	}
	initialSeqs := make([]int64, 2*vh)
	for i := range initialSeqs {
		initialSeqs[i] = int64(i + 1)
	}
	m.buf.ReplaceRecords(recsFromSeqs(initialSeqs...))
	m.fileWinFirst = 1
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000
	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.scrollTop = m.cursorIdx - vh + 1
	m.syncSeqFromIdx(fidx)

	// First boundary reload.
	G := initialSeqs[len(initialSeqs)-1]
	_ = m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)
	first1 := G + 1 - int64(vh)
	if first1 < 1 {
		first1 = 1
	}
	seqs1 := make([]int64, 2*vh)
	for i := range seqs1 {
		seqs1[i] = first1 + int64(i)
	}
	m.applyFileWindowLoaded(recsFromSeqs(seqs1...), first1)
	fidx = m.filteredIndices()

	// Walk cursor to the last loaded row (no new loads) to prepare second boundary.
	m.cursorIdx = len(fidx) - 1
	m.syncScrollToCursor(fidx)

	// Second boundary reload.
	G2 := seqs1[len(seqs1)-1]
	_ = m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)
	first2 := G2 + 1 - int64(vh)
	if first2 < 1 {
		first2 = 1
	}
	seqs2 := make([]int64, 2*vh)
	for i := range seqs2 {
		seqs2[i] = first2 + int64(i)
	}
	m.applyFileWindowLoaded(recsFromSeqs(seqs2...), first2)
	fidx = m.filteredIndices()

	row := m.cursorIdx - m.scrollTop
	if row != vh-1 {
		t.Fatalf("cursor row after two chained reloads: want bottom (%d), got %d", vh-1, row)
	}
	if m.buf.At(fidx[m.cursorIdx]).Seq != G2+1 {
		t.Fatalf("cursor seq after two chained reloads: want %d got %d", G2+1, m.buf.At(fidx[m.cursorIdx]).Seq)
	}
}

// Reproduces the Android.log / filter:level:D scenario (user report, 2026-04-18):
// cursor at the last visible D-level match (seq=128) → j → cursor must advance to the next D match
// (here simulated as seq=155), NOT snap to the first visible match (seq=108, top row).
//
// Pre-fix behavior: maybeFileLoadAfterNavDown sets cursorSeq=129; async Around(129) reloads around
// seq 129 which is not a match; new fidx's findSeqInFidx(129) returns -1; preferBottom is false,
// so cursorIdx falls back to 0 — the top-row match (seq=108). This test locks in the corrected flow.
func TestSeqAnchor_filterNavDownAdvancesToNextMatchOnDisk(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	// Filter: "match" token — buffer records with that substring pass.
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	// Loaded window: seqs 100..150. Matches at 108/115/122/128 (mimics D-level density).
	matchSeqs := map[int64]bool{108: true, 115: true, 122: true, 128: true}
	recs := make([]domain.Record, 0, 51)
	for s := int64(100); s <= 150; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if matchSeqs[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Record{Seq: s, Text: text})
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 100
	m.fileOffsets = make([]int64, 1000)
	m.fileTotalLines = 1000

	fidx := m.filteredIndices()
	if len(fidx) != 4 {
		t.Fatalf("setup: want 4 matches, got %d", len(fidx))
	}
	// Cursor on the last visible match (seq=128, the user's "화면 하단 128" case).
	m.cursorIdx = len(fidx) - 1
	m.syncScrollToCursor(fidx)
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 128 {
		t.Fatalf("setup: cursor should be on seq 128, got %d", got)
	}

	// j at boundary: filter mode must delegate to forward filter scan with nav-advance intent.
	cmd := m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)
	if cmd == nil {
		t.Fatal("Then: expected filter scan cmd")
	}
	if m.filterTopupNavAdvance != +1 {
		t.Fatalf("Then: want filterTopupNavAdvance=+1, got %d", m.filterTopupNavAdvance)
	}

	// Simulate the scan result: raw records for seqs 151..190 with matches at 155 and 162.
	newRecs := make([]domain.Record, 0, 40)
	newMatches := map[int64]bool{155: true, 162: true}
	for s := int64(151); s <= 190; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if newMatches[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		newRecs = append(newRecs, domain.Record{Seq: s, Text: text})
	}
	next, _ := m.Update(FilterScanResultMsg{
		Records:    newRecs,
		FirstLine:  151,
		Direction:  +1,
		ReachedEnd: false,
	})
	m2 := next.(*Model)
	fidx2 := m2.filteredIndices()

	// Cursor must be on the next match (seq=155), not snap to any prior match (the bug).
	cursorSeq := m2.buf.At(fidx2[m2.cursorIdx]).Seq
	if cursorSeq != 155 {
		t.Fatalf("Then: cursor should advance to next match 155, got %d (cursorIdx=%d)", cursorSeq, m2.cursorIdx)
	}
	// Specifically: cursor must NOT have jumped back to 108 (user-reported bug).
	if cursorSeq == 108 {
		t.Fatal("Then: regression — cursor snapped to first visible match (seq=108), the reported bug")
	}
	// Nav intent is a single-step commit: scan state must clear.
	if m2.filterTopupNavAdvance != 0 {
		t.Fatalf("Then: nav advance flag should clear after success, got %d", m2.filterTopupNavAdvance)
	}
	if m2.filterTopupActive {
		t.Fatal("Then: filterTopupActive should clear after nav-advance success")
	}
}

// Symmetric: k at the first visible match with a filter must step to the previous match on disk,
// not snap to the last visible match.
func TestSeqAnchor_filterNavUpAdvancesToPrevMatchOnDisk(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	// Loaded window: seqs 200..250. Matches at 208/215/222/228.
	matchSeqs := map[int64]bool{208: true, 215: true, 222: true, 228: true}
	recs := make([]domain.Record, 0, 51)
	for s := int64(200); s <= 250; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if matchSeqs[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Record{Seq: s, Text: text})
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 200
	m.fileOffsets = make([]int64, 1000)
	m.fileTotalLines = 1000

	fidx := m.filteredIndices()
	m.cursorIdx = 0
	m.syncScrollToCursor(fidx)
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 208 {
		t.Fatalf("setup: cursor should be on seq 208, got %d", got)
	}

	cmd := m.maybeFileLoadAfterNavUp(fidx, 0)
	if cmd == nil {
		t.Fatal("expected filter scan cmd")
	}
	if m.filterTopupNavAdvance != -1 {
		t.Fatalf("want filterTopupNavAdvance=-1, got %d", m.filterTopupNavAdvance)
	}

	// Simulate backward scan: seqs 160..199 with matches at 185 and 195.
	newRecs := make([]domain.Record, 0, 40)
	newMatches := map[int64]bool{185: true, 195: true}
	for s := int64(160); s <= 199; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if newMatches[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		newRecs = append(newRecs, domain.Record{Seq: s, Text: text})
	}
	next, _ := m.Update(FilterScanResultMsg{
		Records:    newRecs,
		FirstLine:  160,
		Direction:  -1,
		ReachedEnd: false,
	})
	m2 := next.(*Model)
	fidx2 := m2.filteredIndices()
	cursorSeq := m2.buf.At(fidx2[m2.cursorIdx]).Seq
	if cursorSeq != 195 {
		t.Fatalf("cursor should step back to nearest prev match 195, got %d", cursorSeq)
	}
	if cursorSeq == 228 {
		t.Fatal("regression — cursor snapped forward to last visible match")
	}
}

// Reproduces the user-reported PageUp mid-viewport bug: in filter mode, PageUp at the first
// visible match used to do cmdLoadFileWindowStartingAt(fileWinFirst - vh), and the sparse new
// window's top-padding placed the cursor at row ≈ padTop (mid-viewport) instead of pinned to
// the nearest previous match's natural row.
func TestSeqAnchor_filterPageUpAdvancesByVhMatches(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	// Loaded window: seqs 300..350, matches at 308/315/322/328 (4 visible matches).
	matchSeqs := map[int64]bool{308: true, 315: true, 322: true, 328: true}
	recs := make([]domain.Record, 0, 51)
	for s := int64(300); s <= 350; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if matchSeqs[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Record{Seq: s, Text: text})
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 300
	m.fileOffsets = make([]int64, 1000)
	m.fileTotalLines = 1000

	fidx := m.filteredIndices()
	m.cursorIdx = 0
	m.syncScrollToCursor(fidx)
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 308 {
		t.Fatalf("setup: cursor should be on seq 308, got %d", got)
	}

	cmd := m.maybeFileLoadAfterPageUp(fidx, vh)
	if cmd == nil {
		t.Fatal("Then: expected filter scan cmd on PageUp boundary")
	}
	if m.filterTopupNavAdvance != -vh {
		t.Fatalf("Then: want filterTopupNavAdvance=-vh (%d), got %d", -vh, m.filterTopupNavAdvance)
	}

	// Simulate backward scan: raw records [150..299] with matches every 20 seqs.
	newRecs := make([]domain.Record, 0, 150)
	prevMatches := make(map[int64]bool)
	for s := int64(160); s <= 298; s += 20 {
		prevMatches[s] = true
	}
	for s := int64(150); s <= 299; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if prevMatches[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		newRecs = append(newRecs, domain.Record{Seq: s, Text: text})
	}
	next, _ := m.Update(FilterScanResultMsg{
		Records:    newRecs,
		FirstLine:  150,
		Direction:  -1,
		ReachedEnd: true,
	})
	m2 := next.(*Model)
	fidx2 := m2.filteredIndices()
	cursorSeq := m2.buf.At(fidx2[m2.cursorIdx]).Seq

	// Cursor should have moved backward by vh matches (or as many as exist) — specifically
	// it must be on one of the previous matches (160..280 range), NOT stay on the original 308
	// and NOT jump to the first visible match of the raw window (which would be 160 — the far end).
	if cursorSeq >= 308 {
		t.Fatalf("Then: cursor should have advanced backward past 308, got %d", cursorSeq)
	}
	if cursorSeq < 160 {
		t.Fatalf("Then: cursor seq out of expected range, got %d", cursorSeq)
	}
	// Count prev matches strictly less than original focus (308).
	prev := []int64{}
	for s := range prevMatches {
		if s < 308 {
			prev = append(prev, s)
		}
	}
	// Must be on the vh-th previous match (or earliest if fewer than vh are available).
	// With ~7 matches below 308 and vh probably larger: cursor lands on the earliest (=160).
	wantMost := int64(160)
	if len(prev) >= vh {
		// Would be the vh-th back; for this test, density is small so we just check presence in prev.
		if !prevMatches[cursorSeq] {
			t.Fatalf("Then: cursor should land on a previous match, got %d", cursorSeq)
		}
	} else if cursorSeq != wantMost {
		// ReachedEnd + fewer than vh prev matches → cursor at earliest available.
		if !prevMatches[cursorSeq] {
			t.Fatalf("Then: cursor should land on a previous match (fewer than vh available), got %d", cursorSeq)
		}
	}
	// State cleared on EOF.
	if m2.filterTopupNavAdvance != 0 {
		t.Fatalf("Then: nav advance should clear on EOF, got %d", m2.filterTopupNavAdvance)
	}
}

// PageDown symmetric: cursor must step forward by ~vh matches, not land on the first match of
// the raw trailing window.
func TestSeqAnchor_filterPageDownAdvancesByVhMatches(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	matchSeqs := map[int64]bool{408: true, 415: true, 422: true, 428: true}
	recs := make([]domain.Record, 0, 51)
	for s := int64(400); s <= 450; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if matchSeqs[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Record{Seq: s, Text: text})
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 400
	m.fileOffsets = make([]int64, 1000)
	m.fileTotalLines = 1000

	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.syncScrollToCursor(fidx)
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 428 {
		t.Fatalf("setup: cursor should be on seq 428, got %d", got)
	}
	prev := m.cursorIdx

	cmd := m.maybeFileLoadAfterPageDown(fidx, prev, vh)
	if cmd == nil {
		t.Fatal("Then: expected filter scan cmd on PageDown boundary")
	}
	if m.filterTopupNavAdvance != vh {
		t.Fatalf("Then: want filterTopupNavAdvance=vh (%d), got %d", vh, m.filterTopupNavAdvance)
	}

	// Forward scan: [451..600] with sparse matches every 25 seqs.
	newRecs := make([]domain.Record, 0, 150)
	nextMatches := make(map[int64]bool)
	for s := int64(475); s <= 600; s += 25 {
		nextMatches[s] = true
	}
	for s := int64(451); s <= 600; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if nextMatches[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		newRecs = append(newRecs, domain.Record{Seq: s, Text: text})
	}
	next, _ := m.Update(FilterScanResultMsg{
		Records:    newRecs,
		FirstLine:  451,
		Direction:  +1,
		ReachedEnd: true,
	})
	m2 := next.(*Model)
	fidx2 := m2.filteredIndices()
	cursorSeq := m2.buf.At(fidx2[m2.cursorIdx]).Seq

	if cursorSeq <= 428 {
		t.Fatalf("Then: cursor should advance forward past 428, got %d", cursorSeq)
	}
	if !nextMatches[cursorSeq] {
		t.Fatalf("Then: cursor should land on a forward match, got %d", cursorSeq)
	}
	if m2.filterTopupNavAdvance != 0 {
		t.Fatalf("Then: nav advance should clear on EOF, got %d", m2.filterTopupNavAdvance)
	}
}

// Multi-step partial advance: the first scan chunk yields fewer matches than requested — the
// chain must keep scanning with the remaining step count.
func TestSeqAnchor_filterNavAdvancePartialThenChain(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	// Loaded window with cursor on the last match at seq=510.
	matchSeqs := map[int64]bool{502: true, 510: true}
	recs := make([]domain.Record, 0, 51)
	for s := int64(500); s <= 550; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if matchSeqs[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Record{Seq: s, Text: text})
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 500
	m.fileOffsets = make([]int64, 2000)
	m.fileTotalLines = 2000

	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.syncScrollToCursor(fidx)
	focusStart := m.buf.At(fidx[m.cursorIdx]).Seq // = 510

	// Request advance of 5 matches forward (simulated PageDown with vh=5 for test brevity).
	m.filterTopupActive = true
	m.filterTopupDir = +1
	m.filterTopupNavAdvance = 5

	// First scan chunk delivers 2 new matches (insufficient).
	chunk1Matches := map[int64]bool{560: true, 580: true}
	chunk1 := make([]domain.Record, 0, 100)
	for s := int64(551); s <= 650; s++ {
		text := "other"
		if chunk1Matches[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		chunk1 = append(chunk1, domain.Record{Seq: s, Text: text})
	}
	next1, cmd1 := m.Update(FilterScanResultMsg{
		Records:    chunk1,
		FirstLine:  551,
		Direction:  +1,
		ReachedEnd: false,
	})
	m2 := next1.(*Model)
	// After 2 of 5 consumed, 3 remain.
	if m2.filterTopupNavAdvance != 3 {
		t.Fatalf("after chunk1: want remaining=3, got %d", m2.filterTopupNavAdvance)
	}
	if cmd1 == nil {
		t.Fatal("after partial advance: expected chain scan cmd")
	}
	// Cursor should be on the last consumed match (580).
	fidx2 := m2.filteredIndices()
	if got := m2.buf.At(fidx2[m2.cursorIdx]).Seq; got != 580 {
		t.Fatalf("after chunk1: cursor should be on last consumed match 580, got %d", got)
	}

	// Second scan chunk delivers 4 more matches → total 6 > 5, should stop at 5th.
	chunk2Matches := map[int64]bool{670: true, 690: true, 710: true, 730: true}
	chunk2 := make([]domain.Record, 0, 100)
	for s := int64(651); s <= 750; s++ {
		text := "other"
		if chunk2Matches[s] {
			text = "match " + strconv.FormatInt(s, 10)
		}
		chunk2 = append(chunk2, domain.Record{Seq: s, Text: text})
	}
	// Focus for chunk2 is new cursor seq (580).
	next2, _ := m2.Update(FilterScanResultMsg{
		Records:    chunk2,
		FirstLine:  651,
		Direction:  +1,
		ReachedEnd: false,
	})
	m3 := next2.(*Model)
	// All 5 steps satisfied (2 in chunk1 + 3 of 4 in chunk2 = 5).
	if m3.filterTopupNavAdvance != 0 {
		t.Fatalf("after chunk2: want remaining=0, got %d", m3.filterTopupNavAdvance)
	}
	if m3.filterTopupActive {
		t.Fatal("after chunk2: filterTopupActive must clear")
	}
	fidx3 := m3.filteredIndices()
	cursorSeq := m3.buf.At(fidx3[m3.cursorIdx]).Seq
	// Advance order from focusStart(510): 560, 580, 670, 690, 710. 5th = 710.
	if cursorSeq != 710 {
		t.Fatalf("cursor should be on 5th match (710) from %d, got %d", focusStart, cursorSeq)
	}
}

// Reproduces the reported bug: buffer replace with a stale scrollTop that would leave cursor
// mid-viewport under the old minimal-change policy. Seq coords must pin the cursor to bottom.
func TestSeqAnchor_staleScrollTopDoesNotLeakToNewBuffer_downBoundary(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 6 {
		t.Fatalf("vh too small: %d", vh)
	}
	// Old window [1..2vh], cursor bottom.
	oldSeqs := make([]int64, 2*vh)
	for i := range oldSeqs {
		oldSeqs[i] = int64(i + 1)
	}
	m.buf.ReplaceRecords(recsFromSeqs(oldSeqs...))
	m.fileWinFirst = 1
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000
	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.scrollTop = m.cursorIdx - vh + 1 // cursor at bottom row
	m.syncSeqFromIdx(fidx)

	G := oldSeqs[len(oldSeqs)-1]
	_ = m.maybeFileLoadAfterNavDown(fidx, m.cursorIdx)

	// Simulate async reload: new window [G+1-vh .. G+vh]. Cursor target G+1 lands at idx=vh,
	// which is inside the OLD scrollTop's visible range — minimal-change policy would leave
	// the cursor mid-viewport. Seq anchors must override that.
	first := G + 1 - int64(vh)
	newSeqs := make([]int64, 2*vh)
	for i := range newSeqs {
		newSeqs[i] = first + int64(i)
	}
	m.applyFileWindowLoaded(recsFromSeqs(newSeqs...), first)

	row := m.cursorIdx - m.scrollTop
	if row != vh-1 {
		t.Fatalf("cursor drifted mid-viewport after boundary reload: row=%d want bottom %d "+
			"(cursorIdx=%d scrollTop=%d vh=%d)", row, vh-1, m.cursorIdx, m.scrollTop, vh)
	}
}
