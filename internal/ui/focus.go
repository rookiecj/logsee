package ui

import (
	"strings"

	"git.inpt.fr/42dottools/log/internal/filter"
	"git.inpt.fr/42dottools/log/internal/userstate"
	"github.com/charmbracelet/bubbletea"
)

// Focus is the non-modal TUI key-routing target: which surface consumes ordinary keys for this tick (PRD §6.0, §6.0.1).
// Modal layers (F1 help, §6.4.1 history overlay) are handled in handleKey before Focus is consulted.
type Focus int

const (
	// FocusLogList: cursor, list keys, Enter or ':' opens filter editor, / opens highlight editor (PRD §6.1–§6.3).
	FocusLogList Focus = iota
	// FocusFilterEditor: filter draft and §6.4 keys; shared §6.2 movement still affects the list.
	FocusFilterEditor
	// FocusHighlightEditor: search/highlight compose draft (PRD §6.5).
	FocusHighlightEditor
)

// keyFocus returns keyboard focus from persistent Model flags. On startup, !filterEdit && !searchCompose → FocusLogList.
func (m *Model) keyFocus() Focus {
	if m.filterEdit {
		return FocusFilterEditor
	}
	if m.searchCompose {
		return FocusHighlightEditor
	}
	return FocusLogList
}

func (m *Model) handleKeyFilterEditor(msg tea.KeyMsg, fidx []int, vh int) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left":
		m.clampFilterCursor()
		if m.filterCursor > 0 {
			m.filterCursor--
		}
		return m, nil
	case "right":
		m.clampFilterCursor()
		rs := []rune(m.filterDraft)
		if m.filterCursor < len(rs) {
			m.filterCursor++
		}
		return m, nil
	}
	if msg.String() == "down" {
		m.openHistFilter()
		return m, nil
	}
	if ok, bcmd := m.tryBrowseKey(msg, fidx, vh); ok {
		return m, bcmd
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.popFilterInputFocus()
		m.syncScrollToCursor(fidx)
		return m, nil
	case tea.KeyEnter:
		prog, err := filter.Parse(m.filterDraft)
		if err != nil {
			m.filterErr = err.Error()
			return m, nil
		}
		oldFidx := m.filteredIndices()
		ringRow := -1
		if len(oldFidx) > 0 && m.cursorIdx >= 0 && m.cursorIdx < len(oldFidx) {
			ringRow = oldFidx[m.cursorIdx]
		}
		m.prog = prog
		m.appliedFilter = strings.TrimSpace(m.filterDraft)
		m.filterErr = ""
		m.filterEdit = false
		m.searchCompose = false
		m.searchDraft = m.searchBuf
		m.clearAllSelection()
		if m.appliedFilter != "" {
			m.filterHistory = userstate.PushMRU(m.filterHistory, m.appliedFilter, userstate.MaxHistoryEntries)
		}
		m.persistState()
		fidx = m.filteredIndices()
		if m.follow && len(fidx) > 0 {
			m.cursorIdx = len(fidx) - 1
		} else {
			m.remapCursorPreservingRingRow(fidx, ringRow)
		}
		m.syncScrollToCursor(fidx)
		if m.filePartial && m.appliedFilter != "" && len(fidx) < m.viewportH() {
			m.filterTopupActive = true
			m.filterTopupDir = m.pickFilterTopupDirWhenUndersized()
			return m, m.cmdFindFilterTopupByDir()
		}
		m.filterTopupActive = false
		m.filterTopupDir = 0
		return m, nil
	case tea.KeyBackspace:
		rs := []rune(m.filterDraft)
		m.clampFilterCursor()
		if m.filterCursor > 0 {
			m.filterCursor--
			rs = append(rs[:m.filterCursor], rs[m.filterCursor+1:]...)
			m.filterDraft = string(rs)
		}
		return m, nil
	case tea.KeySpace:
		rs := []rune(m.filterDraft)
		m.clampFilterCursor()
		at := m.filterCursor
		rs = append(rs[:at], append([]rune{' '}, rs[at:]...)...)
		m.filterDraft = string(rs)
		m.filterCursor++
		return m, nil
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			rs := []rune(m.filterDraft)
			m.clampFilterCursor()
			at := m.filterCursor
			ins := []rune(string(msg.Runes))
			rs = append(rs[:at], append(ins, rs[at:]...)...)
			m.filterDraft = string(rs)
			m.filterCursor += len(ins)
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) handleKeyHighlightEditor(msg tea.KeyMsg, fidx []int, vh int) (tea.Model, tea.Cmd) {
	if msg.String() == "down" {
		m.openHistHighlight()
		return m, nil
	}
	if m.trySearchComposeKey(msg, fidx) {
		return m, nil
	}
	if ok, bcmd := m.tryBrowseKey(msg, fidx, vh); ok {
		return m, bcmd
	}
	return m, nil
}

// remapVimNavKeysForLogList maps h/j/k/l to arrows, G to End, n/p to Ctrl+n/Ctrl+p (match nav)
// so tryBrowseKey matches ↑↓←→/End/next-prev-hit. Only used on FocusLogList so filter/highlight
// compose can still type these runes (PRD §6.).
func remapVimNavKeysForLogList(msg tea.KeyMsg) tea.KeyMsg {
	switch msg.String() {
	case "k":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "j":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "h":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "l":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "G":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	default:
		return msg
	}
}

func (m *Model) handleKeyLogList(msg tea.KeyMsg, fidx []int, vh int) (tea.Model, tea.Cmd) {
	msg = remapVimNavKeysForLogList(msg)
	if ok, bcmd := m.tryBrowseKey(msg, fidx, vh); ok {
		return m, bcmd
	}
	if ok, bcmd := m.tryBookmarkKey(msg, fidx); ok {
		return m, bcmd
	}
	fidx = m.filteredIndices()
	if msg.String() == " " || msg.Type == tea.KeySpace {
		m.toggleSpacePickOnList(fidx)
		return m, nil
	}
	if msg.String() == "c" && len(fidx) > 0 {
		m.copyLogLines(fidx)
		return m, nil
	}
	switch msg.String() {
	case "enter", ":":
		m.clearAllSelection()
		m.searchCompose = false
		m.searchDraft = m.searchBuf
		m.searchCaret = len([]rune(m.searchBuf))
		m.filterEdit = true
		m.filterDraft = m.appliedFilter
		m.syncFilterCursorEnd()
		m.filterErr = ""
		return m, nil
	case "/":
		m.searchCompose = true
		m.searchDraft = m.searchBuf
		m.searchCaret = len([]rune(m.searchDraft))
		m.syncScrollToCursor(fidx)
		return m, nil
	case "esc":
		m.searchBuf = ""
		m.searchDraft = ""
		m.searchCompose = false
		m.searchCaret = 0
		return m, nil
	default:
		return m, nil
	}
}
