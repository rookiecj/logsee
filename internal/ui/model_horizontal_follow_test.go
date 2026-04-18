package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"git.inpt.fr/42dottools/log/internal/buffer"
)

func TestTryBrowseKey_horizontalAtTailRestoresFollow(t *testing.T) {
	// Given: many lines, cursor on last line, scroll tail-aligned, follow off, wrap off
	r := buffer.NewRing(80)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12 // topChrome=2 bottomChrome=1 → viewportH=9
	m.width = 40
	for i := 0; i < 30; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("L%d", i)})
	}
	m.follow = false
	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.scrollTop = len(fidx) - m.viewportH()
	m.lineWrap = false
	m.colRuneOff = 0
	if !m.tailAligned(fidx) {
		t.Fatal("Given: tail-aligned view")
	}
	// When: right (horizontal scroll at tail)
	if ok, _ := m.tryBrowseKey(tea.KeyMsg{Type: tea.KeyRight}, fidx, m.viewportH()); !ok {
		t.Fatal("When: right key expected handled")
	}
	// Then: follow on (still tail-aligned; horizontal does not leave tail)
	if !m.follow {
		t.Fatal("Then: follow should be on when tail-aligned after horizontal key")
	}
	if m.colRuneOff != 1 {
		t.Fatalf("Then: colRuneOff want 1, got %d", m.colRuneOff)
	}
}

func TestTryBrowseKey_wrapOnHorizontalDoesNotChangeFollow(t *testing.T) {
	// Given: wrap on at tail with follow on — horizontal keys do not scroll and must not toggle follow
	r := buffer.NewRing(80)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 40
	for i := 0; i < 30; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("L%d", i)})
	}
	if !m.follow {
		t.Fatal("Given: follow on at tail after appends")
	}
	m.lineWrap = true
	fidx := m.filteredIndices()
	// When
	if ok, _ := m.tryBrowseKey(tea.KeyMsg{Type: tea.KeyRight}, fidx, m.viewportH()); !ok {
		t.Fatal("When: right key")
	}
	// Then
	if !m.follow {
		t.Fatal("Then: follow unchanged when wrap on (no horizontal offset change)")
	}
}

func TestTryBrowseKey_horizontalNotTailLeavesFollowOff(t *testing.T) {
	// Given: last logical line but scroll not at bottom (not tail-aligned)
	r := buffer.NewRing(80)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.height = 12
	m.width = 40
	for i := 0; i < 30; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("L%d", i)})
	}
	m.follow = false
	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	m.scrollTop = len(fidx) - m.viewportH() - 1
	if m.scrollTop < 0 {
		t.Fatal("bad scrollTop")
	}
	if m.tailAligned(fidx) {
		t.Fatal("Given: not tail-aligned")
	}
	// When
	if ok, _ := m.tryBrowseKey(tea.KeyMsg{Type: tea.KeyRight}, fidx, m.viewportH()); !ok {
		t.Fatal("When: right key")
	}
	// Then
	if m.follow {
		t.Fatal("Then: follow should stay off when not tail-aligned")
	}
}
