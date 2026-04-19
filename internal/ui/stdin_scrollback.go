package ui

import (
	"git.inpt.fr/42dottools/log/internal/domain"

	"github.com/charmbracelet/bubbletea"
)

// stdinScrollbackMsg carries the result of an async disk read that replaces the
// ring with a historical window (Home/PageUp/PageDown/NavUp/NavDown) or with the
// live tail (End exit).
type stdinScrollbackMsg struct {
	Records      []domain.Line
	First        int64 // first seq in the window
	TargetSeq    int64 // seq the cursor should land on (0 = default, top of window)
	PreferBottom bool  // place cursor at the bottom row of the viewport after load
	Exit         bool  // true when re-attaching to the live tail (End)
	Err          error
}

// loadStdinScrollbackWindow fetches [first, last] through the RingStreamProvider
// and returns a stdinScrollbackMsg with the supplied cursor hints. Used by every
// stdin-scrollback command (Home, NavUp/Down, PageUp/Down).
func (m *Model) loadStdinScrollbackWindow(first, last, targetSeq int64, preferBottom, exit bool) tea.Cmd {
	rsp, ok := m.windowProvider.(*RingStreamProvider)
	if !ok || !rsp.HasDiskFallback() || last < first {
		return nil
	}
	return func() tea.Msg {
		recs, err := rsp.Fetch(first, last)
		if err != nil {
			return stdinScrollbackMsg{Err: err, Exit: exit}
		}
		return stdinScrollbackMsg{
			Records:      recs,
			First:        first,
			TargetSeq:    targetSeq,
			PreferBottom: preferBottom,
			Exit:         exit,
		}
	}
}

// cmdLoadStdinScrollbackAt loads a 2*vh window starting at firstSeq. Home routes here.
func (m *Model) cmdLoadStdinScrollbackAt(firstSeq int64) tea.Cmd {
	rsp, ok := m.windowProvider.(*RingStreamProvider)
	if !ok || !rsp.HasDiskFallback() || firstSeq < 1 {
		return nil
	}
	vh := max(m.viewportH(), 1)
	last := firstSeq + int64(2*vh) - 1
	if totalRecv := rsp.TotalLines(); totalRecv > 0 && last > totalRecv {
		last = totalRecv
	}
	return m.loadStdinScrollbackWindow(firstSeq, last, firstSeq, false, false)
}

// cmdLoadStdinScrollbackEndingAt loads a 2*vh window ending at lastSeq and pins the
// cursor to lastSeq at the bottom row. NavDown/PageDown at the ring tail route here.
func (m *Model) cmdLoadStdinScrollbackEndingAt(lastSeq int64) tea.Cmd {
	rsp, ok := m.windowProvider.(*RingStreamProvider)
	if !ok || !rsp.HasDiskFallback() || lastSeq < 1 {
		return nil
	}
	if totalRecv := rsp.TotalLines(); totalRecv > 0 && lastSeq > totalRecv {
		lastSeq = totalRecv
	}
	horizon := rsp.Horizon()
	vh := max(m.viewportH(), 1)
	first := max(lastSeq-int64(2*vh)+1, horizon, 1)
	return m.loadStdinScrollbackWindow(first, lastSeq, lastSeq, true, false)
}

// cmdExitStdinScrollback loads the live tail and prepares the model to resume
// follow-mode pushes. The End key in stdin+scrollback routes through this.
func (m *Model) cmdExitStdinScrollback() tea.Cmd {
	rsp, ok := m.windowProvider.(*RingStreamProvider)
	if !ok || !rsp.HasDiskFallback() {
		return nil
	}
	totalRecv := rsp.TotalLines()
	if totalRecv < 1 {
		return nil
	}
	vh := max(m.viewportH(), 1)
	first := max(totalRecv-int64(2*vh)+1, rsp.Horizon(), 1)
	return m.loadStdinScrollbackWindow(first, totalRecv, totalRecv, true, true)
}

// canLazySearch reports whether the model can extend a ring-local search onto
// disk via a WindowProvider. True for filePartial mode (file-slice or test-injected
// provider) and for stdin whose RingStreamProvider has disk fallback configured.
func (m *Model) canLazySearch() bool {
	if m.filePartial {
		return m.windowProvider != nil || len(m.fileOffsets) > 0
	}
	if rsp, ok := m.windowProvider.(*RingStreamProvider); ok && rsp.HasDiskFallback() {
		return true
	}
	return false
}

