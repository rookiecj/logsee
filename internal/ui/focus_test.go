package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
)

func TestModel_keyFocus_default_is_log_list(t *testing.T) {
	// Given: fresh model (no filter edit, no search compose)
	m := NewModel(buffer.NewRing(8), nil, false, false, "", "stdin", "test", nil, nil, nil)
	// When
	f := m.keyFocus()
	// Then
	if f != FocusLogList {
		t.Fatalf("expected FocusLogList, got %v", f)
	}
}

func TestModel_keyFocus_filter_edit(t *testing.T) {
	// Given: filter input mode
	m := NewModel(buffer.NewRing(8), nil, false, false, "", "stdin", "test", nil, nil, nil)
	m.filterEdit = true
	// When
	f := m.keyFocus()
	// Then
	if f != FocusFilterEditor {
		t.Fatalf("expected FocusFilterEditor, got %v", f)
	}
}

func TestModel_keyFocus_highlight_compose(t *testing.T) {
	// Given: highlight compose from list (not filter edit)
	m := NewModel(buffer.NewRing(8), nil, false, false, "", "stdin", "test", nil, nil, nil)
	m.searchCompose = true
	// When
	f := m.keyFocus()
	// Then
	if f != FocusHighlightEditor {
		t.Fatalf("expected FocusHighlightEditor, got %v", f)
	}
}

func TestModel_keyFocus_filter_wins_over_compose(t *testing.T) {
	// Given: both flags true (defensive: filter branch must take precedence in keyFocus)
	m := NewModel(buffer.NewRing(8), nil, false, false, "", "stdin", "test", nil, nil, nil)
	m.filterEdit = true
	m.searchCompose = true
	// When
	f := m.keyFocus()
	// Then
	if f != FocusFilterEditor {
		t.Fatalf("expected FocusFilterEditor to win, got %v", f)
	}
}
