package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestLogList_ctrlI_togglesLineNumbers(t *testing.T) {
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b"})
	if m.noLineNumbers {
		t.Fatal("precondition: expected line numbers on by default")
	}
	// Ctrl+I (shares keycode with Tab) toggles the column off
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlI}))
	if !m.noLineNumbers {
		t.Fatal("expected noLineNumbers=true after first Ctrl+I")
	}
	// Second press toggles it back on
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlI}))
	if m.noLineNumbers {
		t.Fatal("expected noLineNumbers=false after second Ctrl+I")
	}
}

func TestLogList_tab_togglesLineNumbers(t *testing.T) {
	// Tab produces the same keycode as Ctrl+I on terminals; the binding matches both.
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyTab}))
	if !m.noLineNumbers {
		t.Fatal("expected Tab to toggle line numbers off")
	}
}

func TestFilterEdit_ctrlI_doesNotToggle(t *testing.T) {
	// In filter compose Tab/Ctrl+I should not toggle the log-list setting.
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{':'}}))
	if !m.filterEdit {
		t.Fatal("precondition: expected filterEdit")
	}
	before := m.noLineNumbers
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlI}))
	if m.noLineNumbers != before {
		t.Fatalf("line numbers must not toggle while in filter edit (was %v, now %v)", before, m.noLineNumbers)
	}
}
