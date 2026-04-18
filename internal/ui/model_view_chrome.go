package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// View implements tea.Model.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.helpOpen {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderHelpDialog())
	}
	if m.searchScanConfirmOpen {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderSearchConfirmDialog())
	}
	if m.histKind != histNone {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderHistOverlay())
	}
	body := m.renderLog()
	filterRow := m.renderChromeLine(m.buildFilterPlain(), m.filterBarStyle())
	if m.filterEdit {
		filterRow = m.renderFilterEditRow()
	}
	return filterRow + "\n" +
		m.renderChromeLine(m.buildHighlightLine(), m.highlightBarStyle()) + "\n" +
		body + "\n" +
		m.renderStatusBar()
}

// renderHelpDialog is the F1 modal (PRD §5·§6.1): version line + mode-grouped keymap; Esc or F1 closes.
func (m *Model) renderHelpDialog() string {
	if m.helpFilterSyntax {
		return RenderFilterSyntaxHelpDialog(m.version, m.width)
	}
	return RenderHelpDialog(m.version, m.width)
}

func (m *Model) renderSearchConfirmDialog() string {
	body := "Highlight next/prev scan exceeded 100MiB.\nContinue scanning in the same direction?\n\nEnter/Y: continue   Esc/N: cancel"
	boxW := m.width * 2 / 3
	if boxW < 48 {
		boxW = 48
	}
	if boxW > 96 {
		boxW = 96
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Render("Search Confirm")
	msg := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(body)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(boxW).
		MaxWidth(boxW).
		Render(title + "\n\n" + msg)
	return box
}

// buildStatusBaseText is the status line without clipboard toast or selection suffix.
func (m *Model) buildStatusBaseText() string {
	fidx := m.filteredIndices()
	in := "in:" + truncateStatusPath(m.inputSource, 28)
	if m.stdinClosed {
		in += ":eof"
	} else {
		in += ":read"
	}
	follow := "follow:on"
	if !m.follow {
		follow = "follow:off"
	}
	out := "out:-"
	if m.store != nil {
		out = "out:" + truncateStatusPath(m.outPath, 36)
	}
	wrap := "wrap:off"
	if m.lineWrap {
		wrap = "wrap:on"
	}
	parts := []string{
		fmt.Sprintf("lines:%d/%d", len(fidx), m.statusLineTotal()),
		m.statusTypeText(),
		follow,
		in,
		wrap,
		out,
	}
	return strings.Join(parts, "  ")
}

func (m *Model) statusTypeText() string {
	switch m.logTypeKind {
	case LogTypeAuto:
		if !m.logFormatResolved {
			return "type:auto~"
		}
		return "type:" + formatShortName(m.effectiveLogFmt)
	case LogTypePlain:
		return "type:plain"
	case LogTypeADB:
		return "type:adb"
	default:
		return "type:?"
	}
}

func (m *Model) buildStatusSelText() string {
	var parts []string
	if m.selAnchor >= 0 {
		lo, hi, _ := m.selectionRange()
		parts = append(parts, fmt.Sprintf("sel:%d-%d", lo+1, hi+1))
	}
	if n := len(m.picked); n > 0 {
		parts = append(parts, fmt.Sprintf("picked:%d", n))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}

func (m *Model) buildStatusLine() string {
	s := m.buildStatusBaseText()
	if m.copyFlash != "" {
		s += "  " + m.copyFlash
	}
	s += m.buildStatusSelText()
	return s
}

// statusBaseStyledBlock renders lines / follow / in / wrap / out with on/off value coloring (PRD §5).
func (m *Model) statusBaseStyledBlock() string {
	st := m.statusBarStyle()
	onSt := m.statusValueOnStyle()
	offSt := m.statusValueOffStyle()
	fidx := m.filteredIndices()
	lines := st.Render(fmt.Sprintf("lines:%d/%d", len(fidx), m.statusLineTotal()))
	typePart := st.Render(m.statusTypeText())
	in := "in:" + truncateStatusPath(m.inputSource, 28)
	if m.stdinClosed {
		in += ":eof"
	} else {
		in += ":read"
	}
	inPart := st.Render(in)
	var followPart string
	if m.follow {
		followPart = st.Render("follow:") + onSt.Render("on")
	} else {
		followPart = st.Render("follow:") + offSt.Render("off")
	}
	out := "out:-"
	if m.store != nil {
		out = "out:" + truncateStatusPath(m.outPath, 36)
	}
	outPart := st.Render(out)
	var wrapPart string
	if m.lineWrap {
		wrapPart = st.Render("wrap:") + onSt.Render("on")
	} else {
		wrapPart = st.Render("wrap:") + offSt.Render("off")
	}
	sp := st.Render("  ")
	return lipgloss.JoinHorizontal(lipgloss.Top, lines, sp, typePart, sp, followPart, sp, inPart, sp, wrapPart, sp, outPart)
}

// copyToastStyle highlights clipboard feedback on the status row for one tick after copy.
func (m *Model) copyToastStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28"))
}

// renderStatusBar draws the bottom chrome; when copyFlash is set, that segment uses copyToastStyle for visibility.
func (m *Model) renderStatusBar() string {
	w := m.width
	if w < 8 {
		w = 8
	}
	sel := m.buildStatusSelText()
	if m.copyFlash == "" {
		norm := m.statusBarStyle()
		row := lipgloss.JoinHorizontal(lipgloss.Top, m.statusBaseStyledBlock(), norm.Render(sel))
		return lipgloss.Place(w, 1, lipgloss.Left, lipgloss.Top, row)
	}
	flash := "  " + m.copyFlash
	const minBase = 10
	swFlash := runewidth.StringWidth(flash)
	swSel := runewidth.StringWidth(sel)
	// Reserve space for toast (up to ~40% width) and selection; remainder for base fields.
	maxFlash := w * 2 / 5
	if maxFlash < 14 {
		maxFlash = 14
	}
	if maxFlash > w-minBase-2 {
		maxFlash = w - minBase - 2
		if maxFlash < 8 {
			maxFlash = 8
		}
	}
	wF := swFlash
	if wF > maxFlash {
		wF = maxFlash
	}
	wS := swSel
	if wS > w-minBase-wF {
		wS = w - minBase - wF
		if wS < 0 {
			wS = 0
		}
	}
	wB := w - wF - wS
	flashVis := flash
	if wB < minBase {
		wB = minBase
		wF = w - wB - wS
		if wF < 8 {
			wF = 8
		}
		flashVis = visibleByCells(flash, wF)
		wF = runewidth.StringWidth(flashVis)
		wB = w - wF - wS
	} else if runewidth.StringWidth(flashVis) > wF {
		flashVis = visibleByCells(flash, wF)
		wF = runewidth.StringWidth(flashVis)
		wB = w - wF - wS
	}
	selVis := visibleByCells(sel, wS)
	flashPad := padDisplayWidth(flashVis, wF)
	selPad := padDisplayWidth(selVis, wS)

	norm := m.statusBarStyle()
	leftBlock := m.statusBaseStyledBlock()
	left := lipgloss.Place(wB, 1, lipgloss.Left, lipgloss.Top, leftBlock)
	mid := m.copyToastStyle().Render(flashPad)
	right := norm.Render(selPad)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	return lipgloss.Place(w, 1, lipgloss.Left, lipgloss.Top, row)
}

func (m *Model) buildFilterPlain() string {
	var b strings.Builder
	b.WriteString("FILTER  │  ")
	if m.filterEdit {
		b.WriteString("> ")
		b.WriteString(m.filterDraft)
	} else {
		if strings.TrimSpace(m.appliedFilter) == "" {
			b.WriteString("∅")
		} else {
			b.WriteString(m.appliedFilter)
		}
		b.WriteString("  │  Enter/:=filter edit")
	}
	if m.filterErr != "" {
		b.WriteString("  │  err: ")
		b.WriteString(m.filterErr)
	}
	return b.String()
}

// renderFilterEditRow draws the filter chrome row with a reversed caret at filterCursor (rune index).
func (m *Model) renderFilterEditRow() string {
	w := m.width
	if w < 8 {
		w = 8
	}
	m.clampFilterCursor()
	st := m.filterBarStyle()
	rs := []rune(m.filterDraft)
	c := m.filterCursor
	prefix := "FILTER  │  > "
	left := string(rs[:c])
	var midRune rune = '_'
	if c < len(rs) {
		midRune = rs[c]
	}
	right := ""
	if c < len(rs) {
		right = string(rs[c+1:])
	}
	errPart := ""
	if m.filterErr != "" {
		errPart = "  │  err: " + m.filterErr
	}
	plain := prefix + left + string(midRune) + right + errPart
	prefRunes := visiblePrefixRunes(plain, w)
	caretRuneIdx := len([]rune(prefix + left))
	s := string(prefRunes)
	sw := runewidth.StringWidth(s)
	if sw < w {
		s += strings.Repeat(" ", w-sw)
	}
	if caretRuneIdx >= len(prefRunes) {
		return st.Width(w).MaxWidth(w).Render(s)
	}
	before := string(prefRunes[:caretRuneIdx])
	caretChar := string(prefRunes[caretRuneIdx])
	after := string(prefRunes[caretRuneIdx+1:])
	row := lipgloss.JoinHorizontal(lipgloss.Top,
		st.Render(before),
		st.Reverse(true).Render(caretChar),
		st.Render(after),
	)
	jw := lipgloss.Width(row)
	if jw < w {
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, st.Render(strings.Repeat(" ", w-jw)))
	}
	return row
}

