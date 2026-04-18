package ui

import (
	"strings"
	"testing"
)

func TestWrapLineRunes_emptyLineOneSegment(t *testing.T) {
	// Given: empty rune slice and positive width
	// When:
	sp := wrapLineRunes([]rune{}, 10)
	// Then: one zero-length visual row
	if len(sp) != 1 || sp[0][0] != 0 || sp[0][1] != 0 {
		t.Fatalf("expected single {{0,0}} segment, got %#v", sp)
	}
}

func TestWrapLineRunes_splitsByDisplayWidth_rejoinsOriginal(t *testing.T) {
	// Given: ASCII longer than maxCells
	s := "abcdefghijkl"
	rs := []rune(s)
	maxCells := 5
	// When:
	sp := wrapLineRunes(rs, maxCells)
	// Then: segments cover full string without gaps or reorder
	if len(sp) < 3 {
		t.Fatalf("expected at least 3 segments for 12 chars / width 5, got %d %#v", len(sp), sp)
	}
	var b strings.Builder
	for _, p := range sp {
		b.WriteString(string(rs[p[0]:p[1]]))
	}
	if b.String() != s {
		t.Fatalf("rejoined %q want %q", b.String(), s)
	}
}

func TestWrapLineRunes_wideRuneConsumesTwoCells(t *testing.T) {
	// Given: a rune with display width 2 and maxCells 3 → at most one wide + one narrow per segment edge case
	rs := []rune("a界b界c")
	sp := wrapLineRunes(rs, 3)
	// When / Then: rejoin
	var b strings.Builder
	for _, p := range sp {
		b.WriteString(string(rs[p[0]:p[1]]))
	}
	if b.String() != string(rs) {
		t.Fatalf("rejoin: got %q want %q", b.String(), string(rs))
	}
}
