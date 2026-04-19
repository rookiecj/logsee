package ui

import (
	"os"
	"strings"

	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"

	"github.com/charmbracelet/bubbletea"
)

// FilePartialBootstrapMsg seeds the first on-disk window when opening a log file (partial loading).
type FilePartialBootstrapMsg struct {
	Path  string
	Lines []string
	Err   error
}

// FileIndexReadyMsg delivers the full-file line-start byte offset table (enables random window reads).
type FileIndexReadyMsg struct {
	Offsets []int64
	Err     error
}

// FileWindowLoadedMsg replaces the in-memory window after an async read from disk.
type FileWindowLoadedMsg struct {
	Records   []domain.Line
	FirstLine int64
	Err       error
}

type SearchScanResultMsg struct {
	FoundSeq     int64
	NeedConfirm  bool
	ResumeSeq    int64
	ScannedBytes int64
	Direction    int
	ReachedEnd   bool
	Err          error
}

type FilterScanResultMsg struct {
	Records    []domain.Line
	FirstLine  int64
	Direction  int
	ReachedEnd bool
	Err        error
}

func (m *Model) applyFilePartialBootstrap(path string, lines []string) {
	m.filePartial = true
	m.filePath = path
	m.follow = false
	m.cursorIdx = 0
	m.scrollTop = 0
	m.scrollSegTop = 0
	m.colRuneOff = 0
	recs := make([]domain.Line, len(lines))
	for i, t := range lines {
		recs[i] = domain.Line{Seq: int64(i + 1), Text: t}
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 1
	// Phase 1 seq coord: anchor cursor to first line; viewTopSeq = 1 places top of window at file start.
	if len(recs) > 0 {
		m.cursorSeq = 1
		m.viewTopSeq = 1
	} else {
		m.cursorSeq = 0
		m.viewTopSeq = 0
	}
}

func (m *Model) applyFileIndexReady(offsets []int64) {
	m.fileOffsets = offsets
	m.fileTotalLines = len(offsets)
	if st, err := os.Stat(m.filePath); err == nil {
		m.fileSizeBytes = st.Size()
	}
	// Phase 2: once offsets + size are known, hand ownership of random-access reads to a
	// WindowProvider. All disk-fetching commands route through this interface from now on.
	m.windowProvider = NewFileSliceProvider(m.filePath, offsets, m.fileSizeBytes)
	m.stdinClosed = true
	m.maybeResolveAutoFormat()
}

// applyFileWindowLoaded replaces the ring with a fresh window and rebuilds the viewport state
// from the seq anchors (cursorSeq, viewTopSeq). These anchors are the authoritative view state
// in file partial mode (docs/plans/seq-coord-pull-window-plan.md) — so ring replacement cannot
// perturb the cursor's screen row.
//
// Phase 4: pendingFocusSeq / pendingFocusPreferBottom are gone. The cmd functions now set
// cursorSeq / viewTopSeq directly, and this function infers the "prefer bottom row" intent from
// viewTopSeq < cursorSeq (the signature of a bottom-pin load like End / PageDown / AroundBottom).
func (m *Model) applyFileWindowLoaded(recs []domain.Line, firstLine int64) {
	m.fileWinFirst = firstLine
	m.buf.ReplaceRecords(recs)
	fidx := m.filteredIndices()

	if len(fidx) == 0 {
		m.cursorIdx = 0
		m.scrollTop = 0
		m.scrollSegTop = 0
		m.cursorSeq = 0
		m.viewTopSeq = 0
		m.follow = false
		m.clampSelectionToBuffer(fidx)
		return
	}

	// Infer bottom-row pin intent: a bottom-pinned cmd sets viewTopSeq = cursorSeq - (vh-1) < cursorSeq.
	// Top pins (most paths) set viewTopSeq == cursorSeq.
	preferBottom := m.cursorSeq > 0 && m.viewTopSeq > 0 && m.viewTopSeq < m.cursorSeq
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	fallbackRow := 0
	if preferBottom {
		fallbackRow = vh - 1
	}

	// Cursor fallback when cursorSeq is missing from the new fidx (filter drops it, load reached
	// a different slice, etc.): preferBottom → last row; else top row. Set cursorIdx explicitly so
	// syncIdxFromSeq's clampCursor branch preserves it.
	if m.findSeqInFidx(fidx, m.cursorSeq) < 0 {
		if preferBottom {
			m.cursorIdx = len(fidx) - 1
		} else {
			m.cursorIdx = 0
		}
	}

	m.syncIdxFromSeq(fidx, fallbackRow)
	m.clampSelectionToBuffer(fidx)

	if m.lineWrap && m.cursorIdx == len(fidx)-1 {
		m.pinWrapSegScrollToLogicalTail(fidx)
	}
	m.follow = m.cursorIdx == len(fidx)-1 && m.tailAligned(fidx)
	// Keep seq anchors in sync with the idx state we just derived (covers wrap-segment fixups above).
	m.syncSeqFromIdx(fidx)
}

// statusLineTotal returns the "lines:N" status strip value (total line count).
//
// Resolution order (docs/plans/stdin-fileprovider-unify-plan.md, Phase 2):
//  1. File-partial mode with a known line count — authoritative, comes from [applyFileIndexReady].
//  2. A [WindowProvider] advertising a positive TotalLines — covers stdin's [RingStreamProvider],
//     whose counter reflects cumulative receives (monotonic across ring evictions).
//  3. Ring size — fallback for tests that neither seed fileTotalLines nor install a provider.
func (m *Model) statusLineTotal() int {
	if m.filePartial && m.fileTotalLines > 0 {
		return m.fileTotalLines
	}
	if m.windowProvider != nil {
		if tot := m.windowProvider.TotalLines(); tot > 0 {
			return int(tot)
		}
	}
	if m.buf == nil {
		return 0
	}
	return m.buf.Len()
}

// cmdExpandFileWindowIfUndersized reloads when the buffered window is smaller than the canonical
// partial-load size min(2*viewportH, fileTotalLines) while the file still has unloaded lines.
// Typical case: End/G before WindowSizeMsg used vh=1 so only ~2 lines were read; after resize the
// log area would otherwise stay mostly blank (renderLog top-padding) until the user navigates.
func (m *Model) cmdExpandFileWindowIfUndersized() tea.Cmd {
	if !m.filePartial || len(m.fileOffsets) == 0 || m.fileTotalLines <= 0 || m.buf == nil {
		return nil
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	want := 2 * vh
	if want > m.fileTotalLines {
		want = m.fileTotalLines
	}
	if m.buf.Len() >= want {
		return nil
	}
	if m.fileTotalLines <= m.buf.Len() {
		return nil
	}
	fidx := m.filteredIndices()
	if len(fidx) == 0 {
		return nil
	}
	m.clampCursor(fidx)
	seq := m.buf.At(fidx[m.cursorIdx]).Seq
	if seq == int64(m.fileTotalLines) {
		return m.cmdLoadFileWindowAroundBottom(seq, vh)
	}
	return m.cmdLoadFileWindowAround(seq)
}

func (m *Model) cmdLoadFileWindowAround(globalLine int64) tea.Cmd {
	prov := m.windowProviderOrFallback()
	if !m.filePartial || prov == nil || prov.TotalLines() == 0 || globalLine < 1 {
		return nil
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	win := 2 * vh
	first := globalLine - int64(vh)
	if first < 1 {
		first = 1
	}
	last := first + int64(win) - 1
	tot := prov.TotalLines()
	if last > tot {
		last = tot
		first = last - int64(win) + 1
		if first < 1 {
			first = 1
		}
	}
	fl := first
	ll := last
	// Phase 4b: seq anchors encode the caller's target. cursorSeq = intended cursor landing.
	// viewTopSeq is preserved (already set by the caller — nav handlers, bookmark jump, expand).
	m.cursorSeq = globalLine
	return func() tea.Msg {
		recs, err := prov.Fetch(fl, ll)
		if err != nil {
			return FileWindowLoadedMsg{Err: err}
		}
		return FileWindowLoadedMsg{Records: recs, FirstLine: fl}
	}
}

func (m *Model) cmdLoadFileWindowStartingAt(first int64) tea.Cmd {
	prov := m.windowProviderOrFallback()
	if !m.filePartial || prov == nil || prov.TotalLines() == 0 || first < 1 {
		return nil
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	win := 2 * vh
	last := first + int64(win) - 1
	tot := prov.TotalLines()
	if last > tot {
		last = tot
	}
	fl := first
	ll := last
	// Cursor lands on the first loaded seq (top row).
	m.cursorSeq = first
	m.viewTopSeq = first
	return func() tea.Msg {
		recs, err := prov.Fetch(fl, ll)
		if err != nil {
			return FileWindowLoadedMsg{Err: err}
		}
		return FileWindowLoadedMsg{Records: recs, FirstLine: fl}
	}
}

func (m *Model) cmdLoadFileWindowAroundTop(globalLine int64, vh int) tea.Cmd {
	prov := m.windowProviderOrFallback()
	if !m.filePartial || prov == nil || prov.TotalLines() == 0 || globalLine < 1 {
		return nil
	}
	if vh < 1 {
		vh = 1
	}
	win := 2 * vh
	first := globalLine
	last := first + int64(win) - 1
	tot := prov.TotalLines()
	if last > tot {
		last = tot
		first = last - int64(win) + 1
		if first < 1 {
			first = 1
		}
	}
	fl := first
	ll := last
	// Top pin: cursor at globalLine, viewport top at the same seq.
	m.cursorSeq = globalLine
	m.viewTopSeq = globalLine
	return func() tea.Msg {
		recs, err := prov.Fetch(fl, ll)
		if err != nil {
			return FileWindowLoadedMsg{Err: err}
		}
		return FileWindowLoadedMsg{Records: recs, FirstLine: fl}
	}
}

func (m *Model) maybeFileLoadAfterNavDown(fidx []int, prevCursor int) tea.Cmd {
	if !m.filePartial || len(m.fileOffsets) == 0 || len(fidx) == 0 {
		return nil
	}
	if m.cursorIdx != prevCursor {
		return nil
	}
	if prevCursor != len(fidx)-1 {
		return nil
	}
	G := m.buf.At(fidx[prevCursor]).Seq
	if G >= int64(m.fileTotalLines) {
		return nil
	}
	// Filter active: j at the last match means "jump to next filter match on disk".
	// Delegate to filter-scan; FilterScanResultMsg handler advances cursor using filterTopupNavAdvance.
	// cmdLoadFileWindowAround(G+1) would not work here: G+1 is usually not a match, so after load
	// cursorSeq=G+1 falls out of the new fidx and cursor jumps to the top of the visible matches.
	if !m.prog.Empty() {
		m.filterTopupActive = true
		m.filterTopupDir = +1
		m.filterTopupNavAdvance = +1
		return m.cmdFindFilterMatchForwardFromWindowEnd()
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	// Advance seq anchors so cursor lands at bottom row of the reloaded window
	// (applyFileWindowLoaded consumes cursorSeq/viewTopSeq to rebuild cursorIdx/scrollTop).
	m.cursorSeq = G + 1
	top := G + 1 - int64(vh-1)
	if top < 1 {
		top = 1
	}
	m.viewTopSeq = top
	return m.cmdLoadFileWindowAround(G + 1)
}

func (m *Model) maybeFileLoadAfterNavUp(fidx []int, prevCursor int) tea.Cmd {
	if !m.filePartial || len(m.fileOffsets) == 0 || len(fidx) == 0 {
		return nil
	}
	if m.cursorIdx != prevCursor {
		return nil
	}
	if prevCursor != 0 {
		return nil
	}
	G := m.buf.At(fidx[prevCursor]).Seq
	if G <= 1 {
		return nil
	}
	// Filter active: k at the first match → scan backward for the previous match.
	if !m.prog.Empty() {
		m.filterTopupActive = true
		m.filterTopupDir = -1
		m.filterTopupNavAdvance = -1
		return m.cmdFindFilterMatchBackwardFromWindowStart()
	}
	// Anchor cursor to G-1 and pin view top to the same seq so cursor lands on row 0.
	m.cursorSeq = G - 1
	m.viewTopSeq = G - 1
	return m.cmdLoadFileWindowAround(G - 1)
}

func (m *Model) maybeFileLoadAfterPageUp(fidx []int, vh int) tea.Cmd {
	if !m.filePartial || len(m.fileOffsets) == 0 {
		return nil
	}
	if len(fidx) == 0 {
		return nil
	}
	if m.cursorIdx != 0 {
		return nil
	}
	if m.fileWinFirst <= 1 {
		return nil
	}
	// Filter mode: PageUp's intent is "vh matches up". Raw window load would land cursorSeq on a
	// non-matching seq and the sparse fallback row ends up mid-viewport (padTop pushes cursor down).
	// Delegate to backward filter-scan; result handler consumes filterTopupNavAdvance steps.
	if !m.prog.Empty() {
		m.filterTopupActive = true
		m.filterTopupDir = -1
		m.filterTopupNavAdvance = -vh
		return m.cmdFindFilterMatchBackwardFromWindowStart()
	}
	first := m.fileWinFirst - int64(vh)
	if first < 1 {
		first = 1
	}
	m.filterTopupDir = -1
	return m.cmdLoadFileWindowStartingAt(first)
}

func (m *Model) maybeFileLoadAfterPageDown(fidx []int, prevCursor int, vh int) tea.Cmd {
	if !m.filePartial || len(m.fileOffsets) == 0 || len(fidx) == 0 {
		return nil
	}
	if m.cursorIdx != prevCursor {
		return nil
	}
	if prevCursor != len(fidx)-1 {
		return nil
	}
	G := m.buf.At(fidx[prevCursor]).Seq
	tgt := G + int64(vh)
	if tgt > int64(m.fileTotalLines) {
		tgt = int64(m.fileTotalLines)
	}
	if tgt <= G {
		return nil
	}
	// Filter mode: advance cursor vh matches forward via disk scan (see PageUp note).
	if !m.prog.Empty() {
		m.filterTopupActive = true
		m.filterTopupDir = +1
		m.filterTopupNavAdvance = vh
		return m.cmdFindFilterMatchForwardFromWindowEnd()
	}
	m.filterTopupDir = +1
	return m.cmdLoadFileWindowAroundBottom(tgt, vh)
}

func (m *Model) cmdBookmarkJumpToSeq(seq int64) tea.Cmd {
	if !m.filePartial || len(m.fileOffsets) == 0 {
		return nil
	}
	if seq < 1 || seq > int64(len(m.fileOffsets)) {
		return nil
	}
	// Bookmark: top pin at the bookmarked seq.
	m.cursorSeq = seq
	m.viewTopSeq = seq
	return m.cmdLoadFileWindowAround(seq)
}

func (m *Model) cmdStartLazySearch(dir int, fidx []int) tea.Cmd {
	if len(fidx) == 0 {
		return nil
	}
	m.clampCursor(fidx)
	curSeq := m.buf.At(fidx[m.cursorIdx]).Seq
	start := curSeq + int64(dir)
	if dir < 0 {
		start = curSeq - 1
	}
	return m.cmdScanSearchInFile(dir, start, 0, false)
}

func (m *Model) cmdScanSearchInFile(dir int, startSeq int64, scanned int64, confirmed bool) tea.Cmd {
	prov := m.windowProviderOrFallback()
	if prov == nil || prov.TotalLines() == 0 || strings.TrimSpace(m.searchBuf) == "" {
		return nil
	}
	// Must be a disk-capable provider: filePartial with offsets, or stdin with --out fallback.
	if !m.canLazySearch() {
		return nil
	}
	total := prov.TotalLines()
	query := m.searchBuf
	prog := m.prog
	ignoreCase := m.ignoreCase
	logFmt := m.effectiveLogFormat()
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	win := int64(2 * vh)
	const limitBytes = int64(100 << 20)
	hlNames := m.highlightNames
	return func() tea.Msg {
		seq := startSeq
		acc := scanned
		for {
			var first, last int64
			if dir > 0 {
				if seq > total {
					return SearchScanResultMsg{Direction: dir, ReachedEnd: true}
				}
				first = seq
				last = first + win - 1
				if last > total {
					last = total
				}
				seq = last + 1
			} else {
				if seq < 1 {
					return SearchScanResultMsg{Direction: dir, ReachedEnd: true}
				}
				last = seq
				first = last - win + 1
				if first < 1 {
					first = 1
				}
				seq = first - 1
			}
			chunkBytes := prov.EstimateBytes(first, last)
			if !confirmed && acc+chunkBytes > limitBytes {
				resume := first
				if dir < 0 {
					resume = last
				}
				return SearchScanResultMsg{
					NeedConfirm:  true,
					ResumeSeq:    resume,
					ScannedBytes: acc,
					Direction:    dir,
				}
			}
			recs, err := prov.Fetch(first, last)
			if err != nil {
				return SearchScanResultMsg{Err: err}
			}
			acc += chunkBytes
			if dir < 0 {
				for i := len(recs) - 1; i >= 0; i-- {
					if filter.Match(recs[i].Text, prog, ignoreCase, logFmt) && SearchMatchesLineWithNames(recs[i].Text, query, false, hlNames) {
						return SearchScanResultMsg{FoundSeq: recs[i].Seq, ScannedBytes: acc, Direction: dir}
					}
				}
			} else {
				for i := 0; i < len(recs); i++ {
					if filter.Match(recs[i].Text, prog, ignoreCase, logFmt) && SearchMatchesLineWithNames(recs[i].Text, query, false, hlNames) {
						return SearchScanResultMsg{FoundSeq: recs[i].Seq, ScannedBytes: acc, Direction: dir}
					}
				}
			}
			if (dir > 0 && last >= total) || (dir < 0 && first <= 1) {
				return SearchScanResultMsg{Direction: dir, ReachedEnd: true, ScannedBytes: acc}
			}
		}
	}
}

func (m *Model) cmdFindFilterMatchForwardFromWindowEnd() tea.Cmd {
	prov := m.windowProviderOrFallback()
	if !m.canLazySearch() || prov == nil || prov.TotalLines() == 0 || m.prog.Empty() {
		return nil
	}
	total := prov.TotalLines()
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	win := int64(2 * vh)
	start := m.fileWinFirst + int64(m.buf.Len())
	if start < 1 {
		start = 1
	}
	return func() tea.Msg {
		if start > total {
			return FilterScanResultMsg{Direction: +1, ReachedEnd: true}
		}
		first := start
		last := first + win - 1
		if last > total {
			last = total
		}
		recs, err := prov.Fetch(first, last)
		if err != nil {
			return FilterScanResultMsg{Direction: +1, Err: err}
		}
		return FilterScanResultMsg{Records: recs, FirstLine: first, Direction: +1, ReachedEnd: last >= total}
	}
}

func (m *Model) cmdFindFilterMatchBackwardFromWindowStart() tea.Cmd {
	prov := m.windowProviderOrFallback()
	if !m.canLazySearch() || prov == nil || prov.TotalLines() == 0 || m.prog.Empty() {
		return nil
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	win := int64(2 * vh)
	end := m.fileWinFirst - 1
	if end < 1 {
		return nil
	}
	return func() tea.Msg {
		last := end
		first := last - win + 1
		if first < 1 {
			first = 1
		}
		recs, err := prov.Fetch(first, last)
		if err != nil {
			return FilterScanResultMsg{Direction: -1, Err: err}
		}
		return FilterScanResultMsg{Records: recs, FirstLine: first, Direction: -1, ReachedEnd: first <= 1}
	}
}

func (m *Model) cmdLoadFileWindowAroundBottom(globalLine int64, vh int) tea.Cmd {
	prov := m.windowProviderOrFallback()
	if !m.filePartial || prov == nil || prov.TotalLines() == 0 || globalLine < 1 {
		return nil
	}
	if vh < 1 {
		vh = 1
	}
	win := 2 * vh
	first := globalLine - int64(vh) + 1
	if first < 1 {
		first = 1
	}
	last := first + int64(win) - 1
	tot := prov.TotalLines()
	if last > tot {
		last = tot
		first = last - int64(win) + 1
		if first < 1 {
			first = 1
		}
	}
	fl := first
	ll := last
	// Bottom pin: cursor at globalLine, viewport top at globalLine-(vh-1). applyFileWindowLoaded
	// infers preferBottom from viewTopSeq < cursorSeq.
	m.cursorSeq = globalLine
	top := globalLine - int64(vh-1)
	if top < 1 {
		top = 1
	}
	m.viewTopSeq = top
	return func() tea.Msg {
		recs, err := prov.Fetch(fl, ll)
		if err != nil {
			return FileWindowLoadedMsg{Err: err}
		}
		return FileWindowLoadedMsg{Records: recs, FirstLine: fl}
	}
}
