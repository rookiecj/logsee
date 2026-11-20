package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
	"logsee/internal/usecase"
	"testing"
	"unicode/utf8"
)

func TestReviewUTF8ViewportBoundaries(t *testing.T) {
	logs := []LogLineModel{{RawLineNumber: 1, Text: "가나다라마"}}
	for _, wrap := range []bool{false, true} {
		for _, row := range visualLogModels(logs, 10, 8, wrap, 1) {
			if !utf8.ValidString(row.Text) {
				t.Errorf("wrap=%v invalid UTF-8 after slicing: %q", wrap, row.Text)
			}
		}
	}
}

func TestReviewOverlappingColorsPreserveText(t *testing.T) {
	text := "error"
	ranges := usecase.NewSearchMatcher("error#red err#green").HighlightRanges(text)
	got := ansi.Strip(styleHighlightedText(text, ranges, logListStyle))
	if got != text {
		t.Fatalf("visible log text changed from %q to %q", text, got)
	}
}

func TestDisplayCellWidth(t *testing.T) {
	for _, text := range []string{"가나다", "A👨‍👩‍👧‍👦B", "ééé"} {
		for width := 1; width < 8; width++ {
			for offset := 0; offset < 8; offset++ {
				a, b := horizontalTextWindow(text, width, offset)
				got := text[a:b]
				if !utf8.ValidString(got) || uniseg.StringWidth(got) > width {
					t.Fatalf("width %d offset %d: %q", width, offset, got)
				}
			}
		}
	}
}
