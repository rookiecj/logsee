package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestModel_filterEdit_leftRight_movesCaret_insertUsesCaret(t *testing.T) {
	// Given: filter edit with draft and caret at end
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a"})
	m.filterEdit = true
	m.filterDraft = "abcd"
	m.syncFilterCursorEnd()
	// When: move caret left twice then type X
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'X'}}))
	// Then: insertion is before former caret position
	if m.filterDraft != "abXcd" {
		t.Fatalf("filterDraft want %q got %q", "abXcd", m.filterDraft)
	}
	wantCursor := len([]rune("abX")) // after inserted X
	if m.filterCursor != wantCursor {
		t.Fatalf("filterCursor want %d got %d", wantCursor, m.filterCursor)
	}
}

func TestModel_filterEdit_left_doesNotScrollLogHorizontally(t *testing.T) {
	// Given: wrap off, horizontal offset set, filter input focus
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.lineWrap = false
	long := strings.Repeat("M", 120)
	m.applyIncomingLines([]string{long})
	m.colRuneOff = 5
	m.filterEdit = true
	m.filterDraft = "ab"
	m.syncFilterCursorEnd()
	// When: Left adjusts filter caret only
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	// Then: log horizontal scroll unchanged; filter caret moved
	if m.colRuneOff != 5 {
		t.Fatalf("colRuneOff want 5 got %d", m.colRuneOff)
	}
	if m.filterCursor != 1 {
		t.Fatalf("filterCursor want 1 got %d", m.filterCursor)
	}
}

func TestModel_filterEdit_View_rowWidthMatchesTerminal(t *testing.T) {
	// Given: filter edit and fixed width
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 70, 8
	m.applyIncomingLines([]string{"z"})
	m.filterEdit = true
	m.filterDraft = "+z"
	m.syncFilterCursorEnd()
	// When:
	out := m.View()
	lines := strings.Split(out, "\n")
	// Then: first row display width matches m.width (lipgloss)
	if len(lines) < 1 {
		t.Fatal("expected at least one line")
	}
	if got := lipgloss.Width(lines[0]); got != m.width {
		t.Fatalf("filter row width want %d got %d", m.width, got)
	}
}
