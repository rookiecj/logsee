package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
	"github.com/charmbracelet/bubbletea"
)

func step(m *Model, msg tea.Msg) *Model {
	next, _ := m.Update(msg)
	return next.(*Model)
}

func TestModel_filterMode_esc_preservesFilterAndHighlight(t *testing.T) {
	// Given: applied filter and committed highlight search, no selection (PRD §6.6)
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"hello", "world", "hello there"})
	m.filterEdit = true
	m.filterDraft = "hello"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	if m.appliedFilter == "" {
		t.Fatal("expected applied filter")
	}
	m.searchBuf = "world"
	m.searchDraft = m.searchBuf
	// When: Esc on log list with no selection and no compose
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	// Then: both filter and highlight survive — Esc no longer clears searchBuf (PRD §6.6 step 3 removed)
	if m.appliedFilter == "" || m.prog.Empty() {
		t.Fatalf("expected filter still applied, applied=%q empty=%v", m.appliedFilter, m.prog.Empty())
	}
	if m.searchBuf != "world" || m.searchDraft != "world" {
		t.Fatalf("expected search preserved, buf=%q draft=%q", m.searchBuf, m.searchDraft)
	}
	if len(m.filteredIndices()) >= 3 {
		t.Fatalf("expected filtered list still shorter than 3, got %d", len(m.filteredIndices()))
	}
}

func TestModel_filterMode_esc_clearsSelectionOnly_preservesRest(t *testing.T) {
	// Given: applied filter, range selection, and committed highlight search
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x1", "x2", "x3"})
	m.filterEdit = true
	m.filterDraft = "x"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.selAnchor = 0
	m.cursorIdx = 1
	m.searchBuf = "x2"
	m.searchDraft = m.searchBuf
	// When: first Esc — selection drops, filter and highlight untouched
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.selAnchor >= 0 {
		t.Fatal("expected selection cleared")
	}
	if m.appliedFilter == "" {
		t.Fatal("expected filter still applied")
	}
	if m.searchBuf != "x2" {
		t.Fatalf("expected search unchanged after first esc, got %q", m.searchBuf)
	}
	// When: second Esc — now a no-op on the log list (highlight is no longer cleared by Esc)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.appliedFilter == "" {
		t.Fatal("expected filter still applied after second esc")
	}
	if m.searchBuf != "x2" {
		t.Fatalf("expected highlight preserved on second esc, got %q", m.searchBuf)
	}
}

func TestModel_ctrlF_ctrlB_pageStep(t *testing.T) {
	// Given: many lines and cursor at top
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	vh := m.viewportH()
	var lines []string
	for i := 0; i < vh+3; i++ {
		lines = append(lines, "L")
	}
	m.applyIncomingLines(lines)
	m.cursorIdx = 0
	m.scrollTop = 0
	// When: Ctrl+F (page down)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlF}))
	// Then: cursor advanced by viewport height
	if m.cursorIdx != vh {
		t.Fatalf("cursor after ctrl+f: want %d got %d", vh, m.cursorIdx)
	}
	// When: Ctrl+B (page up) once
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlB}))
	// Then: cursor moves to the top visible row first (PRD: top-anchor first)
	if m.cursorIdx != 1 {
		t.Fatalf("cursor after first ctrl+b: want 1 got %d", m.cursorIdx)
	}
	// When: Ctrl+B again
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlB}))
	// Then: already at top visible row, so page up to the previous page
	if m.cursorIdx != 0 {
		t.Fatalf("cursor after second ctrl+b: want 0 got %d", m.cursorIdx)
	}
}

func TestModel_filterEdit_navDoesNotChangeDraft(t *testing.T) {
	// Given: filter input mode with a draft
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.filterDraft = "ab"
	m.cursorIdx = 1
	// When: up arrow (browse, not draft) — plain ↓ opens filter history while in filter input (PRD §6.4.1)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyUp}))
	// Then: draft unchanged and cursor moved
	if m.filterDraft != "ab" {
		t.Fatalf("draft mutated: %q", m.filterDraft)
	}
	if m.cursorIdx != 0 {
		t.Fatalf("expected cursor idx 0, got %d", m.cursorIdx)
	}
}

