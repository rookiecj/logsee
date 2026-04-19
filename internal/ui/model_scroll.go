package ui

import (
	"strings"

	"git.inpt.fr/42dottools/log/internal/filter"
)

func (m *Model) viewportH() int {
	v := m.height - topChromeLines - bottomChromeLines
	if v < 1 {
		return 1
	}
	return v
}

func (m *Model) maxScrollTop(fidx []int) int {
	vh := m.viewportH()
	if len(fidx) == 0 {
		return 0
	}
	t := len(fidx) - vh
	if t < 0 {
		return 0
	}
	return t
}

func (m *Model) effectiveLogFormat() filter.LogFormat {
	return m.effectiveLogFmt
}

// maybeResolveAutoFormat runs one-shot auto detection when enough non-empty lines exist or stdin closed.
func (m *Model) maybeResolveAutoFormat() {
	if m.logTypeKind != LogTypeAuto || m.logFormatResolved {
		return
	}
	nEmpty := m.countNonEmptyOldest(m.logProbeLines)
	if !m.stdinClosed && nEmpty < m.logProbeLines {
		return
	}
	if m.buf == nil || m.buf.Len() == 0 {
		if m.stdinClosed {
			m.effectiveLogFmt = filter.FormatPlain
			m.logFormatResolved = true
		}
		return
	}
	lines := m.oldestRingLines()
	detected := filter.DetectLogFormatN(lines, m.logProbeLines)
	m.effectiveLogFmt = filter.EffectiveFormatFromDetect(detected)
	m.logFormatResolved = true
}

func (m *Model) countNonEmptyOldest(maxCount int) int {
	if m.buf == nil || maxCount < 1 {
		return 0
	}
	var n int
	for i := 0; i < m.buf.Len(); i++ {
		if strings.TrimSpace(m.buf.At(i).Text) != "" {
			n++
			if n >= maxCount {
				return n
			}
		}
	}
	return n
}

func (m *Model) oldestRingLines() []string {
	if m.buf == nil {
		return nil
	}
	n := m.buf.Len()
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = m.buf.At(i).Text
	}
	return out
}

