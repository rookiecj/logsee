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

func TestLogList_vimKey_J_extendsRangeDownLikeShiftDown(t *testing.T) {
	// Given: log list, three lines, cursor on first, no prior selection
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0
	m.follow = false
	// When: Shift+J (vim) — should behave like Shift+↓
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m2 := next.(*Model)
	// Then: selection anchor set, cursor advanced one line
	if m2.selAnchor < 0 {
		t.Fatal("Then: expected selection anchor after Shift+J")
	}
	if m2.cursorIdx != 1 {
		t.Fatalf("Then: want cursorIdx 1, got %d", m2.cursorIdx)
	}
}

func TestLogList_vimKey_K_extendsRangeUpLikeShiftUp(t *testing.T) {
	// Given: cursor on last line
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 2
	m.follow = false
	// When: Shift+K
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	m2 := next.(*Model)
	// Then:
	if m2.selAnchor < 0 {
		t.Fatal("Then: expected selection anchor after Shift+K")
	}
	if m2.cursorIdx != 1 {
		t.Fatalf("Then: want cursorIdx 1, got %d", m2.cursorIdx)
	}
}

func TestFilterEditor_vimKey_J_insertsNotNav(t *testing.T) {
	// Given: filter compose — J must not be remapped
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"x"})
	m.filterEdit = true
	m.filterDraft = ""
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m2 := next.(*Model)
	if m2.filterDraft != "J" {
		t.Fatalf("Then: want filterDraft %q, got %q", "J", m2.filterDraft)
	}
	if m2.selAnchor >= 0 {
		t.Fatal("Then: filter mode must not create a range selection on J")
	}
}

func TestHighlightEditor_vimKey_K_insertsNotNav(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	m.applyIncomingLines([]string{"line"})
	m.searchCompose = true
	m.searchDraft = ""
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	m2 := next.(*Model)
	if m2.searchDraft != "K" {
		t.Fatalf("Then: want searchDraft %q, got %q", "K", m2.searchDraft)
	}
}
