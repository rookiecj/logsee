package ui

import (
	"strings"
	"unicode"

	"git.inpt.fr/42dottools/log/internal/filter"
	"git.inpt.fr/42dottools/log/internal/userstate"
	"github.com/charmbracelet/bubbletea"
)

func (m *Model) clearAppliedFilter() {
	m.prog = filter.Program{}
	m.appliedFilter = ""
	m.filterErr = ""
}

// popFilterInputFocus implements PRD §6.0: one step of the mode stack — filter-input focus → list focus
// (exit filter edit, revert draft to applied filter string; prog unchanged).
func (m *Model) popFilterInputFocus() {
	m.filterEdit = false
	m.filterDraft = m.appliedFilter
	m.filterErr = ""
}

func (m *Model) syncFilterCursorEnd() {
	m.filterCursor = len([]rune(m.filterDraft))
}

func (m *Model) clampFilterCursor() {
	n := len([]rune(m.filterDraft))
	if m.filterCursor < 0 {
		m.filterCursor = 0
	}
	if m.filterCursor > n {
		m.filterCursor = n
	}
}

func (m *Model) clampSearchCaret() {
	rs := []rune(m.searchDraft)
	n := len(rs)
	if m.searchCaret > n {
		m.searchCaret = n
	}
	if m.searchCaret < 0 {
		m.searchCaret = 0
	}
}

func (m *Model) insertSearchDraft(s string) {
	rs := []rune(m.searchDraft)
	ins := []rune(s)
	m.clampSearchCaret()
	out := make([]rune, 0, len(rs)+len(ins))
	out = append(out, rs[:m.searchCaret]...)
	out = append(out, ins...)
	out = append(out, rs[m.searchCaret:]...)
	m.searchDraft = string(out)
	m.searchCaret += len(ins)
}

// trySearchComposeKey handles highlight compose-only keys. Returns true if the key was consumed (including no-op left at 0).
// Navigation keys (e.g. ↑↓) return false so tryBrowseKey runs (PRD §6.2·§6.5).
func (m *Model) trySearchComposeKey(msg tea.KeyMsg, fidx []int) bool {
	if !m.searchCompose {
		return false
	}
	switch msg.String() {
	case "/":
		// ':' is only a filter shortcut from the log list (PRD: vim-style); in highlight compose it is text.
		m.clearAllSelection()
		m.searchCompose = false
		m.searchDraft = m.searchBuf
		m.clampSearchCaret()
		m.filterEdit = true
		m.filterDraft = m.appliedFilter
		m.syncFilterCursorEnd()
		m.filterErr = ""
		return true
	case "enter":
		m.searchBuf = strings.TrimSpace(m.searchDraft)
		m.searchDraft = m.searchBuf
		m.searchCompose = false
		m.searchCaret = len([]rune(m.searchBuf))
		if m.searchBuf != "" {
			m.highlightHistory = userstate.PushMRU(m.highlightHistory, m.searchBuf, userstate.MaxHistoryEntries)
		}
		m.persistState()
		m.syncScrollToCursor(fidx)
		return true
	case "esc":
		if m.hasListSelection() {
			m.clearAllSelection()
			return true
		}
		m.searchDraft = m.searchBuf
		m.searchCompose = false
		m.searchCaret = len([]rune(m.searchBuf))
		return true
	case "n":
		m.insertSearchDraft("n")
		m.syncScrollToCursor(fidx)
		return true
	case "p":
		m.insertSearchDraft("p")
		m.syncScrollToCursor(fidx)
		return true
	case "c":
		m.insertSearchDraft("c")
		m.syncScrollToCursor(fidx)
		return true
	case "backspace":
		m.clampSearchCaret()
		rs := []rune(m.searchDraft)
		if m.searchCaret > 0 {
			m.searchDraft = string(append(rs[:m.searchCaret-1], rs[m.searchCaret:]...))
			m.searchCaret--
		}
		return true
	case " ":
		m.insertSearchDraft(" ")
		return true
	case "left":
		m.clampSearchCaret()
		if m.searchCaret > 0 {
			m.searchCaret--
		}
		m.syncScrollToCursor(fidx)
		return true
	case "right":
		m.clampSearchCaret()
		rs := []rune(m.searchDraft)
		if m.searchCaret < len(rs) {
			m.searchCaret++
		}
		m.syncScrollToCursor(fidx)
		return true
	default:
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			var ins strings.Builder
			for _, r := range msg.Runes {
				if unicode.IsPrint(r) {
					ins.WriteRune(r)
				}
			}
			if ins.Len() > 0 {
				m.insertSearchDraft(ins.String())
				m.syncScrollToCursor(fidx)
			}
			return true
		}
		return false
	}
}