func TestModel_colonFromLogList_entersFilterEdit_likeEnter(t *testing.T) {
	// Given: log list focus with data
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b"})
	m.appliedFilter = "a"
	// When: ':' (vim-style filter entry)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{':'}}))
	// Then: filter input mode, draft from applied filter
	if !m.filterEdit {
		t.Fatal("expected filter edit after :")
	}
	if m.filterDraft != "a" {
		t.Fatalf("filterDraft want applied filter, got %q", m.filterDraft)
	}
	if m.searchCompose {
		t.Fatal("expected search compose off")
	}
}

func TestModel_colonFromSearchCompose_insertsIntoDraft_doesNotOpenFilter(t *testing.T) {
	// Given: search compose open (highlight input focus)
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.searchBuf = "q"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	if !m.searchCompose {
		t.Fatal("expected compose after /")
	}
	m.searchDraft = "edited"
	m.searchCaret = len([]rune(m.searchDraft))
	// When: ':' — must stay in highlight and insert ':' (list-only shortcut opens filter)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{':'}}))
	// Then: still composing, not filter; draft includes ':'
	if !m.searchCompose {
		t.Fatal("expected compose still on")
	}
	if m.filterEdit {
		t.Fatal("expected filter edit off")
	}
	if m.searchDraft != "edited:" {
		t.Fatalf("search draft want %q got %q", "edited:", m.searchDraft)
	}
}

func TestModel_filterApply_whenNoMatchInWindow_requestsForwardScanInFilePartial(t *testing.T) {
	// Given: file-partial mode with a non-empty filter but no match in current window
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filePartial = true
	m.fileOffsets = []int64{0, 10, 20, 30}
	m.fileWinFirst = 1
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "a"},
		{Seq: 2, Text: "b"},
	})
	m.filterEdit = true
	m.filterDraft = "zzz"

	// When: apply filter with Enter
	next, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m = next.(*Model)

	// Then: no match now, so scan cmd is requested
	if len(m.filteredIndices()) != 0 {
		t.Fatalf("expected no matches in current window, got %d", len(m.filteredIndices()))
	}
	if cmd == nil {
		t.Fatal("expected follow-up scan cmd when partial-file filter has no in-window matches")
	}
}

func TestModel_filterApply_whenMatchesBelowViewport_requestsForwardScanInFilePartial(t *testing.T) {
	// Given: file-partial mode and filter matches exist but fewer than viewport rows
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filePartial = true
	m.fileOffsets = []int64{0, 10, 20, 30, 40}
	m.fileWinFirst = 1
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "match a"},
		{Seq: 2, Text: "x"},
		{Seq: 3, Text: "match b"},
	})
	m.filterEdit = true
	m.filterDraft = "match"

	// When: apply filter with Enter
	next, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m = next.(*Model)

	// Then: since filtered rows are below viewport capacity, follow-up scan cmd is requested
	if got := len(m.filteredIndices()); got == 0 {
		t.Fatal("expected at least one in-window match")
	}
	if cmd == nil {
		t.Fatal("expected follow-up scan cmd when filtered rows are fewer than viewport height")
	}
}

func TestModel_filterTopup_appendsLoadedWindowUntilViewportFilled(t *testing.T) {
	// Given: active filter topup with one match in current buffer
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 8 // viewportH = 5
	m.filePartial = true
	m.filterTopupActive = true
	p, err := filter.Parse("m")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "m1"},
		{Seq: 2, Text: "x"},
	})

	// When: next file window arrives
	next, _ := m.Update(FilterScanResultMsg{
		Records: []domain.Line{
			{Seq: 3, Text: "m2"},
			{Seq: 4, Text: "m3"},
			{Seq: 5, Text: "m4"},
			{Seq: 6, Text: "m5"},
		},
		FirstLine:  3,
		ReachedEnd: true,
	})
	m = next.(*Model)

	// Then: matches can fill viewport and topup stops
	if got := len(m.filteredIndices()); got < m.viewportH() {
		t.Fatalf("expected filtered rows >= viewportH, got %d < %d", got, m.viewportH())
	}
	if m.filterTopupActive {
		t.Fatal("expected filterTopupActive to stop when viewport is filled")
	}
}