func (m *Model) buildHighlightLine() string {
	if m.filterEdit {
		return "Enter=apply filter  Esc=revert draft  Backspace=delete  ←→=move in draft"
	}
	if m.searchCompose {
		m.clampSearchCaret()
		var b strings.Builder
		b.WriteString("HIGHLIGHT  │  > ")
		rs := []rune(m.searchDraft)
		if m.searchCaret >= len(rs) {
			b.WriteString(m.searchDraft)
			b.WriteByte('_')
		} else {
			b.WriteString(string(rs[:m.searchCaret]))
			b.WriteByte('|')
			b.WriteString(string(rs[m.searchCaret:]))
		}
		b.WriteString("  │  Enter=commit  Esc=cancel")
		if strings.TrimSpace(m.searchBuf) != "" {
			b.WriteString("  n/p next/prev (no wrap)")
		}
		b.WriteString("  ctrl+w wrap  shift+sel  space=pick  c=range|picks|line")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("HIGHLIGHT  │  ")
	if strings.TrimSpace(m.searchBuf) == "" {
		b.WriteString("∅")
	} else {
		b.WriteString(m.searchBuf)
	}
	b.WriteString("  │  /=highlight edit")
	if strings.TrimSpace(m.searchBuf) != "" {
		b.WriteString("  n/p next/prev (no wrap)  Ctrl+n/p in filter")
	}
	b.WriteString("  ctrl+w wrap  shift+range  space=pick  c=range|picks|cursor  Esc=stack")
	return b.String()
}

func (m *Model) filterBarStyle() lipgloss.Style {
	st := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("254"))
	if m.filterEdit {
		return st.Background(lipgloss.Color("25"))
	}
	return st.Background(lipgloss.Color("236"))
}

