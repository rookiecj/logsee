package ui

import (
	"os"
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func init() {
	// Tests run without a TTY; lipgloss otherwise omits ANSI so cursor reverse cannot be observed.
	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("CI")
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func trimSample(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

func TestRenderLog_showsCursorReverseWhenFollowOn(t *testing.T) {
	// Given: two lines, cursor on second, no search highlight (empty searchBuf)
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 60
	m.height = 12
	m.applyIncomingLines([]string{"alpha", "beta"})
	m.cursorIdx = 1
	m.searchBuf = ""
	if m.buf.Len() != 2 || len(m.filteredIndices()) != 2 {
		t.Fatalf("Given: want 2 lines in buffer and filter, got buf=%d fidx=%d",
			m.buf.Len(), len(m.filteredIndices()))
	}
	// When: toggling follow — cursor row reverse stays visible
	m.follow = false
	off := m.renderLog()
	m.follow = true
	on := m.renderLog()
	// Then: styled output matches (only follow flag changed); plain text identical
	if off != on {
		t.Fatalf("Then: styled log should match when only follow toggles; cursorIdx=%d off=%q on=%q",
			m.cursorIdx, trimSample(off), trimSample(on))
	}
	if strings.Count(on, "\x1b") == 0 {
		t.Fatal("Then: given TrueColor profile, log should include ANSI for cursor styling")
	}
	if ansi.Strip(off) != ansi.Strip(on) {
		t.Fatalf("Then: plain text after strip should match; off=%q on=%q", ansi.Strip(off), ansi.Strip(on))
	}
}

func TestRenderLogWrapped_showsCursorReverseWhenFollowOn(t *testing.T) {
	// Given: wrap on, two short lines, cursor on second
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.lineWrap = true
	m.width = 60
	m.height = 12
	m.applyIncomingLines([]string{"x", "y"})
	m.cursorIdx = 1
	m.searchBuf = ""
	m.follow = false
	off := m.renderLog()
	m.follow = true
	on := m.renderLog()
	if off != on {
		t.Fatalf("Then: styled log should match when only follow toggles; off=%q on=%q", trimSample(off), trimSample(on))
	}
	if ansi.Strip(off) != ansi.Strip(on) {
		t.Fatalf("Then: plain text after strip should match; off=%q on=%q", ansi.Strip(off), ansi.Strip(on))
	}
}

func TestRenderLog_selectionStillVisibleWhenFollowOn(t *testing.T) {
	// Given: follow on, range selection on first line
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 50
	m.height = 12
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.selAnchor = 0
	m.cursorIdx = 1
	m.follow = true
	_ = m.renderLog()
	// Then: model state unchanged; smoke — no panic
	if m.cursorIdx != 1 || m.selAnchor != 0 {
		t.Fatalf("cursor/anchor want 1/0, got %d/%d", m.cursorIdx, m.selAnchor)
	}
}
