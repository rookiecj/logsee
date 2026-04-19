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
// Critical: the simulated window must match what
// cmdLoadFileWindowAroundBottom actually fetches —
// `[foundSeq-vh+1 .. foundSeq+vh]`. A full-file load hides the bug
// because the ring then contains every match the bottom-row placement
// needs, which isn't what happens in production.
//
// Pre-fix: the small 2*vh raw-line window holds only ~2*vh*density
// filtered matches (≈15 for density=3, vh=22), with the cursor roughly
// in the middle of that filtered slice. applyFileWindowLoaded's
// viewTopSeq = cursorSeq - (vh-1) in Seq space drops into fidx[0], so
// scrollTop=0 and cursor row = cursorIdx ≈ 7 (middle of the viewport).
//
// Post-fix: cmdLoadFileWindowAroundBottom loads a back-skewed window
// large enough for the filter to deliver (vh-1) matches above the
// cursor; applyFileWindowLoaded re-anchors viewTopSeq through fidx.
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

	// File total = 10_000 lines; every 3rd seq is a "match".
	const density = 3
	m.fileOffsets = make([]int64, 10_000)
	m.fileTotalLines = 10_000

	// Simulate what cmdLoadFileWindowAroundBottom loads: a window sized
	// by that function around foundSeq. To mirror production we replay
	// the window bounds the call would produce, then hand the raw
	// records to applyFileWindowLoaded.
	const foundSeq = int64(600)
	// cmdLoadFileWindowAroundBottom currently reads
	// [foundSeq-vh+1 .. foundSeq+vh]; post-fix it may read more. The
	// test emulates the production fetch by calling the command and
	// reading back the bounds it expects.
	first, last := bottomWindowBounds(foundSeq, int64(vh), int64(m.fileTotalLines), m.prog)
	recs := make([]domain.Line, 0, last-first+1)
	for s := first; s <= last; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if s%density == 0 {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Line{Seq: s, Text: text})
	}

	// cmdLoadFileWindowAroundBottom pins cursor to foundSeq and sets the
	// top anchor above it in Seq space.
	m.cursorSeq = foundSeq
	top := foundSeq - int64(vh-1)
	if top < 1 {
		top = 1
	}
	m.viewTopSeq = top

	// Window load result arrives.
	m.applyFileWindowLoaded(recs, first)

	fidx := m.filteredIndices()
	row := m.cursorIdx - m.scrollTop
	if row != vh-1 {
		t.Errorf("cursor row after search nav-next disk load: got %d, want %d (bottom) — cursorIdx=%d scrollTop=%d fidxLen=%d window=[%d..%d]",
			row, vh-1, m.cursorIdx, m.scrollTop, len(fidx), first, last)
	}
	if m.buf.At(fidx[m.cursorIdx]).Seq != foundSeq {
		t.Errorf("cursor should land on foundSeq=%d, got %d", foundSeq, m.buf.At(fidx[m.cursorIdx]).Seq)
	}
}

// recordingProvider captures the most recent Fetch bounds so the test
// can assert cmdLoadFileWindowAroundBottom reads a back-skewed window
// when a filter is active. The records themselves come from a fixed
// density-1/3 pattern so the test stays deterministic.
type recordingProvider struct {
	total     int64
	lastFirst int64
	lastLast  int64
}

func (p *recordingProvider) Fetch(first, last int64) ([]domain.Line, error) {
	p.lastFirst, p.lastLast = first, last
	recs := make([]domain.Line, 0, last-first+1)
	for s := first; s <= last; s++ {
		text := "other " + strconv.FormatInt(s, 10)
		if s%3 == 0 {
			text = "match " + strconv.FormatInt(s, 10)
		}
		recs = append(recs, domain.Line{Seq: s, Text: text})
	}
	return recs, nil
}

func (p *recordingProvider) TotalLines() int64                     { return p.total }
func (p *recordingProvider) FileSize() int64                       { return p.total * 80 }
func (p *recordingProvider) EstimateBytes(first, last int64) int64 { return (last - first + 1) * 80 }

// bottomWindowBounds mirrors cmdLoadFileWindowAroundBottom's window math
// so the test exercises the same fetch size as production. Keep this
// synchronised with the implementation — if the production window size
// changes, update this helper too.
func bottomWindowBounds(foundSeq, vh, total int64, prog filter.Program) (first, last int64) {
	backward := vh
	if !prog.Empty() {
		// When a filter is active the raw-line density may be far below
		// one match per line; extending the lookback lets the filtered
		// fidx still carry (vh-1) matches above the cursor.
		backward = 10 * vh
	}
	first = foundSeq - backward + 1
	if first < 1 {
		first = 1
	}
	last = foundSeq + vh
	if last > total {
		last = total
	}
	return first, last
}

// TestLoadFileWindowAroundBottom_FilterExtendsBackward locks in the
// production window math: cmdLoadFileWindowAroundBottom must fetch a
// back-skewed window when a filter is active, otherwise the downstream
// applyFileWindowLoaded cannot place the cursor at the bottom row
// (there aren't enough filtered-in matches above the cursor in a
// raw-2*vh slice). Pair this test with the end-to-end bottom-row
// assertion above so either piece fails loudly on regression.
func TestLoadFileWindowAroundBottom_FilterExtendsBackward(t *testing.T) {
	m := newFilePartialModelForSeq(t)
	vh := m.viewportH()
	if vh < 4 {
		t.Fatalf("vh too small for scenario: %d", vh)
	}
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatal(err)
	}
	m.prog = p
	m.appliedFilter = "match"

	const total = int64(10_000)
	m.fileTotalLines = int(total)
	m.fileOffsets = make([]int64, total)
	prov := &recordingProvider{total: total}
	m.SetWindowProvider(prov)

	const foundSeq = int64(600)
	cmd := m.cmdLoadFileWindowAroundBottom(foundSeq, vh)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	_ = cmd()

	// Under filter: backward extent must be at least 3*vh raw lines so
	// the density-1/3 match density still yields >= vh-1 matches above
	// the cursor. 10*vh is the implementation's current choice.
	backwardSpan := foundSeq - prov.lastFirst + 1
	minBackward := int64(3 * vh)
	if backwardSpan < minBackward {
		t.Errorf("backward window span = %d raw lines, want >= %d (%dx vh) so filter-sparse fidx can fill (vh-1) above cursor",
			backwardSpan, minBackward, minBackward/int64(vh))
	}
	if prov.lastLast < foundSeq {
		t.Errorf("window must include foundSeq: last=%d, foundSeq=%d", prov.lastLast, foundSeq)
	}
}
