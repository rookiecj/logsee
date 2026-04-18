package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const bookmarkSlotCount = 9

// listPrefixDisplayWidth is the fixed display width of the list left gutter (seq+bookmark or bookmark-only).
func listPrefixDisplayWidth(noLineNumbers bool) int {
	if !noLineNumbers {
		return runewidth.StringWidth(fmt.Sprintf("%6d %c ", 999_999, '9'))
	}
	return runewidth.StringWidth("  ")
}

func bookmarkBadgeStyle(isCursor bool) lipgloss.Style {
	st := lipgloss.NewStyle().
		Background(lipgloss.Color("25")).
		Foreground(lipgloss.Color("230")).
		Bold(true)
	if isCursor {
		// High contrast on reversed cursor row (PRD §5 cursor strip).
		st = lipgloss.NewStyle().
			Background(lipgloss.Color("214")).
			Foreground(lipgloss.Color("16")).
			Bold(true)
	}
	return st
}

// formatLinenoAndBookmarkPrefix renders right-aligned seq + optional styled bookmark digit (PRD §6.7).
func (m *Model) formatLinenoAndBookmarkPrefix(seq int64, isCursor bool) string {
	seqPart := fmt.Sprintf("%6d ", seq)
	k := m.bookmarkSlotIndex(seq)
	if k == 0 {
		return seqPart + "  "
	}
	ch := string(rune('0' + k))
	badge := bookmarkBadgeStyle(isCursor).Render(ch)
	return seqPart + badge + " "
}

// formatNoLineNumberBookmarkPrefix is the two-cell gutter before body when --no-line-numbers (PRD §6.7).
func (m *Model) formatNoLineNumberBookmarkPrefix(seq int64, isCursor bool) string {
	k := m.bookmarkSlotIndex(seq)
	if k == 0 {
		return "  "
	}
	ch := string(rune('0' + k))
	badge := bookmarkBadgeStyle(isCursor).Render(ch)
	return padDisplayWidth(badge+" ", 2)
}

// bookmarkSlotIndex returns 1..9 if seq occupies a slot, else 0 (PRD §6.7).
func (m *Model) bookmarkSlotIndex(seq int64) int {
	for i := 0; i < bookmarkSlotCount; i++ {
		if m.bookmarkSlot[i] == seq {
			return i + 1
		}
	}
	return 0
}

// tryBookmarkKey handles PRD §6.7 when FocusLogList; returns true if the key was consumed.
func (m *Model) tryBookmarkKey(msg tea.KeyMsg, fidx []int) (bool, tea.Cmd) {
	switch msg.String() {
	case "m":
		m.toggleBookmarkAtCursor(fidx)
		return true, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		slot := int(msg.String()[0] - '1')
		return true, m.bookmarkJumpCmd(slot, fidx)
	default:
		return false, nil
	}
}

func (m *Model) bookmarkJumpCmd(slot0 int, fidx []int) tea.Cmd {
	if slot0 < 0 || slot0 >= bookmarkSlotCount {
		return nil
	}
	seq := m.bookmarkSlot[slot0]
	if seq == 0 {
		return nil
	}
	for fi, ri := range fidx {
		if m.buf.At(ri).Seq == seq {
			m.applyBookmarkJump(fi, fidx)
			return nil
		}
	}
	return m.cmdBookmarkJumpToSeq(seq)
}

func (m *Model) toggleBookmarkAtCursor(fidx []int) {
	if len(fidx) == 0 {
		return
	}
	m.clampCursor(fidx)
	seq := m.buf.At(fidx[m.cursorIdx]).Seq
	if k := m.bookmarkSlotIndex(seq); k > 0 {
		m.bookmarkSlot[k-1] = 0
		return
	}
	for i := 0; i < bookmarkSlotCount; i++ {
		if m.bookmarkSlot[i] == 0 {
			m.bookmarkSlot[i] = seq
			return
		}
	}
	repl := m.bookmarkRotateNext
	if repl < 1 || repl > bookmarkSlotCount {
		repl = 1
	}
	m.bookmarkSlot[repl-1] = seq
	m.bookmarkRotateNext = repl%bookmarkSlotCount + 1
}

// applyBookmarkJump moves the cursor like next/prev match nav (PRD §6.7: clear range only, keep picks; follow §8.5).
func (m *Model) applyBookmarkJump(toFI int, fidx []int) {
	prev := m.cursorIdx
	m.clearRangeSelection()
	m.follow = false
	m.cursorIdx = toFI
	m.syncScrollToCursor(fidx)
	if m.lineWrap && m.cursorIdx != prev && len(fidx) > 0 && m.cursorIdx == len(fidx)-1 {
		m.pinWrapSegScrollToLogicalTail(fidx)
	}
	if m.cursorIdx != prev && m.tailAligned(fidx) {
		m.follow = true
	}
}
