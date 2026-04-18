package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_buildCopyText_noSelection_copiesCursorLine(t *testing.T) {
	// Given: three lines, cursor on middle, no range selection
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"alpha", "beta", "gamma"})
	m.selAnchor = -1
	m.cursorIdx = 1
	fidx := m.filteredIndices()
	// When:
	text, n, ok := m.buildCopyText(fidx)
	// Then:
	if !ok || n != 1 || text != "beta" {
		t.Fatalf("got ok=%v n=%d text=%q", ok, n, text)
	}
}

func TestModel_buildCopyText_selection_copiesAllLinesInRange(t *testing.T) {
	// Given: anchor..cursor spans three filtered lines
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"L0", "L1", "L2"})
	m.selAnchor = 0
	m.cursorIdx = 2
	fidx := m.filteredIndices()
	// When:
	text, n, ok := m.buildCopyText(fidx)
	// Then:
	if !ok || n != 3 || text != "L0\nL1\nL2" {
		t.Fatalf("got ok=%v n=%d text=%q", ok, n, text)
	}
}

func TestModel_buildCopyText_rangeAndPicks_unionSorted(t *testing.T) {
	// Given: Shift range 1..2 and a Space pick at 0 (outside the range)
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"L0", "L1", "L2", "L3"})
	m.picked[0] = struct{}{}
	m.selAnchor = 1
	m.cursorIdx = 2
	fidx := m.filteredIndices()
	// When:
	text, n, ok := m.buildCopyText(fidx)
	// Then: filtered-index order, three lines
	if !ok || n != 3 || text != "L0\nL1\nL2" {
		t.Fatalf("got ok=%v n=%d text=%q", ok, n, text)
	}
}

func TestModel_buildCopyText_rangeAndPicks_overlapDedupes(t *testing.T) {
	// Given: Shift range 0..1 and Space pick on line 1 (inside range)
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"L0", "L1", "L2"})
	m.picked[1] = struct{}{}
	m.selAnchor = 0
	m.cursorIdx = 1
	fidx := m.filteredIndices()
	// When:
	text, n, ok := m.buildCopyText(fidx)
	// Then: two lines, no duplicate
	if !ok || n != 2 || text != "L0\nL1" {
		t.Fatalf("got ok=%v n=%d text=%q", ok, n, text)
	}
}

func TestModel_buildCopyText_emptyList(t *testing.T) {
	// Given: no lines
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	// When:
	_, _, ok := m.buildCopyText(m.filteredIndices())
	// Then:
	if ok {
		t.Fatal("expected ok=false for empty list")
	}
}

func TestModel_filterEdit_c_appendsToDraft(t *testing.T) {
	// Given: filter input focus and a draft
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"only"})
	m.filterEdit = true
	m.filterDraft = "keep"
	m.syncFilterCursorEnd()
	// When: plain c (not list copy)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'c'}}))
	// Then:
	if m.filterDraft != "keepc" {
		t.Fatalf("filterDraft want %q, got %q", "keepc", m.filterDraft)
	}
}

func TestModel_listFocus_c_withoutRange_copiesCursorLine(t *testing.T) {
	// Given: list focus, lines, no Shift range (PRD §8.6: copy cursor line)
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.selAnchor = -1
	m.cursorIdx = 1
	// When:
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'c'}}))
	// Then:
	if m.copyFlash != "1 line copied" {
		t.Fatalf("copyFlash want %q, got %q", "1 line copied", m.copyFlash)
	}
}

func TestModel_listFocus_c_withSelection_setsCopyFeedback(t *testing.T) {
	// Given: list focus and a Shift+down range (anchor 0, cursor 1)
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"L0", "L1", "L2"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}))
	if m.selAnchor < 0 {
		t.Fatal("expected selection anchor after shift+down")
	}
	// When:
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'c'}}))
	// Then: clipboard success or host error — either way user-visible feedback is set
	if m.copyFlash == "" {
		t.Fatal("expected non-empty copy feedback")
	}
	if strings.Contains(m.copyFlash, "copied") && m.copyFlash != "2 lines copied" && m.copyFlash != "1 line copied" {
		t.Fatalf("unexpected success message: %q", m.copyFlash)
	}
}
