package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_ctrlNWithoutCommittedSearch_doesNotMutateSearchDraft(t *testing.T) {
	// Given: list focus, no applied search, draft empty
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a"})
	m.searchBuf = ""
	m.searchDraft = ""
	// When: Ctrl+n (consumed by tryBrowseKey, no navigation)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlN}))
	// Then: draft still empty (not "n" from accidental fallthrough)
	if m.searchDraft != "" {
		t.Fatalf("searchDraft want empty, got %q", m.searchDraft)
	}
}

func TestModel_keyNWithCommittedSearch_movesCursorLikeCtrlN(t *testing.T) {
	// Given: same as Ctrl+n navigation test
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x", "ok", "x2"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "x"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 0
	// When: n (vim-style; remapped to ctrl+n on log list only)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	// Then:
	if m.cursorIdx != 2 {
		t.Fatalf("want cursor on second x line (fidx 2), got %d", m.cursorIdx)
	}
}

func TestModel_ctrlNWithCommittedSearch_movesCursor(t *testing.T) {
	// Given: two matching lines and committed query
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x", "ok", "x2"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "x"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 0
	// When: Ctrl+n
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlN}))
	// Then:
	if m.cursorIdx != 2 {
		t.Fatalf("want cursor on second x line (fidx 2), got %d", m.cursorIdx)
	}
}

func TestModel_ctrlPWithCommittedSearch_movesCursorPrev(t *testing.T) {
	// Given: two "x" lines, committed search, cursor on second match
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x", "ok", "x2"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "x"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 2
	// When: Ctrl+p (previous match toward list head, no wrap)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlP}))
	// Then: cursor on first x line
	if m.cursorIdx != 0 {
		t.Fatalf("want cursor fidx 0, got %d", m.cursorIdx)
	}
}

func TestModel_ctrlNAtLastMatch_doesNotWrap(t *testing.T) {
	// Given: two matches, cursor already on last match
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x", "ok", "x2"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "x"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 2
	// When: Ctrl+n (no later match)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlN}))
	// Then:
	if m.cursorIdx != 2 {
		t.Fatalf("want cursor unchanged at fidx 2, got %d", m.cursorIdx)
	}
}

func TestModel_ctrlN_multiToken_OR_movesToFirstLineMatchingAnyToken(t *testing.T) {
	// Given: two lines; first has no token; second matches second token only
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"no hit here", "gamma ray"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = `alpha "gamma ray"`
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 0
	// When: Ctrl+n
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlN}))
	// Then: cursor moves to line matching quoted phrase
	if m.cursorIdx != 1 {
		t.Fatalf("want cursor fidx 1, got %d", m.cursorIdx)
	}
}

func TestModel_ctrlPAtFirstMatch_doesNotWrap(t *testing.T) {
	// Given: two matches, cursor on first match
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x", "ok", "x2"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "x"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 0
	// When: Ctrl+p (no earlier match)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlP}))
	// Then:
	if m.cursorIdx != 0 {
		t.Fatalf("want cursor unchanged at fidx 0, got %d", m.cursorIdx)
	}
}

func TestModel_searchScanResult_overBudget_opensConfirmModal(t *testing.T) {
	// Given: model with partial-file mode
	r := buffer.NewRing(20)
	var tm tea.Model = NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m := tm.(*Model)
	m.width, m.height = 80, 24
	m.filePartial = true
	m.fileOffsets = []int64{0, 10, 20}

	// When: lazy scan reports over-budget confirmation required
	tm, _ = tm.Update(SearchScanResultMsg{
		NeedConfirm:  true,
		ResumeSeq:    2,
		ScannedBytes: 100 << 20,
		Direction:    +1,
	})
	m = tm.(*Model)

	// Then: confirm modal state is armed
	if !m.searchScanConfirmOpen {
		t.Fatal("expected search confirm modal open")
	}
	if m.searchScanResumeSeq != 2 || m.searchScanDir != +1 {
		t.Fatalf("unexpected resume state: seq=%d dir=%d", m.searchScanResumeSeq, m.searchScanDir)
	}
}

func TestModel_ctrlN_filePartial_usesInWindowMatchBeforeLazyLoad(t *testing.T) {
	// Given: partial-file mode and a next match already inside current loaded window
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filePartial = true
	m.fileOffsets = []int64{0, 10, 20, 30}
	m.buf.ReplaceRecords([]domain.Record{
		{Seq: 1, Text: "x one"},
		{Seq: 2, Text: "middle"},
		{Seq: 3, Text: "x two"},
	})
	m.searchBuf = "x"
	m.cursorIdx = 0
	fidx := m.filteredIndices()

	// When: Ctrl+n
	ok, cmd := m.tryBrowseKey(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlN}), fidx, m.viewportH())

	// Then: move in-window only; no lazy-load command should be issued
	if !ok {
		t.Fatal("expected ctrl+n handled")
	}
	if m.cursorIdx != 2 {
		t.Fatalf("cursor want 2 got %d", m.cursorIdx)
	}
	if cmd != nil {
		t.Fatal("did not expect lazy-load cmd when in-window match exists")
	}
}