// tryBrowseKey handles navigation, selection, and search hit jumps.
// It returns false for keys that should fall through to filter-draft editing, search-buffer editing, or other handlers.
func (m *Model) tryBrowseKey(msg tea.KeyMsg, fidx []int, vh int) (bool, tea.Cmd) {
	switch msg.String() {
	case "shift+up":
		m.navVertical(fidx, -1, true)
		return true, nil
	case "shift+down":
		m.navVertical(fidx, +1, true)
		return true, nil
	case "shift+home":
		m.ensureSelAnchor()
		m.follow = false
		m.cursorIdx = 0
		m.syncScrollToCursor(fidx)
		return true, nil
	case "shift+end":
		m.ensureSelAnchor()
		m.follow = true
		if len(fidx) > 0 {
			m.cursorIdx = len(fidx) - 1
		}
		m.syncScrollToCursor(fidx)
		return true, nil
	case "shift+left":
		m.clearRangeSelection()
		if !m.lineWrap && m.colRuneOff > 0 {
			m.colRuneOff--
			m.setFollowFromTailAlignment(fidx)
		}
		return true, nil
	case "shift+right":
		m.clearRangeSelection()
		if !m.lineWrap {
			m.colRuneOff++
			m.setFollowFromTailAlignment(fidx)
		}
		return true, nil
	case "ctrl+n":
		m.clearRangeSelection()
		if m.searchBuf != "" {
			if m.filePartial && len(m.fileOffsets) > 0 {
				prev := m.cursorIdx
				m.gotoNextSearchHit(fidx)
				if m.cursorIdx != prev {
					return true, nil
				}
				return true, m.cmdStartLazySearch(+1, fidx)
			}
			m.gotoNextSearchHit(fidx)
		}
		return true, nil
	case "ctrl+p":
		m.clearRangeSelection()
		if m.searchBuf != "" {
			if m.filePartial && len(m.fileOffsets) > 0 {
				prev := m.cursorIdx
				m.gotoPrevSearchHit(fidx)
				if m.cursorIdx != prev {
					return true, nil
				}
				return true, m.cmdStartLazySearch(-1, fidx)
			}
			m.gotoPrevSearchHit(fidx)
		}
		return true, nil
	case "up":
		prev := m.cursorIdx
		m.navVertical(fidx, -1, false)
		return true, m.maybeFileLoadAfterNavUp(fidx, prev)
	case "down":
		prev := m.cursorIdx
		m.navVertical(fidx, +1, false)
		return true, m.maybeFileLoadAfterNavDown(fidx, prev)
	case "pgup", "ctrl+b":
		m.clearRangeSelection()
		m.follow = false
		m.pageUpTopAnchorFirst(fidx, vh)
		m.syncScrollToCursor(fidx)
		return true, m.maybeFileLoadAfterPageUp(fidx, vh)
	case "pgdown", "ctrl+f":
		prev := m.cursorIdx
		m.clearRangeSelection()
		m.follow = false
		m.cursorIdx += vh
		if len(fidx) == 0 {
			m.cursorIdx = 0
		} else if m.cursorIdx > len(fidx)-1 {
			m.cursorIdx = len(fidx) - 1
		}
		m.syncScrollToCursor(fidx)
		if m.lineWrap && len(fidx) > 0 && m.cursorIdx == len(fidx)-1 {
			m.pinWrapSegScrollToLogicalTail(fidx)
		}
		if m.tailAligned(fidx) {
			m.follow = true
		}
		cmd := m.maybeFileLoadAfterPageDown(fidx, prev, vh)
		return true, cmd
	case "home":
		m.clearRangeSelection()
		m.follow = false
		if m.filePartial && len(m.fileOffsets) > 0 {
			return true, m.cmdLoadFileWindowStartingAt(1)
		}
		m.cursorIdx = 0
		m.syncScrollToCursor(fidx)
		return true, nil
	case "end":
		m.clearRangeSelection()
		m.follow = true
		if m.filePartial && len(m.fileOffsets) > 0 && m.fileTotalLines > 0 {
			return true, m.cmdLoadFileWindowAroundBottom(int64(m.fileTotalLines), vh)
		}
		if len(fidx) > 0 {
			m.cursorIdx = len(fidx) - 1
		}
		m.syncScrollToCursor(fidx)
		return true, nil
	case "left":
		m.clearRangeSelection()
		if !m.lineWrap && m.colRuneOff > 0 {
			m.colRuneOff--
			m.setFollowFromTailAlignment(fidx)
		}
		return true, nil
	case "right":
		m.clearRangeSelection()
		if !m.lineWrap {
			m.colRuneOff++
			m.setFollowFromTailAlignment(fidx)
		}
		return true, nil
	default:
		return false, nil
	}
}

