package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_spaceTogglePickOnCursor(t *testing.T) {
	// Given: list focus, three lines, cursor on first line
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"L0", "L1", "L2"})
	m.cursorIdx = 0
	// When: Space twice
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeySpace}))
	if _, ok := m.picked[0]; !ok {
		t.Fatal("expected line 0 picked")
	}
	if !strings.Contains(m.buildStatusSelText(), "picked:1") {
		t.Fatalf("status want picked:1, got %q", m.buildStatusSelText())
	}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeySpace}))
	// Then: toggled off
	if len(m.picked) != 0 {
		t.Fatalf("expected no picks, got %v", m.picked)
	}
}

func TestModel_plainDownClearsShiftRangeKeepsSpacePicks(t *testing.T) {
	// Given: Shift+down range anchor 0..1, then Space pick on line 2
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c", "d"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}))
	if m.selAnchor < 0 {
		t.Fatal("expected range anchor")
	}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeySpace}))
	if len(m.picked) != 1 {
		t.Fatalf("expected one pick, got %d", len(m.picked))
	}
	// When: plain down (clears Shift range only)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	// Then: anchor cleared, pick remains
	if m.selAnchor >= 0 {
		t.Fatal("expected Shift range cleared after plain nav")
	}
	if _, ok := m.picked[2]; !ok {
		t.Fatal("expected Space pick on index 2 preserved")
	}
}

func TestModel_spaceWithShiftRangeCommitsAllLinesToPicks(t *testing.T) {
	// Given: range 0..2 via anchor
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}))
	// When: Space commits range to picks
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeySpace}))
	// Then: three picks, no anchor
	if m.selAnchor >= 0 {
		t.Fatal("expected range cleared after commit")
	}
	if len(m.picked) != 3 {
		t.Fatalf("expected 3 picks, got %d", len(m.picked))
	}
}

func TestModel_escClearsRangeAndPicks(t *testing.T) {
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x", "y"})
	m.picked[0] = struct{}{}
	m.selAnchor = 1
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.hasListSelection() {
		t.Fatal("expected all list selection cleared")
	}
}

func TestModel_composeEscClearsPicksBeforeComposePop(t *testing.T) {
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b"})
	m.searchBuf = "x"
	m.searchCompose = true
	m.searchDraft = "edit"
	m.picked[0] = struct{}{}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if !m.searchCompose {
		t.Fatal("expected compose still on after first Esc (list selection cleared only)")
	}
	if len(m.picked) != 0 {
		t.Fatal("expected picks cleared")
	}
	if m.searchDraft != "edit" {
		t.Fatalf("draft should stay until compose pop, got %q", m.searchDraft)
	}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.searchCompose {
		t.Fatal("expected compose popped on second Esc")
	}
	if m.searchDraft != "x" {
		t.Fatalf("draft want %q, got %q", "x", m.searchDraft)
	}
}

func TestModel_cWithPicksNoRangeCopiesAllPickedLinesSorted(t *testing.T) {
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"only0", "only1", "only2"})
	m.selAnchor = -1
	m.picked[0] = struct{}{}
	m.picked[2] = struct{}{}
	m.cursorIdx = 1
	fidx := m.filteredIndices()
	text, n, ok := m.buildCopyText(fidx)
	want := "only0\nonly2"
	if !ok || n != 2 || text != want {
		t.Fatalf("got ok=%v n=%d text=%q want %q", ok, n, text, want)
	}
}
