package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
)

func TestBuildHighlightLine_compose_usesFilterLikePattern(t *testing.T) {
	// Given: list focus, highlight compose, draft text
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.searchCompose = true
	m.searchDraft = "needle"
	m.searchCaret = len([]rune("needle"))
	m.searchBuf = ""
	// When:
	line := m.buildHighlightLine()
	// Then: label and >draft_ mirror filter chrome (FILTER │ > …)
	if !strings.Contains(line, "HIGHLIGHT") || !strings.Contains(line, "│") || !strings.Contains(line, "> needle_") {
		t.Fatalf("unexpected chrome: %q", line)
	}
}

func TestBuildHighlightLine_idle_showsEmptySetWhenNoHighlight(t *testing.T) {
	// Given: no committed highlight
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.searchCompose = false
	m.searchBuf = ""
	m.searchDraft = ""
	// When:
	line := m.buildHighlightLine()
	// Then:
	if !strings.Contains(line, "HIGHLIGHT") || !strings.Contains(line, "∅") {
		t.Fatalf("unexpected chrome: %q", line)
	}
}

func TestBuildHighlightLine_idle_showsCommittedQuery(t *testing.T) {
	// Given: committed highlight string
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.searchBuf = "error"
	m.searchDraft = m.searchBuf
	// When:
	line := m.buildHighlightLine()
	// Then:
	if !strings.Contains(line, "error") || strings.Contains(line, "search:") || strings.Contains(line, "search>") {
		t.Fatalf("unexpected chrome: %q", line)
	}
}
