package tui

import (
	"strings"
	"testing"
)

func TestHelpViewportLinesOverlayUsesListHeightWithoutExtraZones(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:    80,
		Height:   10,
		HelpOpen: true,
		Status:   baseStatusModel(),
	})

	if len(frame.Zones) != 4 {
		t.Fatalf("zone count = %d, want 4 fixed zones without appended help", len(frame.Zones))
	}
	list := findZone(t, frame, ZoneLogList)
	if !list.HelpOverlay {
		t.Fatal("log list zone must render help overlay")
	}
	if got, want := list.Height, 7; got != want {
		t.Fatalf("log list height = %d, want %d", got, want)
	}
	if got, want := countFrameLines(frame), 10; got != want {
		t.Fatalf("frame line count = %d, want %d", got, want)
	}
}

func TestHelpModalIncludesRequiredPRDSections(t *testing.T) {
	content := strings.Join(HelpViewportLines(0, 20, 80), "\n")
	for _, section := range []string{
		"Movement",
		"Follow",
		"Search match",
		"Filter",
		"Search",
		"Bookmarks",
		"Wrap",
		"Selection",
		"Modes",
		"Help",
		"Quit",
	} {
		if !strings.Contains(content, section) {
			t.Fatalf("help content missing section %q: %q", section, content)
		}
	}
	if !strings.Contains(content, "Esc/F1/? close") {
		t.Fatalf("help content missing close keys: %q", content)
	}
	if !strings.Contains(content, "? from log list") {
		t.Fatalf("help content missing log-list question key: %q", content)
	}
}

func TestHelpViewportLinesScrollChangesVisibleBody(t *testing.T) {
	first := HelpViewportLines(0, 6, 60)
	second := HelpViewportLines(1, 6, 60)
	if len(first) != 6 || len(second) != 6 {
		t.Fatalf("viewport heights = %d and %d, want 6", len(first), len(second))
	}
	if first[1] == second[1] {
		t.Fatalf("scrolled body did not change: first=%q second=%q", first[1], second[1])
	}
	if HelpMaxScrollOffset(6) == 0 {
		t.Fatal("help content should require scroll in a 6-line viewport")
	}
}

func countFrameLines(frame Frame) int {
	return strings.Count(FrameText(frame), "\n")
}
