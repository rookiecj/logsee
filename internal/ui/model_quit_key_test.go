package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestQuit_q_quitsOnLogListOnly(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	if m.keyFocus() != FocusLogList {
		t.Fatalf("precondition: expected FocusLogList, got %v", m.keyFocus())
	}
	_, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'q'}}))
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd from q on log list")
	}
}

func TestQuit_q_isTextInFilterEdit(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	// Enter filter edit via ':' (PRD §6.1)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{':'}}))
	if !m.filterEdit {
		t.Fatal("precondition: expected filterEdit")
	}
	next, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'q'}}))
	if cmd != nil {
		t.Fatal("q should not quit while in filter edit")
	}
	nm := next.(*Model)
	if nm.filterDraft != "q" {
		t.Fatalf("expected q inserted into filter draft, got %q", nm.filterDraft)
	}
}

func TestQuit_q_isTextInHighlightCompose(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	if !m.searchCompose {
		t.Fatal("precondition: expected searchCompose")
	}
	next, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'q'}}))
	if cmd != nil {
		t.Fatal("q should not quit while in highlight compose")
	}
	nm := next.(*Model)
	if nm.searchDraft != "q" {
		t.Fatalf("expected q inserted into search draft, got %q", nm.searchDraft)
	}
}

func TestQuit_q_swallowedInHelpDialog(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.helpOpen = true
	_, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'q'}}))
	if cmd != nil {
		t.Fatal("q should not quit when help dialog is open")
	}
	if !m.helpOpen {
		t.Fatal("help dialog should remain open")
	}
}

func TestQuit_ctrlQ_quitsOnLogListOnly(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	_, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlQ}))
	if cmd == nil {
		t.Fatal("expected tea.Quit from Ctrl+Q on log list")
	}
}

func TestQuit_ctrlQ_swallowedInFilterEdit(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{':'}}))
	if !m.filterEdit {
		t.Fatal("precondition: expected filterEdit")
	}
	_, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlQ}))
	if cmd != nil {
		t.Fatal("Ctrl+Q should not quit while in filter edit")
	}
}

func TestQuit_ctrlQ_swallowedInHelpDialog(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.helpOpen = true
	_, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlQ}))
	if cmd != nil {
		t.Fatal("Ctrl+Q should not quit while help is open")
	}
	if !m.helpOpen {
		t.Fatal("help should remain open")
	}
}

func TestQuit_ctrlC_quitsEverywhere(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Model)
	}{
		{"log list", func(m *Model) {}},
		{"filter edit", func(m *Model) { m.filterEdit = true }},
		{"highlight compose", func(m *Model) { m.searchCompose = true }},
		{"help open", func(m *Model) { m.helpOpen = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := buffer.NewRing(10)
			m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
			m.width, m.height = 80, 24
			tc.setup(m)
			_, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlC}))
			if cmd == nil {
				t.Fatalf("%s: expected tea.Quit from Ctrl+C", tc.name)
			}
		})
	}
}
