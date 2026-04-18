package ui

import (
	"fmt"
	"strings"

	"git.inpt.fr/42dottools/log/internal/userstate"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type histOverlayKind uint8

const (
	histNone histOverlayKind = iota
	histFilterPick
	histHighlightPick
)

func (m *Model) openHistFilter() {
	m.histKind = histFilterPick
	m.histItems = append([]string(nil), m.filterHistory...)
	m.histSel = 0
	m.histClampSel()
}

func (m *Model) openHistHighlight() {
	m.histKind = histHighlightPick
	m.histItems = append([]string(nil), m.highlightHistory...)
	m.histSel = 0
	m.histClampSel()
}

func (m *Model) histClampSel() {
	if len(m.histItems) == 0 {
		m.histSel = 0
		return
	}
	if m.histSel < 0 {
		m.histSel = 0
	}
	if m.histSel >= len(m.histItems) {
		m.histSel = len(m.histItems) - 1
	}
}

// handleHistKey consumes keys while a history overlay is open (PRD §6.4.1).
func (m *Model) handleHistKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.histKind = histNone
		m.histItems = nil
		return m, nil
	case "enter":
		if len(m.histItems) > 0 && m.histSel >= 0 && m.histSel < len(m.histItems) {
			picked := m.histItems[m.histSel]
			if m.histKind == histFilterPick {
				m.filterDraft = picked
				m.syncFilterCursorEnd()
			} else {
				m.searchDraft = picked
				m.searchCaret = len([]rune(m.searchDraft))
			}
		}
		m.histKind = histNone
		m.histItems = nil
		return m, nil
	case "up":
		if m.histSel > 0 {
			m.histSel--
		}
		m.histClampSel()
		return m, nil
	case "down":
		if len(m.histItems) > 0 && m.histSel < len(m.histItems)-1 {
			m.histSel++
		}
		m.histClampSel()
		return m, nil
	default:
		// PRD §6.4.1: swallow other keys while overlay is open.
		return m, nil
	}
}

func (m *Model) renderHistOverlay() string {
	title := "Filter history"
	if m.histKind == histHighlightPick {
		title = "Highlight history"
	}
	maxW := m.width - 4
	if maxW < 24 {
		maxW = 24
	}
	if maxW > 80 {
		maxW = 80
	}
	maxLines := m.viewportH() + 2
	if maxLines < 6 {
		maxLines = 6
	}
	if maxLines > 16 {
		maxLines = 16
	}

	items := m.histItems
	scroll := 0
	if len(items) > maxLines {
		scroll = m.histSel - maxLines/2
		if scroll < 0 {
			scroll = 0
		}
		if scroll > len(items)-maxLines {
			scroll = len(items) - maxLines
		}
	}
	end := scroll + maxLines
	if end > len(items) {
		end = len(items)
	}
	visible := items[scroll:end]

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
	b.WriteByte('\n')
	if len(items) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("(no saved entries)"))
	} else {
		for i, line := range visible {
			if i > 0 {
				b.WriteByte('\n')
			}
			idx := scroll + i
			lineVis := visibleByCells(line, maxW-4)
			prefix := "  "
			if idx == m.histSel {
				prefix = "> "
			}
			row := prefix + lineVis
			if idx == m.histSel {
				b.WriteString(lipgloss.NewStyle().Reverse(true).Render(row))
			} else {
				b.WriteString(row)
			}
		}
	}
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(fmt.Sprintf("↑↓ select · Enter apply to draft · Esc close · max %d", userstate.MaxHistoryEntries)))

	st := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		MaxWidth(maxW)
	return st.Render(b.String())
}
