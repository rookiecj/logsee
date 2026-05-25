package tui

import (
	"strings"
	"testing"
)

func TestHistoryPickerViewportLines_HighlightsSelection(t *testing.T) {
	// Given: two history items and selected index 1
	items := []string{"newest", "older"}

	// When: rendering the overlay
	lines := HistoryPickerViewportLines("Filter history", items, 1, 0, 5, 40)

	// Then: title and selected marker appear
	if len(lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(lines))
	}
	if !strings.Contains(lines[2], "> older") {
		t.Fatalf("selected line = %q, want > older", lines[2])
	}
}
