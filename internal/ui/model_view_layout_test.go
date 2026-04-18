package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/lipgloss"
)

func TestViewportH_topAndBottomChrome(t *testing.T) {
	// Given: fixed terminal height with default chrome constants
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 10
	// When
	vh := m.viewportH()
	// Then: two top rows + one status row reserved
	want := 10 - topChromeLines - bottomChromeLines
	if vh != want {
		t.Fatalf("viewportH want %d, got %d", want, vh)
	}
}

func TestView_layout_filterOnTopStatusOnBottom(t *testing.T) {
	// Given: dimensions and a few lines so status includes line counts
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "/tmp/out.log", "stdin", "", nil, nil, nil)
	m.width = 70
	m.height = 8
	m.applyIncomingLines([]string{"a", "b"})
	// When
	out := m.View()
	lines := strings.Split(out, "\n")
	// Then: exact row count; first row shows filter chrome; last row shows status fields
	if len(lines) != 8 {
		t.Fatalf("want 8 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "FILTER") {
		t.Fatalf("first line should be filter chrome, got %q", lines[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "lines:") || !strings.Contains(last, "follow:") || !strings.Contains(last, "type:") {
		t.Fatalf("last line should be status bar (lines, type, follow), got %q", last)
	}
}

func TestBuildStatusLine_showsFollowOnThenOff(t *testing.T) {
	// Given: model with ring buffer
	r := buffer.NewRing(5)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	// When: follow is on
	m.follow = true
	// Then
	if got := m.buildStatusLine(); !strings.Contains(got, "follow:on") {
		t.Fatalf("Then: status should include follow:on, got %q", got)
	}
	// When: follow is off
	m.follow = false
	// Then
	if got := m.buildStatusLine(); !strings.Contains(got, "follow:off") {
		t.Fatalf("Then: status should include follow:off, got %q", got)
	}
}

func TestRenderStatusBar_followOnOffStyledDifferently(t *testing.T) {
	// Given: no copy toast so status uses styled base block
	r := buffer.NewRing(3)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 80
	m.height = 6
	m.applyIncomingLines([]string{"x"})
	m.follow = true
	rowOn := m.renderStatusBar()
	m.follow = false
	rowOff := m.renderStatusBar()
	// Then: styled rows differ (on vs off branch uses different lipgloss spans)
	if rowOn == rowOff {
		t.Fatal("expected follow on/off to produce different styled rows")
	}
}

func TestRenderStatusBar_copyFlashUsesHighlightStyle(t *testing.T) {
	// Given: width and a clipboard toast message
	r := buffer.NewRing(5)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 90
	m.height = 6
	m.applyIncomingLines([]string{"a"})
	m.copyFlash = "2 lines copied"
	// When:
	row := m.renderStatusBar()
	// Then: toast text present and ANSI styling applied (lipgloss)
	if !strings.Contains(row, "2 lines copied") {
		t.Fatalf("expected toast text in row: %q", row)
	}
	plain := m.renderChromeLine(m.buildStatusLine(), m.statusBarStyle())
	if row == plain {
		t.Fatal("expected styled status row to differ from plain chrome line")
	}
	if w := lipgloss.Width(row); w != m.width {
		t.Fatalf("Then: row display width want %d, got %d", m.width, w)
	}
}