func TestModel_filterTopup_whenBackwardDirection_prependsLoadedWindow(t *testing.T) {
	// Given: active filter topup in backward direction with current window records
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 8 // viewportH = 5
	m.filePartial = true
	m.fileWinFirst = 10
	m.filterTopupActive = true
	m.filterTopupDir = -1
	p, err := filter.Parse("m")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 10, Text: "m10"},
		{Seq: 11, Text: "m11"},
	})

	// When: previous file window arrives as filter scan result
	next, _ := m.Update(FilterScanResultMsg{
		Records: []domain.Line{
			{Seq: 6, Text: "m6"},
			{Seq: 7, Text: "m7"},
			{Seq: 8, Text: "x"},
			{Seq: 9, Text: "m9"},
		},
		FirstLine:  6,
		Direction:  -1,
		ReachedEnd: true,
	})
	m = next.(*Model)

	// Then: scanned records are prepended and backward window start is updated
	if got := m.buf.At(0).Seq; got != 6 {
		t.Fatalf("expected prepended first seq to be 6, got %d", got)
	}
	if got := m.fileWinFirst; got != 6 {
		t.Fatalf("expected fileWinFirst to move backward to 6, got %d", got)
	}
	if m.filterTopupActive {
		t.Fatal("expected filterTopupActive to stop when viewport is filled")
	}
}

func TestModel_fileIndexReady_resumesFilterTopupWhenIndexArrivesLate(t *testing.T) {
	// Given: filter topup is active but offsets are not ready yet
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 8 // viewportH = 5
	m.filePartial = true
	m.filePath = "/tmp/dummy.log"
	m.appliedFilter = "match"
	m.filterTopupActive = true
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "match a"},
		{Seq: 2, Text: "x"},
	})

	// When: index ready arrives
	next, cmd := m.Update(FileIndexReadyMsg{Offsets: []int64{0, 10, 20, 30, 40, 50}})
	m = next.(*Model)

	// Then: topup should immediately continue with a scan command
	if cmd == nil {
		t.Fatal("expected filter topup scan cmd after index becomes ready")
	}
	if !m.filterTopupActive {
		t.Fatal("expected filterTopupActive to remain on until topup finishes")
	}
}

func TestModel_windowResize_restartsFilterTopupWhenViewportGetsLarger(t *testing.T) {
	// Given: filter already applied and previously considered "filled" for a tiny viewport
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/dummy.log"
	m.fileOffsets = []int64{0, 10, 20, 30, 40, 50}
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "match a"},
		{Seq: 2, Text: "x"},
	})
	// tiny viewport first (viewportH=1) -> effectively no topup pressure
	m.width, m.height = 80, 3

	// When: window becomes larger (viewportH increases)
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	m = next.(*Model)

	// Then: topup should be (re)started
	if cmd == nil {
		t.Fatal("expected topup scan cmd after viewport grows and filtered rows become insufficient")
	}
	if !m.filterTopupActive {
		t.Fatal("expected filterTopupActive true after resize-triggered topup")
	}
}