// filteredIndices is PRD §8.0 layer 1: ring row indices that pass prog
// (all rows when prog is empty). When m.anomalyOnly is set (A toggle),
// lines without a classifier finding are additionally dropped. The
// findings map feeds MatchContext so `anomaly:*` tags resolve on the
// same pass.
func (m *Model) filteredIndices() []int {
	if m.buf == nil {
		return nil
	}
	fmt := m.effectiveLogFormat()
	n := m.buf.Len()
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		rec := m.buf.At(i)
		ctx := filter.MatchContext{Seq: rec.Seq}
		if m.findings != nil {
			if k, ok := m.findings[rec.Seq]; ok {
				ctx.Finding = k.String()
			}
		}
		if !filter.MatchWithContext(rec.Text, ctx, m.prog, m.ignoreCase, fmt) {
			continue
		}
		if m.anomalyOnly && ctx.Finding == "" {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (m *Model) clampCursor(fidx []int) {
	L := len(fidx)
	if L == 0 {
		m.cursorIdx = 0
		return
	}
	if m.cursorIdx >= L {
		m.cursorIdx = L - 1
	}
	if m.cursorIdx < 0 {
		m.cursorIdx = 0
	}
}

// remapCursorPreservingRingRow sets cursorIdx to the index in fidx that references ringRow, or clamps if absent.
func (m *Model) remapCursorPreservingRingRow(fidx []int, ringRow int) {
	if ringRow < 0 || len(fidx) == 0 {
		m.clampCursor(fidx)
		return
	}
	for i, ri := range fidx {
		if ri == ringRow {
			m.cursorIdx = i
			return
		}
	}
	m.clampCursor(fidx)
}

// remapCursorPreservingFilteredSeq sets cursorIdx to the filtered index whose record has seq.
// Returns true when seq was found (cursor updated).
func (m *Model) remapCursorPreservingFilteredSeq(fidx []int, seq int64) bool {
	if seq < 1 || len(fidx) == 0 {
		return false
	}
	for i, ri := range fidx {
		if m.buf.At(ri).Seq == seq {
			m.cursorIdx = i
			return true
		}
	}
	return false
}

// syncSeqFromIdx writes cursorSeq / viewTopSeq from the current cursorIdx / scrollTop + fidx.
// Call after any mutation of cursorIdx or scrollTop so the seq anchors stay current.
// Empty fidx clears anchors (stdin truncation etc.).
func (m *Model) syncSeqFromIdx(fidx []int) {
	if m.buf == nil || len(fidx) == 0 {
		m.cursorSeq = 0
		m.viewTopSeq = 0
		return
	}
	bufLen := m.buf.Len()
	ci := m.cursorIdx
	if ci < 0 {
		ci = 0
	}
	if ci >= len(fidx) {
		ci = len(fidx) - 1
	}
	if ri := fidx[ci]; ri >= 0 && ri < bufLen {
		m.cursorSeq = m.buf.At(ri).Seq
	}
	st := m.scrollTop
	if st < 0 {
		st = 0
	}
	if st >= len(fidx) {
		st = len(fidx) - 1
	}
	if ri := fidx[st]; ri >= 0 && ri < bufLen {
		m.viewTopSeq = m.buf.At(ri).Seq
	}
}

// findSeqInFidx returns the fidx index whose record has Seq == seq, or -1 if absent.
func (m *Model) findSeqInFidx(fidx []int, seq int64) int {
	if seq <= 0 || m.buf == nil {
		return -1
	}
	bufLen := m.buf.Len()
	for i, ri := range fidx {
		if ri < 0 || ri >= bufLen {
			continue
		}
		if m.buf.At(ri).Seq == seq {
			return i
		}
	}
	return -1
}

// syncIdxFromSeq rebuilds cursorIdx / scrollTop from the seq anchors (primary) + fidx.
// fallbackCursorRow is the target screen row (0..vh-1) when viewTopSeq is missing from fidx;
// pass -1 to fall back to minimal-change syncScrollToCursor behavior.
//
// Phase 1 (docs/plans/seq-coord-pull-window-plan.md): this makes ring.ReplaceRecords safe —
// the seq anchors are invariant across buffer replace, so the cursor's viewport row is deterministic.
func (m *Model) syncIdxFromSeq(fidx []int, fallbackCursorRow int) {
	L := len(fidx)
	if L == 0 {
		m.cursorIdx = 0
		m.scrollTop = 0
		m.scrollSegTop = 0
		return
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	// Cursor index: from cursorSeq, else clamp.
	if idx := m.findSeqInFidx(fidx, m.cursorSeq); idx >= 0 {
		m.cursorIdx = idx
	} else {
		m.clampCursor(fidx)
	}
	// Short list: scrollTop irrelevant (top-padded render path).
	if L <= vh {
		m.scrollTop = 0
		if m.lineWrap {
			m.syncScrollSegToCursor(fidx)
		}
		return
	}
	maxTop := L - vh
	// viewTopSeq hit: honor it, but clamp so cursor stays visible.
	if idx := m.findSeqInFidx(fidx, m.viewTopSeq); idx >= 0 {
		st := idx
		if m.cursorIdx < st {
			st = m.cursorIdx
		} else if m.cursorIdx >= st+vh {
			st = m.cursorIdx - vh + 1
		}
		if st < 0 {
			st = 0
		}
		if st > maxTop {
			st = maxTop
		}
		m.scrollTop = st
		if m.lineWrap {
			m.syncScrollSegToCursor(fidx)
		}
		return
	}
	// viewTopSeq miss: place cursor at fallbackCursorRow, else minimal-change from current scrollTop.
	if fallbackCursorRow >= 0 {
		row := fallbackCursorRow
		if row > vh-1 {
			row = vh - 1
		}
		st := m.cursorIdx - row
		if st < 0 {
			st = 0
		}
		if st > maxTop {
			st = maxTop
		}
		m.scrollTop = st
	} else {
		if m.cursorIdx < m.scrollTop {
			m.scrollTop = m.cursorIdx
		} else if m.cursorIdx >= m.scrollTop+vh {
			m.scrollTop = m.cursorIdx - vh + 1
		}
		if m.scrollTop < 0 {
			m.scrollTop = 0
		}
		if m.scrollTop > maxTop {
			m.scrollTop = maxTop
		}
	}
	if m.lineWrap {
		m.syncScrollSegToCursor(fidx)
	}
}

// syncScrollToCursor keeps cursorIdx visible with minimal scroll (PRD §8.5): only when the cursor
// would leave the top or bottom viewport row does scrollTop change by the minimum amount.
//
// Postcondition: cursorSeq / viewTopSeq are refreshed from the new cursorIdx / scrollTop so they
// stay the authoritative view anchors across any subsequent Ring.ReplaceRecords (Phase 1 seq coord).
func (m *Model) syncScrollToCursor(fidx []int) {
	m.clampCursor(fidx)
	if m.lineWrap {
		m.syncScrollSegToCursor(fidx)
		m.syncSeqFromIdx(fidx)
		return
	}
	m.scrollSegTop = 0
	L := len(fidx)
	vh := m.viewportH()
	if L == 0 {
		m.scrollTop = 0
		m.syncSeqFromIdx(fidx)
		return
	}
	if L <= vh {
		m.scrollTop = 0
		m.syncSeqFromIdx(fidx)
		return
	}
	maxTop := L - vh
	if m.cursorIdx < m.scrollTop {
		m.scrollTop = m.cursorIdx
	} else if m.cursorIdx >= m.scrollTop+vh {
		m.scrollTop = m.cursorIdx - vh + 1
	}
	if m.scrollTop > maxTop {
		m.scrollTop = maxTop
	}
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
	if m.cursorIdx < m.scrollTop {
		m.scrollTop = m.cursorIdx
	} else if m.cursorIdx >= m.scrollTop+vh {
		m.scrollTop = m.cursorIdx - vh + 1
		if m.scrollTop < 0 {
			m.scrollTop = 0
		}
	}
	m.syncSeqFromIdx(fidx)
}

func (m *Model) syncScrollSegToCursor(fidx []int) {
	segs := m.buildWrapSegs(fidx)
	S := len(segs)
	vh := m.viewportH()
	if S == 0 {
		m.scrollSegTop = 0
		return
	}
	if S <= vh {
		m.scrollSegTop = 0
		return
	}
	maxTop := S - vh
	firstSeg, lastSeg := -1, -1
	for i, sg := range segs {
		if sg.Fi == m.cursorIdx {
			if firstSeg < 0 {
				firstSeg = i
			}
			lastSeg = i
		}
	}
	if firstSeg < 0 || lastSeg < 0 {
		m.scrollSegTop = 0
		return
	}
	if lastSeg < m.scrollSegTop {
		m.scrollSegTop = lastSeg - vh + 1
	} else if firstSeg >= m.scrollSegTop+vh {
		m.scrollSegTop = firstSeg - vh + 1
	}
	if m.scrollSegTop < 0 {
		m.scrollSegTop = 0
	}
	if m.scrollSegTop > maxTop {
		m.scrollSegTop = maxTop
	}
	if lastSeg < m.scrollSegTop {
		m.scrollSegTop = lastSeg - vh + 1
		if m.scrollSegTop < 0 {
			m.scrollSegTop = 0
		}
	} else if firstSeg >= m.scrollSegTop+vh {
		m.scrollSegTop = firstSeg - vh + 1
		if m.scrollSegTop < 0 {
			m.scrollSegTop = 0
		}
	}
	if m.scrollSegTop > maxTop {
		m.scrollSegTop = maxTop
	}
	// follow on + 마지막 논리 줄: 시각 꼬리(S>vh이면 scrollSegTop=S-vh)를 보여야 tailAligned·신규 줄 follow가 성립(§8.5).
	if m.follow && len(fidx) > 0 && m.cursorIdx == len(fidx)-1 && S > vh {
		m.scrollSegTop = maxTop
	}
}

// pinWrapSegScrollToLogicalTail sets scrollSegTop so the last visual row is the buffer tail (wrap on, S>vh).
// Used when moving to the last logical line while follow is still off so tailAligned can observe 꼬리 정렬.
func (m *Model) pinWrapSegScrollToLogicalTail(fidx []int) {
	if !m.lineWrap {
		return
	}
	segs := m.buildWrapSegs(fidx)
	S := len(segs)
	vh := m.viewportH()
	if S == 0 {
		m.scrollSegTop = 0
		return
	}
	if S <= vh {
		m.scrollSegTop = 0
		return
	}
	m.scrollSegTop = S - vh
}

// tailAligned is true when the view shows the logical tail and the cursor sits on the last filtered line.
func (m *Model) tailAligned(fidx []int) bool {
	L := len(fidx)
	if L == 0 {
		return true
	}
	if m.cursorIdx != L-1 {
		return false
	}
	vh := m.viewportH()
	if m.lineWrap {
		segs := m.buildWrapSegs(fidx)
		S := len(segs)
		if S <= vh {
			return true
		}
		return m.scrollSegTop == S-vh
	}
	if L <= vh {
		return true
	}
	return m.scrollTop == L-vh
}

// setFollowFromTailAlignment sets follow when the view is tail-aligned (PRD §8.5: horizontal scroll restores follow at tail).
func (m *Model) setFollowFromTailAlignment(fidx []int) {
	m.follow = m.tailAligned(fidx)
}

// viewportTopLogicalIndex returns the logical index of the topmost visible log line.
// For short lists (with top padding), this is the first logical row.
func (m *Model) viewportTopLogicalIndex(fidx []int) int {
	L := len(fidx)
	if L == 0 {
		return 0
	}
	if !m.lineWrap {
		vh := m.viewportH()
		if L <= vh {
			return 0
		}
		top := m.scrollTop
		if top < 0 {
			return 0
		}
		if top > L-1 {
			return L - 1
		}
		return top
	}
	segs := m.buildWrapSegs(fidx)
	S := len(segs)
	vh := m.viewportH()
	if S == 0 || S <= vh {
		return 0
	}
	topSeg := m.scrollSegTop
	if topSeg < 0 {
		topSeg = 0
	}
	if topSeg > S-1 {
		topSeg = S - 1
	}
	return segs[topSeg].Fi
}

func (m *Model) pageUpTopAnchorFirst(fidx []int, vh int) {
	L := len(fidx)
	if L == 0 {
		m.cursorIdx = 0
		return
	}
	top := m.viewportTopLogicalIndex(fidx)
	if m.cursorIdx != top {
		m.cursorIdx = top
		return
	}
	m.cursorIdx -= vh
	if m.cursorIdx < 0 {
		m.cursorIdx = 0
	}
}
