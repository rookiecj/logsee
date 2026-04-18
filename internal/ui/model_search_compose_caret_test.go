package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_searchCompose_leftThenInsertsAtCaret(t *testing.T) {
	// Given: highlight compose with draft "ab", caret at end
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "ab"
	m.searchCaret = 2
	// When: left then X
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'X'}}))
	// Then: inserted between a and b
	if m.searchDraft != "aXb" {
		t.Fatalf("want draft aXb, got %q", m.searchDraft)
	}
	if m.searchCaret != 2 {
		t.Fatalf("want caret after X at index 2, got %d", m.searchCaret)
	}
}

func TestModel_searchCompose_leftAtZeroIsNoop(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "z"
	m.searchCaret = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	if m.searchCaret != 0 {
		t.Fatalf("caret want 0, got %d", m.searchCaret)
	}
}
