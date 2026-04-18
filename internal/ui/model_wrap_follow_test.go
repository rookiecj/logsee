package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
)

func TestWrap_follow_pinsVisualTailWhenFollowOnLastLine(t *testing.T) {
	// Given: wrap on, one logical line that wraps to more visual rows than viewport height, follow on
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 36
	m.height = 12 // vh = 9
	m.lineWrap = true
	m.applyIncomingLines([]string{strings.Repeat("W", 400)})
	fidx := m.filteredIndices()
	if len(fidx) != 1 {
		t.Fatalf("Given: want 1 filtered line, got %d", len(fidx))
	}
	m.follow = true
	m.cursorIdx = 0
	m.syncScrollToCursor(fidx)
	// When / Then: 꼬리 정렬이면 tailAligned (신규 줄 follow·§8.5)
	if !m.tailAligned(fidx) {
		t.Fatalf("Then: tailAligned want true (scrollSegTop=%d)", m.scrollSegTop)
	}
}

func TestWrap_follow_newLineKeepsTailWhenLongWrappedBuffer(t *testing.T) {
	// Given: wrap on, long first line (many segments), follow on at tail, cursor on last logical line
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 40
	m.height = 12
	m.lineWrap = true
	m.applyIncomingLines([]string{strings.Repeat("X", 500)})
	fidx := m.filteredIndices()
	if len(fidx) != 1 {
		t.Fatalf("Given: want 1 line, got %d", len(fidx))
	}
	if !m.follow {
		t.Fatal("Given: default follow on after append at tail")
	}
	if !m.tailAligned(fidx) {
		t.Fatalf("Given: tail aligned before second append (scrollSegTop=%d)", m.scrollSegTop)
	}
	// When: another line arrives
	m.applyIncomingLines([]string{"new"})
	// Then: cursor follows new last line and view stays tail-aligned for wrap
	fidx = m.filteredIndices()
	if len(fidx) != 2 {
		t.Fatalf("Then: want 2 lines, got %d", len(fidx))
	}
	if m.cursorIdx != 1 || !m.follow {
		t.Fatalf("Then: want cursor on last + follow on, got idx=%d follow=%v", m.cursorIdx, m.follow)
	}
	if !m.tailAligned(fidx) {
		t.Fatalf("Then: tailAligned want true (scrollSegTop=%d)", m.scrollSegTop)
	}
}

func TestWrap_navDownToLastLine_pinsTailForFollowRestore(t *testing.T) {
	// Given: wrap on, two lines — first is long (many segments), second short; follow off; cursor on first line
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width = 44
	m.height = 12
	m.lineWrap = true
	m.follow = false
	m.applyIncomingLines([]string{strings.Repeat("Y", 500), "z"})
	fidx := m.filteredIndices()
	m.cursorIdx = 0
	m.syncScrollToCursor(fidx)
	// When: move down to last logical line (follow restores if 꼬리 정렬)
	m.navVertical(fidx, +1, false)
	// Then: tailAligned so follow can turn on
	if !m.tailAligned(fidx) {
		t.Fatalf("Then: tailAligned after nav to last line (scrollSegTop=%d)", m.scrollSegTop)
	}
	if !m.follow {
		t.Fatal("Then: follow should restore on last line at tail (§8.5)")
	}
}
