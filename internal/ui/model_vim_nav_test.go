package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestLogList_vimKey_j_movesDownLikeArrow(t *testing.T) {
	// Given: log list, two lines, cursor on first
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"a", "b"})
	m.cursorIdx = 0
	m.follow = false
	// When: j
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m2 := next.(*Model)
	// Then: cursor moves down like ↓
	if m2.cursorIdx != 1 {
		t.Fatalf("Then: want cursorIdx 1, got %d", m2.cursorIdx)
	}
}

func TestLogList_vimKey_k_movesUpLikeArrow(t *testing.T) {
	// Given: two lines, cursor on second
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"a", "b"})
	m.cursorIdx = 1
	m.follow = false
	// When: k
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m2 := next.(*Model)
	// Then:
	if m2.cursorIdx != 0 {
		t.Fatalf("Then: want cursorIdx 0, got %d", m2.cursorIdx)
	}
}

func TestFilterEditor_vimKey_j_insertsNotNav(t *testing.T) {
	// Given: filter compose active — j must not be remapped to ↓
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"x"})
	m.filterEdit = true
	m.filterDraft = ""
	// When: j
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m2 := next.(*Model)
	// Then: draft receives j; cursor stays 0
	if m2.filterDraft != "j" {
		t.Fatalf("Then: want filterDraft %q, got %q", "j", m2.filterDraft)
	}
	if m2.cursorIdx != 0 {
		t.Fatalf("Then: cursor should not move in filter mode, got idx %d", m2.cursorIdx)
	}
}

func TestLogList_vimKey_G_movesToLastLineLikeEnd(t *testing.T) {
	// Given: three lines, cursor on first, follow off
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0
	m.follow = false
	// When: G (vim last line)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m2 := next.(*Model)
	// Then: same as End — last line, follow on
	if m2.cursorIdx != 2 {
		t.Fatalf("Then: want cursorIdx 2, got %d", m2.cursorIdx)
	}
	if !m2.follow {
		t.Fatal("Then: want follow on after G (like End)")
	}
}

func TestFilterEditor_vimKey_G_insertsNotNav(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"x"})
	m.filterEdit = true
	m.filterDraft = ""
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m2 := next.(*Model)
	if m2.filterDraft != "G" {
		t.Fatalf("Then: want filterDraft %q, got %q", "G", m2.filterDraft)
	}
}

func TestHighlightEditor_vimKey_k_insertsNotNav(t *testing.T) {
	// Given: highlight compose — k must type into search draft
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"line"})
	m.searchCompose = true
	m.searchDraft = ""
	// When: k
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m2 := next.(*Model)
	// Then:
	if m2.searchDraft != "k" {
		t.Fatalf("Then: want searchDraft %q, got %q", "k", m2.searchDraft)
	}
}

func TestHighlightEditor_vimKey_G_insertsNotNav(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"line"})
	m.searchCompose = true
	m.searchDraft = ""
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m2 := next.(*Model)
	if m2.searchDraft != "G" {
		t.Fatalf("Then: want searchDraft %q, got %q", "G", m2.searchDraft)
	}
}
