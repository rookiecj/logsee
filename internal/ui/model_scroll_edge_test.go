package ui

import (
	"fmt"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"github.com/charmbracelet/bubbletea"
)

func TestSyncScroll_logical_minimal_unchangedWhenCursorInsideViewport(t *testing.T) {
	// Given: 30 lines, viewport 10 rows, cursor in the middle of the current window
	r := buffer.NewRing(50)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12 // topChrome=2 bottomChrome=1 → viewportH=9
	m.width = 80
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	m.applyIncomingLines(lines)
	m.follow = false
	fidx := m.filteredIndices()
	m.cursorIdx = 15
	m.scrollTop = 10
	// When:
	m.syncScrollToCursor(fidx)
	// Then: scroll position unchanged (cursor already visible)
	if m.scrollTop != 10 {
		t.Fatalf("scrollTop want 10, got %d", m.scrollTop)
	}
}

func TestNavVertical_downAtBottomRowOfViewportScrollsDownByOne(t *testing.T) {
	// Given: cursor on last visible row (scrollTop+vh-1), more lines below (vh=9 when height=12)
	r := buffer.NewRing(50)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	m.applyIncomingLines(lines)
	m.follow = false
	fidx := m.filteredIndices()
	m.cursorIdx = 18
	m.scrollTop = 10
	// When: down
	m.navVertical(fidx, +1, false)
	// Then:
	if m.cursorIdx != 19 {
		t.Fatalf("cursorIdx want 19, got %d", m.cursorIdx)
	}
	if m.scrollTop != 11 {
		t.Fatalf("scrollTop want 11 (scrolled one line), got %d", m.scrollTop)
	}
}

func TestNavVertical_upAtTopRowOfViewportScrollsUpByOne(t *testing.T) {
	// Given: cursor on first visible row
	r := buffer.NewRing(50)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 80
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	m.applyIncomingLines(lines)
	m.follow = false
	fidx := m.filteredIndices()
	m.cursorIdx = 10
	m.scrollTop = 10
	// When: up
	m.navVertical(fidx, -1, false)
	// Then:
	if m.cursorIdx != 9 {
		t.Fatalf("cursorIdx want 9, got %d", m.cursorIdx)
	}
	if m.scrollTop != 9 {
		t.Fatalf("scrollTop want 9, got %d", m.scrollTop)
	}
}

func TestNavVertical_downOnLastLineWithoutMove_keepsFollowOnWhenAlreadyFollowing(t *testing.T) {
	// Given: cursor already on last line with follow on (tail append)
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 5; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("L%d", i)})
	}
	if !m.follow {
		t.Fatal("Given: follow on at tail")
	}
	fidx := m.filteredIndices()
	last := len(fidx) - 1
	// When: down (no room to move)
	m.navVertical(fidx, +1, false)
	// Then: follow stays on at tail; cursor unchanged
	if !m.follow {
		t.Fatal("expected follow on when down does not move but cursor stays on last line at tail")
	}
	if m.cursorIdx != last {
		t.Fatalf("cursorIdx want %d, got %d", last, m.cursorIdx)
	}
}

func TestNavVertical_downOnLastLineWithoutMove_leavesFollowOffWhenNotFollowing(t *testing.T) {
	// Given: last line, follow off (user had scrolled away from tail earlier)
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 5; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("L%d", i)})
	}
	m.follow = false
	fidx := m.filteredIndices()
	last := len(fidx) - 1
	m.cursorIdx = last
	m.syncScrollToCursor(fidx)
	// When: down (no room to move)
	m.navVertical(fidx, +1, false)
	// Then: follow stays off (PRD: no movement does not turn follow on)
	if m.follow {
		t.Fatal("expected follow off when down does not move and follow was off")
	}
	if m.cursorIdx != last {
		t.Fatalf("cursorIdx want %d, got %d", last, m.cursorIdx)
	}
}

