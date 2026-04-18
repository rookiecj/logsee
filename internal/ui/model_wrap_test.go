package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_ctrlW_togglesLineWrap_resetsHorizontalOffset(t *testing.T) {
	// Given: model with horizontal offset and list lines
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x"})
	m.colRuneOff = 7
	// When: Ctrl+W (wrap on)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlW}))
	// Then: wrap is on and column offset cleared
	if !m.lineWrap {
		t.Fatal("expected lineWrap true after first ctrl+w")
	}
	if m.colRuneOff != 0 {
		t.Fatalf("expected colRuneOff 0 when wrap on, got %d", m.colRuneOff)
	}
	// When: Ctrl+W again (wrap off)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlW}))
	// Then:
	if m.lineWrap {
		t.Fatal("expected lineWrap false after second ctrl+w")
	}
}

func TestModel_lineWrap_horizontalKeysDoNotChangeColRuneOff(t *testing.T) {
	// Given: wrap on and zero offset
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"line"})
	m.lineWrap = true
	m.colRuneOff = 0
	// When: left and right
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRight}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	// Then: offset unchanged
	if m.colRuneOff != 0 {
		t.Fatalf("expected colRuneOff 0 in wrap mode, got %d", m.colRuneOff)
	}
}

func TestModel_buildWrapSegs_splitsLongLogicalLine(t *testing.T) {
	// Given: narrow width so one logical line becomes multiple segments
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 24
	m.applyIncomingLines([]string{strings.Repeat("a", 60)})
	fidx := m.filteredIndices()
	// When:
	segs := m.buildWrapSegs(fidx)
	// Then: more than one segment for the single filtered line
	if len(segs) < 2 {
		t.Fatalf("expected multiple wrap segments, got %d %#v", len(segs), segs)
	}
}
