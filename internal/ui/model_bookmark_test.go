package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestModel_bookmark_m_assignsSmallestEmptySlot(t *testing.T) {
	// Given: three lines, cursor on first
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0 // follow-at-tail would otherwise leave cursor on last line after batch apply
	// When: m on line 0 then move down and m on line 1
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	// Then: slot 1 and 2 hold seq 1 and 2
	if m.bookmarkSlot[0] != 1 || m.bookmarkSlot[1] != 2 {
		t.Fatalf("Then: want slot1=1 slot2=2 seq, got %#v", m.bookmarkSlot)
	}
}

func TestModel_bookmark_mOnBookmarkedLineClearsOnlyThatSlot(t *testing.T) {
	// Given: two bookmarks
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	// When: m on line 1 (seq 2) to clear slot 2 only
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	// Then: slot 1 unchanged, slot 2 empty
	if m.bookmarkSlot[0] != 1 || m.bookmarkSlot[1] != 0 {
		t.Fatalf("Then: want slot1=1 slot2=0, got %#v", m.bookmarkSlot)
	}
}

func TestModel_bookmark_fullThenRotatesReplaceSlots(t *testing.T) {
	// Given: nine lines, bookmark each in order (fills 1..9)
	r := buffer.NewRing(30)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 9; i++ {
		m.applyIncomingLines([]string{string(rune('a' + i))})
		m.cursorIdx = m.buf.Len() - 1
		m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	}
	// When: tenth, eleventh, twelfth lines bookmarked (rotate replace 1→2→3)
	for _, label := range []string{"z", "y", "x"} {
		m.applyIncomingLines([]string{label})
		m.cursorIdx = m.buf.Len() - 1
		m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	}
	// Then: slot1=seq10(z), slot2=seq11(y), slot3=seq12(x); slots 4..9 still seq4..seq9
	if m.bookmarkSlot[0] != 10 || m.bookmarkSlot[1] != 11 || m.bookmarkSlot[2] != 12 {
		t.Fatalf("Then: slots 1..3 want seq 10,11,12 got %#v", m.bookmarkSlot[:3])
	}
	if m.bookmarkSlot[3] != 4 || m.bookmarkSlot[8] != 9 {
		t.Fatalf("Then: slots 4..9 unchanged seq 4..9, got %#v", m.bookmarkSlot)
	}
	if m.bookmarkRotateNext != 4 {
		t.Fatalf("Then: next replace slot want 4, got %d", m.bookmarkRotateNext)
	}
}

func TestModel_bookmark_digitJumpsToSlotSeq(t *testing.T) {
	// Given: bookmarks on first two lines
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	m.cursorIdx = 2
	// When: press 1
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'1'}}))
	// Then: cursor on index 0
	if m.cursorIdx != 0 {
		t.Fatalf("Then: cursor want 0, got %d", m.cursorIdx)
	}
}

func TestModel_bookmark_jumpNoopWhenSlotEmptyOrSeqNotInList(t *testing.T) {
	// Given: one line bookmarked, cursor at 0
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"only"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	// When: jump to empty slot 2
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'2'}}))
	// Then: cursor unchanged
	if m.cursorIdx != 0 {
		t.Fatalf("Then: cursor want 0, got %d", m.cursorIdx)
	}
	// When: filter hides bookmarked seq (set prog that matches nothing for that seq)
	m.bookmarkSlot[0] = 99
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'1'}}))
	if m.cursorIdx != 0 {
		t.Fatalf("Then: jump to missing seq noop, cursor want 0, got %d", m.cursorIdx)
	}
}

func TestModel_bookmark_jumpClearsShiftRangeKeepsPicks(t *testing.T) {
	// Given: Shift range and a pick, bookmark on line 0
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyDown}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeySpace}))
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	m.cursorIdx = 2
	// When: jump to slot 1
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'1'}}))
	// Then: range cleared, pick on index 2 preserved
	if m.selAnchor >= 0 {
		t.Fatal("Then: expect Shift range cleared")
	}
	if _, ok := m.picked[2]; !ok {
		t.Fatal("Then: expect pick on index 2 preserved")
	}
}

func TestModel_bookmark_mInSearchComposeInsertsDraft(t *testing.T) {
	// Given: search compose open
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"x"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	// When: m
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	// Then: draft contains m, bookmark slot unchanged
	if !strings.Contains(m.searchDraft, "m") {
		t.Fatalf("Then: search draft should contain m, got %q", m.searchDraft)
	}
	if m.bookmarkSlot[0] != 0 {
		t.Fatalf("Then: bookmark should not be set in compose, got %d", m.bookmarkSlot[0])
	}
}

func TestModel_bookmark_seqPrefixShowsSlotDigit(t *testing.T) {
	// Given: bookmark slot 1 on seq 1
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.applyIncomingLines([]string{"hello"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'m'}}))
	// When / Then: plain seq field + styled badge still contains slot digit (PRD §6.7)
	if m.bookmarkSlotIndex(1) != 1 {
		t.Fatalf("Then: want seq 1 in slot 1")
	}
	p := m.formatLinenoAndBookmarkPrefix(1, false)
	if !strings.Contains(p, "     1") || !strings.Contains(p, "1") {
		t.Fatalf("Then: prefix should include padded seq and bookmark digit, got %q", p)
	}
	if lipgloss.Width(p) != listPrefixDisplayWidth(false) {
		t.Fatalf("Then: prefix display width want %d, got %d", listPrefixDisplayWidth(false), lipgloss.Width(p))
	}
}
