package ui

import (
	"path/filepath"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/userstate"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_filterHistory_givenDown_whenOpen_thenEsc_keepsDraft(t *testing.T) {
	// Given
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filterHistory = []string{"alpha", "beta"}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.filterDraft = "curr"
	// When: open overlay then Esc
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	// Then
	if m.histKind != histFilterPick {
		t.Fatalf("expected filter history overlay, got %v", m.histKind)
	}
	if m.filterDraft != "curr" {
		t.Fatalf("draft: %q", m.filterDraft)
	}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.histKind != histNone {
		t.Fatal("overlay should close")
	}
	if m.filterDraft != "curr" {
		t.Fatalf("after esc draft: %q", m.filterDraft)
	}
}

func TestModel_filterHistory_givenOverlay_whenDownEnter_thenDraftFromSelection(t *testing.T) {
	// Given
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filterHistory = []string{"alpha", "beta"}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.filterDraft = "curr"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	// When: move selection down then Enter
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	// Then
	if m.histKind != histNone {
		t.Fatal("overlay should close")
	}
	if m.filterDraft != "beta" {
		t.Fatalf("want beta got %q", m.filterDraft)
	}
}

func TestModel_highlightHistory_givenCompose_whenDownEsc_thenDraftKept(t *testing.T) {
	// Given
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.highlightHistory = []string{"h1", "h2"}
	m.applyIncomingLines([]string{"line"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "typing"
	// When
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	// Then
	if m.histKind != histHighlightPick {
		t.Fatalf("expected highlight overlay %v", m.histKind)
	}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.searchDraft != "typing" {
		t.Fatalf("draft %q", m.searchDraft)
	}
}

func TestModel_persist_givenStateFile_whenFilterApply_thenSaved(t *testing.T) {
	// Given
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	h := &HistoryOpts{StateFile: path, Initial: userstate.EmptySnapshot()}
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", h, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.filterDraft = "x"
	// When
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	// Then
	loaded, err := userstate.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastFilter != "x" {
		t.Fatalf("LastFilter %q", loaded.LastFilter)
	}
	if len(loaded.FilterHistory) != 1 || loaded.FilterHistory[0] != "x" {
		t.Fatalf("FilterHistory %#v", loaded.FilterHistory)
	}
}

func TestModel_restore_givenSnapshot_whenNewModel_thenFilterAndHighlight(t *testing.T) {
	// Given
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	snap := userstate.Snapshot{
		Version:          userstate.SnapshotVersion,
		LastFilter:       "x",
		LastHighlight:    "needle",
		FilterHistory:    []string{"x"},
		HighlightHistory: []string{"needle"},
	}
	if err := userstate.Save(path, snap); err != nil {
		t.Fatal(err)
	}
	h := &HistoryOpts{StateFile: path, Initial: snap}
	r := buffer.NewRing(10)
	// When
	m := NewModel(r, nil, false, false, "", "stdin", "", h, nil, nil)
	m.applyIncomingLines([]string{"x", "y"})
	// Then
	if m.appliedFilter != "x" || m.searchBuf != "needle" {
		t.Fatalf("applied=%q searchBuf=%q", m.appliedFilter, m.searchBuf)
	}
	if len(m.filteredIndices()) != 1 {
		t.Fatalf("filter should keep one line, got %d", len(m.filteredIndices()))
	}
}
