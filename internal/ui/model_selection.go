package ui

import (
	"fmt"
	"sort"
	"strings"

	"git.inpt.fr/42dottools/log/internal/clipboard"
)

func (m *Model) clearRangeSelection() {
	m.selAnchor = -1
}

func (m *Model) clearSpacePicks() {
	m.picked = make(map[int]struct{})
}

func (m *Model) clearAllSelection() {
	m.clearRangeSelection()
	m.clearSpacePicks()
}

// hasListSelection is true when Shift range is active or at least one Space-picked line exists (PRD §2·§8.6).
func (m *Model) hasListSelection() bool {
	if m.selAnchor >= 0 {
		return true
	}
	return len(m.picked) > 0
}

func (m *Model) ensureSelAnchor() {
	if m.selAnchor < 0 {
		m.selAnchor = m.cursorIdx
	}
}

// selectionRange returns inclusive [lo, hi] in filtered-index space, or ok=false if nothing selected.
func (m *Model) selectionRange() (lo, hi int, ok bool) {
	if m.selAnchor < 0 {
		return 0, 0, false
	}
	lo, hi = m.selAnchor, m.cursorIdx
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, true
}

func (m *Model) lineInRange(fi int) bool {
	lo, hi, ok := m.selectionRange()
	if !ok {
		return false
	}
	return fi >= lo && fi <= hi
}

func (m *Model) linePicked(fi int) bool {
	_, ok := m.picked[fi]
	return ok
}

func (m *Model) lineSelected(fi int) bool {
	return m.lineInRange(fi) || m.linePicked(fi)
}

func (m *Model) linePickOnly(fi int) bool {
	return m.linePicked(fi) && !m.lineInRange(fi)
}

func (m *Model) clampSelectionToBuffer(fidx []int) {
	L := len(fidx)
	if L == 0 {
		m.clearAllSelection()
		m.cursorIdx = 0
		return
	}
	if m.selAnchor >= L {
		m.clearRangeSelection()
	}
	m.clampPickedToListLen(L)
	m.clampCursor(fidx)
}

func (m *Model) clampPickedToListLen(L int) {
	for fi := range m.picked {
		if fi < 0 || fi >= L {
			delete(m.picked, fi)
		}
	}
}

func (m *Model) navVertical(fidx []int, dir int, extend bool) {
	if extend {
		m.ensureSelAnchor()
		m.follow = false
		if dir < 0 {
			if m.cursorIdx > 0 {
				m.cursorIdx--
			}
		} else if dir > 0 {
			if m.cursorIdx < len(fidx)-1 {
				m.cursorIdx++
			}
		}
		m.syncScrollToCursor(fidx)
		return
	}
	m.clearRangeSelection()
	prevFollow := m.follow
	prevIdx := m.cursorIdx
	m.follow = false
	if dir < 0 {
		if m.cursorIdx > 0 {
			m.cursorIdx--
		}
	} else if dir > 0 {
		if m.cursorIdx < len(fidx)-1 {
			m.cursorIdx++
		}
	}
	m.syncScrollToCursor(fidx)
	if m.lineWrap && m.cursorIdx != prevIdx && len(fidx) > 0 && m.cursorIdx == len(fidx)-1 {
		m.pinWrapSegScrollToLogicalTail(fidx)
	}
	if m.cursorIdx != prevIdx && m.tailAligned(fidx) {
		m.follow = true
		return
	}
	// Last line + down with no movement: do not clear follow (stay tail-following).
	if m.cursorIdx == prevIdx && dir > 0 && prevFollow && len(fidx) > 0 && prevIdx == len(fidx)-1 && m.tailAligned(fidx) {
		m.follow = true
	}
}

// buildCopyText returns log text for clipboard: union of Shift range (if any) and Space picks (if any), each line once, sorted by filtered index; else cursor line (PRD §8.6).
func (m *Model) buildCopyText(fidx []int) (text string, nLines int, ok bool) {
	if len(fidx) == 0 {
		return "", 0, false
	}
	m.clampCursor(fidx)
	union := make(map[int]struct{})
	if m.selAnchor >= 0 {
		lo, hi, has := m.selectionRange()
		if has {
			for i := lo; i <= hi; i++ {
				if i >= 0 && i < len(fidx) {
					union[i] = struct{}{}
				}
			}
		}
	}
	for fi := range m.picked {
		if fi >= 0 && fi < len(fidx) {
			union[fi] = struct{}{}
		}
	}
	if len(union) > 0 {
		indices := make([]int, 0, len(union))
		for fi := range union {
			indices = append(indices, fi)
		}
		sort.Ints(indices)
		var b strings.Builder
		for j, i := range indices {
			if j > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(m.buf.At(fidx[i]).Text)
		}
		return b.String(), len(indices), true
	}
	var b strings.Builder
	i := m.cursorIdx
	if i < 0 || i >= len(fidx) {
		return "", 0, false
	}
	b.WriteString(m.buf.At(fidx[i]).Text)
	return b.String(), 1, true
}

func (m *Model) copyLogLines(fidx []int) {
	text, n, ok := m.buildCopyText(fidx)
	if !ok {
		m.copyFlash = "no lines"
		return
	}
	if err := clipboard.SetText(text); err != nil {
		m.copyFlash = err.Error()
		return
	}
	if n == 1 {
		m.copyFlash = "1 line copied"
		return
	}
	m.copyFlash = fmt.Sprintf("%d lines copied", n)
}

// toggleSpacePickOnList: Space on list (not search compose). With Shift range active, adds every line in the range to Space picks and clears the range; otherwise toggles pick on the cursor line (PRD §8.6).
func (m *Model) toggleSpacePickOnList(fidx []int) {
	if len(fidx) == 0 {
		return
	}
	m.clampCursor(fidx)
	if m.selAnchor >= 0 {
		lo, hi, ok := m.selectionRange()
		if ok {
			for i := lo; i <= hi; i++ {
				m.picked[i] = struct{}{}
			}
		}
		m.clearRangeSelection()
		return
	}
	fi := m.cursorIdx
	if _, exists := m.picked[fi]; exists {
		delete(m.picked, fi)
		return
	}
	m.picked[fi] = struct{}{}
}