func TestPageDown_ctrlF_atLastLine_keepsFollowOn(t *testing.T) {
	// Given: cursor on last line, tail-aligned, follow on
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 5; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("L%d", i)})
	}
	if !m.follow {
		t.Fatal("Given: follow on at tail")
	}
	fidx := m.filteredIndices()
	vh := m.viewportH()
	m.cursorIdx = len(fidx) - 1
	m.syncScrollToCursor(fidx)
	if !m.tailAligned(fidx) {
		t.Fatal("Given: tail aligned")
	}
	// When: Ctrl+F (page down) — cursor cannot move past last line
	if ok, _ := m.tryBrowseKey(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlF}), fidx, vh); !ok {
		t.Fatal("expected ctrl+f handled")
	}
	// Then: follow stays on
	if !m.follow {
		t.Fatal("Then: follow should remain on when already at last line")
	}
}

func TestNavVertical_upFromTailDisablesFollow_downBackToLastReenables(t *testing.T) {
	// Given: short list, cursor on last line with follow at tail
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 10; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("line-%d", i)})
	}
	if !m.follow {
		t.Fatal("Given: expected follow on after tail appends")
	}
	fidx := m.filteredIndices()
	// When: move up (커서 이동 → follow off)
	m.navVertical(fidx, -1, false)
	// Then:
	if m.follow {
		t.Fatal("expected follow off after moving away from last line")
	}
	// When: move back to last line
	m.navVertical(fidx, +1, false)
	// Then:
	if !m.follow {
		t.Fatal("expected follow on when cursor returns to last line at tail")
	}
	if m.cursorIdx != len(fidx)-1 {
		t.Fatalf("cursor on last line, got idx %d", m.cursorIdx)
	}
}

func TestPageUp_ctrlB_wrapMode_movesCursorToTopVisibleLogicalFirst(t *testing.T) {
	// Given: wrap mode with cursor below the top visible logical row
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 20, 12
	m.lineWrap = true
	for i := 0; i < 12; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("line-%02d wrap wrap wrap wrap", i)})
	}
	fidx := m.filteredIndices()
	segs := m.buildWrapSegs(fidx)
	if len(segs) <= m.viewportH() {
		t.Fatal("Given: expected wrapped visual rows exceed viewport")
	}
	m.scrollSegTop = 4
	topVisibleFi := segs[m.scrollSegTop].Fi
	if topVisibleFi >= len(fidx)-1 {
		t.Fatalf("Given: expected top visible logical index before tail, got %d", topVisibleFi)
	}
	m.cursorIdx = topVisibleFi + 2
	m.follow = false

	// When: Ctrl+B (page up)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlB}))

	// Then: cursor snaps to the top visible logical row first
	if m.cursorIdx != topVisibleFi {
		t.Fatalf("cursor after ctrl+b in wrap mode: want %d got %d", topVisibleFi, m.cursorIdx)
	}
}

func TestApplyFileWindowLoaded_bottomPinFallback_keepsCursorOffTop(t *testing.T) {
	// Given: partial file mode; a bottom-pinned command set cursorSeq=999 and viewTopSeq=(999-vh+1).
	// The target cursorSeq is not present in the newly loaded window — applyFileWindowLoaded must
	// infer "prefer bottom" from viewTopSeq < cursorSeq and land the cursor on the last match.
	r := buffer.NewRing(200)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filePartial = true
	vh := m.viewportH()
	m.cursorSeq = 999
	top := int64(999 - (vh - 1))
	if top < 1 {
		top = 1
	}
	m.viewTopSeq = top
	recs := []domain.Record{
		{Seq: 101, Text: "line-101"},
		{Seq: 102, Text: "line-102"},
		{Seq: 103, Text: "line-103"},
	}

	// When: a new file window is applied
	m.applyFileWindowLoaded(recs, 101)

	// Then: fallback cursor should stay on bottom row (last fidx idx) instead of jumping to top.
	if m.cursorIdx != 2 {
		t.Fatalf("cursorIdx want 2 (bottom fallback), got %d", m.cursorIdx)
	}
}
