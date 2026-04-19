package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestHelpDialog_F1ShowsVersionAndClosesWithEsc(t *testing.T) {
	// Given: model with a known version string
	r := buffer.NewRing(3)
	var tm tea.Model = NewModel(r, nil, false, false, "", "stdin", "v9.9.9-test", nil, nil, nil)
	m := tm.(*Model)
	m.width = 80
	m.height = 24

	// When: F1
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = tm.(*Model)
	if !m.helpOpen {
		t.Fatal("expected help dialog open after F1")
	}
	view := m.View()
	if !strings.Contains(view, "v9.9.9-test") {
		t.Fatalf("expected version in help view, got:\n%s", view)
	}
	if !strings.Contains(view, "logsee") || !strings.Contains(view, "Ctrl+W") {
		t.Fatalf("expected help title/key hints in view, got:\n%s", view)
	}
	for _, mark := range []string{"이 대화창", "공통", "로그 목록 화면", "필터 입력", "검색(highlight) 입력"} {
		if !strings.Contains(view, mark) {
			t.Fatalf("expected mode section %q in help view, got:\n%s", mark, view)
		}
	}

	// When: Esc
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(*Model)
	// Then: dialog closed
	if m.helpOpen {
		t.Fatal("expected help closed after Esc")
	}
}

func TestFilterSyntaxHelp_F1FromFilterInput(t *testing.T) {
	// Given: filter input focus
	r := buffer.NewRing(3)
	var tm tea.Model = NewModel(r, nil, false, false, "", "stdin", "v1-filter", nil, nil, nil)
	m := tm.(*Model)
	m.width = 100
	m.height = 30
	m.filterEdit = true
	m.filterDraft = "x"
	// When: F1
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = tm.(*Model)
	// Then: filter-syntax help (not full keymap)
	if !m.helpOpen || !m.helpFilterSyntax {
		t.Fatalf("expected filter syntax help, helpOpen=%v helpFilterSyntax=%v", m.helpOpen, m.helpFilterSyntax)
	}
	view := m.View()
	if !strings.Contains(view, "필터 문법") {
		t.Fatalf("expected filter help title in view, got:\n%s", view)
	}
	if !strings.Contains(view, "v1-filter") {
		t.Fatal("expected version in filter help")
	}
	if !strings.Contains(view, "결합 규칙") {
		t.Fatal("expected §7 summary section")
	}
	if strings.Contains(view, "로그 목록 화면") {
		t.Fatal("did not expect full-help-only section in filter syntax dialog")
	}
	// When: Esc — help closes, filter editing continues
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(*Model)
	if m.helpOpen || m.helpFilterSyntax {
		t.Fatal("expected help closed")
	}
	if !m.filterEdit {
		t.Fatal("expected still in filter input after closing help")
	}
}

func TestHelpDialog_qSwallowedWhenHelpOpen_ctrlCStillQuits(t *testing.T) {
	// Given: help open
	r := buffer.NewRing(3)
	var tm tea.Model = NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m := tm.(*Model)
	m.width = 40
	m.height = 10
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyF1})

	// When: q while help open — should be swallowed, help stays, no quit
	var cmd tea.Cmd
	tm, cmd = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m2 := tm.(*Model)
	if cmd != nil {
		t.Fatal("q should not quit while help is open")
	}
	if !m2.helpOpen {
		t.Fatal("help should remain open after q")
	}
	// When: Ctrl+C — still quits from within the help dialog
	_, cmd = tm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C should still quit with help open")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected cmd to send a message")
	}
}

func TestHelpDialog_QuestionOpensHelpOnLogList(t *testing.T) {
	// Given: log list focus (default)
	r := buffer.NewRing(3)
	var tm tea.Model = NewModel(r, nil, false, false, "", "stdin", "v-qm", nil, nil, nil)
	m := tm.(*Model)
	m.width = 80
	m.height = 24

	// When: "?" — works when F1 is captured by the IDE or not delivered as KeyF1
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = tm.(*Model)
	if !m.helpOpen || m.helpFilterSyntax {
		t.Fatalf("expected full help from ?, helpOpen=%v helpFilterSyntax=%v", m.helpOpen, m.helpFilterSyntax)
	}
	view := m.View()
	if !strings.Contains(view, "v-qm") || !strings.Contains(view, "공통") {
		t.Fatalf("expected full help view, got:\n%s", view)
	}
}

func TestHelpDialog_F1TogglesClosed(t *testing.T) {
	// Given: help open
	r := buffer.NewRing(3)
	var tm tea.Model = NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m := tm.(*Model)
	m.width = 60
	m.height = 12
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyF1})

	// When: F1 again
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = tm.(*Model)
	// Then
	if m.helpOpen {
		t.Fatal("expected F1 to close help when already open")
	}
}