// highlightBarStyle mirrors filterBarStyle: strong bar when editing (compose), same palette as applied filter when committed, muted when empty.
func (m *Model) highlightBarStyle() lipgloss.Style {
	st := lipgloss.NewStyle().Bold(true)
	if m.searchCompose {
		return st.Foreground(lipgloss.Color("254")).Background(lipgloss.Color("25"))
	}
	if strings.TrimSpace(m.searchBuf) != "" {
		return st.Foreground(lipgloss.Color("254")).Background(lipgloss.Color("236"))
	}
	return st.Foreground(lipgloss.Color("247")).Background(lipgloss.Color("235"))
}

func (m *Model) statusBarStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color("233")).Foreground(lipgloss.Color("245"))
}

// statusValueOnStyle / statusValueOffStyle: status bar binary flags (PRD §5).
func (m *Model) statusValueOnStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color("233")).Foreground(lipgloss.Color("120")).Bold(true)
}

func (m *Model) statusValueOffStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color("233")).Foreground(lipgloss.Color("240")).Faint(true)
}

func (m *Model) renderChromeLine(plain string, st lipgloss.Style) string {
	w := m.width
	if w < 8 {
		w = 8
	}
	trunc := visibleByCells(plain, w)
	sw := runewidth.StringWidth(trunc)
	if sw < w {
		trunc += strings.Repeat(" ", w-sw)
	}
	return st.Width(w).MaxWidth(w).Render(trunc)
}