// maybeStdinScrollbackLoadNext loads the next window when the cursor sits at the
// ring's last row and more seqs exist beyond it. Intended for callers that have
// already verified "cursor reached the tail edge" (NavDown/PageDown flow).
//
// When a filter is active, delegates to the filter-scan path so the cursor lands
// on the next filter-matching seq (not just lastRingSeq+step, which is usually a
// non-match and would fall back to the window top via idx-0 fallback).
//
// step is the logical advance (1 for ↓, vh for PageDown).
func (m *Model) maybeStdinScrollbackLoadNext(fidx []int, step int) tea.Cmd {
	if !m.stdinScrollback || len(fidx) == 0 {
		return nil
	}
	if m.cursorIdx != len(fidx)-1 {
		return nil
	}
	rsp, ok := m.windowProvider.(*RingStreamProvider)
	if !ok || !rsp.HasDiskFallback() {
		return nil
	}
	total := rsp.TotalLines()
	if total < 1 {
		return nil
	}
	lastRingSeq := m.buf.At(fidx[len(fidx)-1]).Seq
	if lastRingSeq >= total {
		return nil
	}
	if !m.prog.Empty() {
		m.filterTopupActive = true
		m.filterTopupDir = +1
		m.filterTopupNavAdvance = +step
		return m.cmdFindFilterMatchForwardFromWindowEnd()
	}
	target := min(lastRingSeq+int64(step), total)
	return m.cmdLoadStdinScrollbackEndingAt(target)
}

// maybeStdinScrollbackLoadPrev loads the previous window when the cursor sits at
// ring row 0 and earlier seqs are still reachable (first ring seq > horizon).
// Filter-aware via cmdFindFilterMatchBackwardFromWindowStart when a filter is set.
func (m *Model) maybeStdinScrollbackLoadPrev(fidx []int, step int) tea.Cmd {
	if !m.stdinScrollback || len(fidx) == 0 {
		return nil
	}
	if m.cursorIdx != 0 {
		return nil
	}
	rsp, ok := m.windowProvider.(*RingStreamProvider)
	if !ok || !rsp.HasDiskFallback() {
		return nil
	}
	firstRingSeq := m.buf.At(fidx[0]).Seq
	horizon := rsp.Horizon()
	if firstRingSeq <= horizon {
		return nil
	}
	if !m.prog.Empty() {
		m.filterTopupActive = true
		m.filterTopupDir = -1
		m.filterTopupNavAdvance = -step
		return m.cmdFindFilterMatchBackwardFromWindowStart()
	}
	target := max(firstRingSeq-int64(step), horizon)
	return m.cmdLoadStdinScrollbackAt(target)
}

// applyStdinScrollbackLoaded replaces the ring with the loaded window and
// positions the cursor/viewport from the msg hints. Preserves the seq counter
// so incoming lines (which keep arriving during scrollback) stay in sync.
func (m *Model) applyStdinScrollbackLoaded(msg stdinScrollbackMsg) {
	preservedNext := m.buf.NextSeq()
	m.buf.ReplaceRecords(msg.Records)
	m.buf.SetNextSeq(preservedNext)
	m.stdinScrollback = !msg.Exit
	// Share the window anchor with the filter-topup path so it can compute the
	// next/prev disk Fetch bounds correctly (mirrors filePartial's fileWinFirst).
	m.fileWinFirst = msg.First

	fidx := m.filteredIndices()

	if msg.Exit {
		m.follow = true
		if n := len(fidx); n > 0 {
			m.cursorIdx = n - 1
		} else {
			m.cursorIdx = 0
		}
		m.scrollTop = 0
		m.scrollSegTop = 0
		m.filterTopupActive = false
		m.filterTopupDir = 0
		m.filterTopupNavAdvance = 0
		m.fileWinFirst = 0
		m.syncScrollToCursor(fidx)
		return
	}
	m.follow = false

	idx := 0
	if msg.TargetSeq > 0 {
		for i, fi := range fidx {
			if m.buf.At(fi).Seq == msg.TargetSeq {
				idx = i
				break
			}
		}
	}
	m.cursorIdx = idx
	vh := max(m.viewportH(), 1)
	if msg.PreferBottom && idx >= vh-1 {
		m.scrollTop = idx - (vh - 1)
	} else {
		m.scrollTop = 0
	}
	m.scrollSegTop = 0
	m.syncScrollToCursor(fidx)
}