// isF1Key recognizes F1 from bubbletea (KeyF1 or string "f1"). Some terminals/IDEs only deliver a
// consistent Type; others may differ — checking both avoids missing the help binding.
func isF1Key(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyF1 || msg.String() == "f1"
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fidx := m.filteredIndices()
	vh := m.viewportH()
	// Clear previous one-shot feedback; handlers (e.g. `c` copy) may set a new message this tick.
	m.copyFlash = ""

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	if m.helpOpen {
		if isF1Key(msg) || msg.String() == "esc" {
			m.helpOpen = false
			m.helpFilterSyntax = false
		}
		// swallow keys while help is open (PRD §6.1)
		return m, nil
	}

	if m.searchScanConfirmOpen {
		switch msg.String() {
		case "y", "enter":
			m.searchScanConfirmOpen = false
			return m, m.cmdScanSearchInFile(m.searchScanDir, m.searchScanResumeSeq, m.searchScanScanned, true)
		case "n", "esc":
			m.searchScanConfirmOpen = false
			m.copyFlash = "search cancelled"
			return m, nil
		default:
			return m, nil
		}
	}

	// Full keymap: F1, or "?" on log list only (IDE/terminal may steal F1 or send an unmapped CSI).
	// Filter-syntax dialog: F1 from filter input only (PRD §5·§6.4).
	if isF1Key(msg) || (m.keyFocus() == FocusLogList && msg.String() == "?") {
		m.helpOpen = true
		m.helpFilterSyntax = isF1Key(msg) && m.keyFocus() == FocusFilterEditor
		return m, nil
	}

	if m.histKind != histNone {
		return m.handleHistKey(msg)
	}

	switch msg.String() {
	case "ctrl+w":
		m.lineWrap = !m.lineWrap
		if m.lineWrap {
			m.colRuneOff = 0
		}
		fidx = m.filteredIndices()
		m.clampSelectionToBuffer(fidx)
		m.syncScrollToCursor(fidx)
		return m, nil
	}

	// Esc: clear list selection (Shift range + Space picks) before compose- or list-specific Esc (PRD §6.5·§6.6).
	// Not used while FocusFilterEditor (filter Esc pops filter focus only).
	if m.keyFocus() != FocusFilterEditor && msg.String() == "esc" && m.hasListSelection() {
		m.clearAllSelection()
		return m, nil
	}

	switch m.keyFocus() {
	case FocusFilterEditor:
		return m.handleKeyFilterEditor(msg, fidx, vh)
	case FocusHighlightEditor:
		return m.handleKeyHighlightEditor(msg, fidx, vh)
	default:
		return m.handleKeyLogList(msg, fidx, vh)
	}
}

// gotoNextSearchHit / gotoPrevSearchHit: PRD §8.4 n/p match navigation within the filtered
// window. Both route through nextMatchIdxInFidx (Phase 3 SeqMatcher helper) so the ring-local
// scan shares its predicate with future lazy/pull-driven disk scans.
func (m *Model) gotoNextSearchHit(fidx []int) {
	if len(fidx) == 0 || strings.TrimSpace(m.searchBuf) == "" {
		return
	}
	if m.cursorIdx < 0 || m.cursorIdx >= len(fidx) {
		return
	}
	prev := m.cursorIdx
	curSeq := m.buf.At(fidx[m.cursorIdx]).Seq
	idx := m.nextMatchIdxInFidx(fidx, curSeq, +1, m.searchPredicate())
	if idx < 0 {
		return
	}
	m.follow = false
	m.cursorIdx = idx
	m.syncScrollToCursor(fidx)
	if m.lineWrap && m.cursorIdx != prev && m.cursorIdx == len(fidx)-1 {
		m.pinWrapSegScrollToLogicalTail(fidx)
	}
	if m.cursorIdx != prev && m.tailAligned(fidx) {
		m.follow = true
	}
}

func (m *Model) gotoPrevSearchHit(fidx []int) {
	if len(fidx) == 0 || strings.TrimSpace(m.searchBuf) == "" {
		return
	}
	if m.cursorIdx < 0 || m.cursorIdx >= len(fidx) {
		return
	}
	prev := m.cursorIdx
	curSeq := m.buf.At(fidx[m.cursorIdx]).Seq
	idx := m.nextMatchIdxInFidx(fidx, curSeq, -1, m.searchPredicate())
	if idx < 0 {
		return
	}
	m.follow = false
	m.cursorIdx = idx
	m.syncScrollToCursor(fidx)
	if m.lineWrap && m.cursorIdx != prev && m.cursorIdx == len(fidx)-1 {
		m.pinWrapSegScrollToLogicalTail(fidx)
	}
	if m.cursorIdx != prev && m.tailAligned(fidx) {
		m.follow = true
	}
}