func TestModel_fileWindowLoaded_restartsTopupWhenFilteredRowsAreBelowViewport(t *testing.T) {
	// Given: partial-file mode with applied filter and topup currently inactive
	r := buffer.NewRing(200)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 8 // viewportH = 5
	m.filePartial = true
	m.filePath = "/tmp/dummy.log"
	m.fileOffsets = []int64{0, 10, 20, 30, 40, 50, 60, 70}
	m.appliedFilter = "match"
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.filterTopupActive = false

	// When: a new window is loaded (e.g., after page down) with too few matches
	next, cmd := m.Update(FileWindowLoadedMsg{
		Records: []domain.Line{
			{Seq: 21, Text: "match a"},
			{Seq: 22, Text: "x"},
			{Seq: 23, Text: "match b"},
		},
		FirstLine: 21,
	})
	m = next.(*Model)

	// Then: topup is restarted automatically to fill viewport
	if got := len(m.filteredIndices()); got >= m.viewportH() {
		t.Fatalf("test setup invalid: expected filtered rows below viewport, got %d", got)
	}
	if cmd == nil {
		t.Fatal("expected topup scan cmd after window load when filtered rows are insufficient")
	}
	if !m.filterTopupActive {
		t.Fatal("expected filterTopupActive to be turned on")
	}
}

func TestModel_filePartialBootstrap_restartsTopupWhenFilteredRowsAreBelowViewport(t *testing.T) {
	// Given: startup bootstrap in partial-file mode with applied filter and too few in-window matches
	r := buffer.NewRing(200)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 8 // viewportH = 5
	m.filePartial = true
	m.filePath = "/tmp/dummy.log"
	m.fileOffsets = []int64{0, 10, 20, 30, 40, 50}
	m.appliedFilter = "match"
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.filterTopupActive = false

	// When: initial bootstrap lines arrive
	next, cmd := m.Update(FilePartialBootstrapMsg{
		Path:  "/tmp/dummy.log",
		Lines: []string{"match a", "x"},
	})
	m = next.(*Model)

	// Then: topup should start immediately from bootstrap path
	if got := len(m.filteredIndices()); got >= m.viewportH() {
		t.Fatalf("test setup invalid: expected filtered rows below viewport, got %d", got)
	}
	if cmd == nil {
		t.Fatal("expected topup scan cmd after bootstrap when filtered rows are insufficient")
	}
	if !m.filterTopupActive {
		t.Fatal("expected filterTopupActive to be turned on")
	}
}

func TestModel_pickFilterTopupDirWhenUndersized_atEOF_returnsBackward(t *testing.T) {
	// Given: buffered window ends at physical EOF but not at line 1 (End/G / tail window)
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.filePartial = true
	m.fileTotalLines = 100
	m.fileWinFirst = 70
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 99, Text: "x"},
		{Seq: 100, Text: "match"},
	})
	// When:
	dir := m.pickFilterTopupDirWhenUndersized()
	// Then:
	if dir != -1 {
		t.Fatalf("Then: want backward (-1), got %d", dir)
	}
}

func TestModel_pickFilterTopupDirWhenUndersized_notAtEOF_returnsForward(t *testing.T) {
	// Given: window does not include the last physical line yet
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.filePartial = true
	m.fileTotalLines = 100
	m.fileWinFirst = 1
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "match"},
		{Seq: 2, Text: "x"},
	})
	// When:
	dir := m.pickFilterTopupDirWhenUndersized()
	// Then:
	if dir != +1 {
		t.Fatalf("Then: want forward (+1), got %d", dir)
	}
}

func TestModel_FilterScanResultMsg_forwardEOFWithoutRecords_schedulesBackwardTopup(t *testing.T) {
	// Given: forward filter scan hit EOF with no appended chunk; viewport still needs more filtered rows
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.width, m.height = 80, 8
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileOffsets = make([]int64, 100)
	m.fileTotalLines = 100
	m.fileWinFirst = 80
	p, err := filter.Parse("m")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "m"
	m.filterTopupActive = true
	m.filterTopupDir = +1
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 99, Text: "x"},
		{Seq: 100, Text: "match"},
	})

	// When:
	next, cmd := m.Update(FilterScanResultMsg{Direction: +1, ReachedEnd: true})
	m2 := next.(*Model)

	// Then: schedule backward scan instead of stopping topup at EOF
	if cmd == nil {
		t.Fatal("Then: expected backward filter topup cmd")
	}
	if m2.filterTopupDir != -1 {
		t.Fatalf("Then: want filterTopupDir -1, got %d", m2.filterTopupDir)
	}
}
