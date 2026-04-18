package ui

import (
	"fmt"
	"strings"

	"git.inpt.fr/42dottools/log/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) contentWidthForLog(w int) int {
	cw := w
	cw -= listPrefixDisplayWidth(m.noLineNumbers)
	if cw < 4 {
		cw = 4
	}
	return cw
}

func (m *Model) buildWrapSegs(fidx []int) []wrapSeg {
	w := m.width
	if w < 8 {
		w = 8
	}
	cw := m.contentWidthForLog(w)
	var segs []wrapSeg
	for fi := 0; fi < len(fidx); fi++ {
		rec := m.buf.At(fidx[fi])
		rs := []rune(rec.Text)
		for _, sp := range wrapLineRunes(rs, cw) {
			segs = append(segs, wrapSeg{Fi: fi, R0: sp[0], R1: sp[1]})
		}
	}
	return segs
}

func (m *Model) renderLog() string {
	fidx := m.filteredIndices()
	vh := m.viewportH()
	hi := lipgloss.NewStyle().Background(lipgloss.Color(DefaultHighlightBG)).Foreground(lipgloss.Color(DefaultHighlightFG))
	w := m.width
	if w < 8 {
		w = 8
	}
	if m.lineWrap {
		return m.renderLogWrapped(fidx, vh, hi, w)
	}
	L := len(fidx)
	padTop := 0
	if L <= vh {
		padTop = vh - L
	}
	var lines []string
	for row := 0; row < vh; row++ {
		var fi int
		var ok bool
		if L == 0 {
			lines = append(lines, strings.Repeat(" ", w))
			continue
		}
		if L <= vh {
			if row < padTop {
				lines = append(lines, strings.Repeat(" ", w))
				continue
			}
			fi = row - padTop
			ok = true
		} else {
			fi = m.scrollTop + row
			ok = fi < L
		}
		if !ok {
			lines = append(lines, strings.Repeat(" ", w))
			continue
		}
		rec := m.buf.At(fidx[fi])
		isSel := m.lineSelected(fi)
		pickOnly := m.linePickOnly(fi)
		showCursorBar := fi == m.cursorIdx
		line := m.formatLine(rec, hi, w, showCursorBar, isSel, pickOnly)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderLogWrapped(fidx []int, vh int, hi lipgloss.Style, w int) string {
	segs := m.buildWrapSegs(fidx)
	S := len(segs)
	padTop := 0
	if S <= vh {
		padTop = vh - S
	}
	var lines []string
	for row := 0; row < vh; row++ {
		var si int
		var ok bool
		if S == 0 {
			lines = append(lines, strings.Repeat(" ", w))
			continue
		}
		if S <= vh {
			if row < padTop {
				lines = append(lines, strings.Repeat(" ", w))
				continue
			}
			si = row - padTop
			ok = true
		} else {
			si = m.scrollSegTop + row
			ok = si < S
		}
		if !ok {
			lines = append(lines, strings.Repeat(" ", w))
			continue
		}
		seg := segs[si]
		rec := m.buf.At(fidx[seg.Fi])
		isSel := m.lineSelected(seg.Fi)
		pickOnly := m.linePickOnly(seg.Fi)
		showCursorBar := seg.Fi == m.cursorIdx
		line := m.formatWrapSeg(rec, seg, hi, w, showCursorBar, isSel, pickOnly)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) parsedHighlightNeedles() []HighlightNeedle {
	if m.searchBuf == m.hlNeedlesKey && m.hlNeedlesCached != nil {
		return m.hlNeedlesCached
	}
	n := ParseHighlightNeedles(m.searchBuf, m.highlightNames)
	m.hlNeedlesKey = m.searchBuf
	m.hlNeedlesCached = n
	return n
}

// searchHighlightVis styles the visible log slice for search hits. On the cursor row, uses
// per-segment reverse so nested ANSI does not drop invert after the first highlighted run.
func (m *Model) searchHighlightVis(plain string, hi lipgloss.Style, isCursor, isSelected bool) (vis string, baseSeg lipgloss.Style) {
	needles := m.parsedHighlightNeedles()
	if len(needles) == 0 {
		if !isCursor {
			return plain, lipgloss.Style{}
		}
		// No highlight query: still paint cursor row (full visible slice) with reverse / selection.
		if isSelected {
			baseSeg = lipgloss.NewStyle().Reverse(true).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
			return baseSeg.Render(plain), baseSeg
		}
		baseSeg = lipgloss.NewStyle().Reverse(true)
		return baseSeg.Render(plain), baseSeg
	}
	if !isCursor {
		return HighlightFromNeedles(plain, needles, false, hi), lipgloss.Style{}
	}
	if isSelected {
		baseSeg = lipgloss.NewStyle().Reverse(true).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
		defaultMatchSel := lipgloss.NewStyle().Reverse(true).Background(lipgloss.Color(DefaultHighlightBG)).Foreground(lipgloss.Color(DefaultHighlightFG))
		return HighlightWithReverseStylesSelected(plain, needles, false, baseSeg, defaultMatchSel), baseSeg
	}
	baseSeg = lipgloss.NewStyle().Reverse(true)
	matchSeg := baseSeg.Inherit(hi)
	return HighlightReverseFromNeedles(plain, needles, false, baseSeg, matchSeg), baseSeg
}

func (m *Model) formatWrapSeg(rec domain.Record, seg wrapSeg, hi lipgloss.Style, w int, isCursor, isSelected, pickOnly bool) string {
	rs := []rune(rec.Text)
	r0, r1 := seg.R0, seg.R1
	if r0 < 0 {
		r0 = 0
	}
	if r1 > len(rs) {
		r1 = len(rs)
	}
	if r1 < r0 {
		r1 = r0
	}
	piece := string(rs[r0:r1])
	contentW := w
	prefixCells := listPrefixDisplayWidth(m.noLineNumbers)
	contentW -= prefixCells
	if contentW < 4 {
		contentW = 4
	}
	plain := visibleByCells(piece, contentW)
	vis, baseSeg := m.searchHighlightVis(plain, hi, isCursor, isSelected)
	var prefix string
	if !m.noLineNumbers {
		if r0 == 0 {
			prefix = m.formatLinenoAndBookmarkPrefix(rec.Seq, isCursor)
		} else {
			prefix = strings.Repeat(" ", prefixCells)
		}
	} else {
		if r0 == 0 {
			prefix = m.formatNoLineNumberBookmarkPrefix(rec.Seq, isCursor)
		} else {
			prefix = strings.Repeat(" ", prefixCells)
		}
	}
	var line string
	switch {
	case isCursor && prefix != "" && r0 == 0:
		if !m.noLineNumbers {
			seqPlain := fmt.Sprintf("%6d ", rec.Seq)
			k := m.bookmarkSlotIndex(rec.Seq)
			if k == 0 {
				line = baseSeg.Render(seqPlain+"  ") + vis
			} else {
				ch := string(rune('0' + k))
				badge := bookmarkBadgeStyle(true).Render(ch)
				line = baseSeg.Render(seqPlain) + badge + baseSeg.Render(" ") + vis
			}
		} else {
			k := m.bookmarkSlotIndex(rec.Seq)
			if k == 0 {
				line = baseSeg.Render("  ") + vis
			} else {
				ch := string(rune('0' + k))
				badge := bookmarkBadgeStyle(true).Render(ch)
				line = baseSeg.Render(padDisplayWidth(badge+" ", 2)) + vis
			}
		}
	case isCursor && prefix != "":
		line = baseSeg.Render(prefix) + vis
	default:
		line = prefix + vis
	}
	pad := w - lipgloss.Width(line)
	if pad > 0 {
		if isCursor {
			line += baseSeg.Render(strings.Repeat(" ", pad))
		} else {
			line += strings.Repeat(" ", pad)
		}
	}
	st := lipgloss.NewStyle().MaxWidth(w)
	if isCursor {
		return st.Render(line)
	}
	if isSelected {
		if pickOnly {
			st = st.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("252"))
		} else {
			st = st.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
		}
	}
	return st.Render(line)
}

func (m *Model) formatLine(rec domain.Record, hi lipgloss.Style, w int, isCursor, isSelected, pickOnly bool) string {
	rs := []rune(rec.Text)
	col := m.colRuneOff
	if col > len(rs) {
		col = len(rs)
	}
	tail := string(rs[col:])
	contentW := w
	prefixCells := listPrefixDisplayWidth(m.noLineNumbers)
	contentW -= prefixCells
	if contentW < 4 {
		contentW = 4
	}
	plain := visibleByCells(tail, contentW)
	// Search highlight and match navigation use case-sensitive match (PRD §8.2); filter still uses m.ignoreCase.
	vis, baseSeg := m.searchHighlightVis(plain, hi, isCursor, isSelected)
	var prefix string
	if !m.noLineNumbers {
		prefix = m.formatLinenoAndBookmarkPrefix(rec.Seq, isCursor)
	} else {
		prefix = m.formatNoLineNumberBookmarkPrefix(rec.Seq, isCursor)
	}
	var line string
	switch {
	case isCursor && prefix != "" && !m.noLineNumbers:
		seqPlain := fmt.Sprintf("%6d ", rec.Seq)
		k := m.bookmarkSlotIndex(rec.Seq)
		if k == 0 {
			line = baseSeg.Render(seqPlain+"  ") + vis
		} else {
			ch := string(rune('0' + k))
			badge := bookmarkBadgeStyle(true).Render(ch)
			line = baseSeg.Render(seqPlain) + badge + baseSeg.Render(" ") + vis
		}
	case isCursor && prefix != "" && m.noLineNumbers:
		k := m.bookmarkSlotIndex(rec.Seq)
		if k == 0 {
			line = baseSeg.Render("  ") + vis
		} else {
			ch := string(rune('0' + k))
			badge := bookmarkBadgeStyle(true).Render(ch)
			line = baseSeg.Render(padDisplayWidth(badge+" ", 2)) + vis
		}
	default:
		line = prefix + vis
	}
	pad := w - lipgloss.Width(line)
	if pad > 0 {
		if isCursor {
			line += baseSeg.Render(strings.Repeat(" ", pad))
		} else {
			line += strings.Repeat(" ", pad)
		}
	}
	st := lipgloss.NewStyle().MaxWidth(w)
	if isCursor {
		return st.Render(line)
	}
	if isSelected {
		if pickOnly {
			st = st.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("252"))
		} else {
			st = st.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
		}
	}
	return st.Render(line)
}
