package ui

import (
	"fmt"
	"os"

	"git.inpt.fr/42dottools/log/internal/domain"
	"github.com/charmbracelet/bubbletea"
)

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		fidx := m.filteredIndices()
		m.clampSelectionToBuffer(fidx)
		m.syncScrollToCursor(fidx)
		if cmd := m.cmdExpandFileWindowIfUndersized(); cmd != nil {
			return m, cmd
		}
		if m.filePartial && m.appliedFilter != "" && len(fidx) < m.viewportH() {
			if !m.filterTopupActive {
				m.filterTopupActive = true
			}
			if m.filterTopupDir == 0 {
				m.filterTopupDir = m.pickFilterTopupDirWhenUndersized()
			}
			cmd := m.cmdFindFilterTopupByDir()
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case LineMsg:
		m.applyIncomingLines([]string{string(msg)})
		return m, nil

	case LineBatchMsg:
		m.applyIncomingLines([]string(msg))
		return m, nil

	case FilePartialBootstrapMsg:
		if msg.Err != nil {
			fmt.Fprintf(os.Stderr, "logsee: file bootstrap: %v\n", msg.Err)
			return m, tea.Quit
		}
		m.applyFilePartialBootstrap(msg.Path, msg.Lines)
		fidx := m.filteredIndices()
		m.clampSelectionToBuffer(fidx)
		m.syncScrollToCursor(fidx)
		if m.filePartial && m.appliedFilter != "" && len(fidx) < m.viewportH() {
			if !m.filterTopupActive {
				m.filterTopupActive = true
			}
			if m.filterTopupDir == 0 {
				m.filterTopupDir = m.pickFilterTopupDirWhenUndersized()
			}
			cmd := m.cmdFindFilterTopupByDir()
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case FileIndexReadyMsg:
		if msg.Err != nil {
			fmt.Fprintf(os.Stderr, "logsee: file index: %v\n", msg.Err)
			return m, nil
		}
		m.applyFileIndexReady(msg.Offsets)
		if m.filterTopupActive && m.filePartial && m.appliedFilter != "" {
			fidx := m.filteredIndices()
			if len(fidx) >= m.viewportH() {
				m.filterTopupActive = false
				return m, nil
			}
			if m.filterTopupDir == 0 {
				m.filterTopupDir = m.pickFilterTopupDirWhenUndersized()
			}
			cmd := m.cmdFindFilterTopupByDir()
			if cmd == nil {
				m.filterTopupActive = false
				return m, nil
			}
			return m, cmd
		}
		return m, nil

	case stdinScrollbackMsg:
		if msg.Err != nil {
			fmt.Fprintf(os.Stderr, "logsee: stdin scrollback: %v\n", msg.Err)
			return m, nil
		}
		m.applyStdinScrollbackLoaded(msg)
		// Filter active + undersized filtered fidx: chain a topup so the viewport fills
		// with filter matches (mirror of FileWindowLoadedMsg's topup trigger).
		if m.stdinScrollback && m.appliedFilter != "" {
			fidx := m.filteredIndices()
			if len(fidx) >= m.viewportH() {
				m.filterTopupActive = false
			} else {
				if !m.filterTopupActive {
					m.filterTopupActive = true
				}
				if m.filterTopupDir == 0 {
					m.filterTopupDir = m.pickFilterTopupDirWhenUndersized()
				}
				if cmd := m.cmdFindFilterTopupByDir(); cmd != nil {
					return m, cmd
				}
				m.filterTopupActive = false
			}
		}
		return m, nil

	case FileWindowLoadedMsg:
		if msg.Err != nil {
			fmt.Fprintf(os.Stderr, "logsee: file window: %v\n", msg.Err)
			return m, nil
		}
		m.applyFileWindowLoaded(msg.Records, msg.FirstLine)
		if cmd := m.cmdExpandFileWindowIfUndersized(); cmd != nil {
			return m, cmd
		}
		if m.filePartial && m.appliedFilter != "" {
			fidx := m.filteredIndices()
			if len(fidx) >= m.viewportH() {
				m.filterTopupActive = false
			} else {
				if !m.filterTopupActive {
					m.filterTopupActive = true
				}
				if m.filterTopupDir == 0 {
					m.filterTopupDir = m.pickFilterTopupDirWhenUndersized()
				}
				cmd := m.cmdFindFilterTopupByDir()
				if cmd == nil {
					m.filterTopupActive = false
				} else {
					return m, cmd
				}
			}
		}
		return m, nil

	case SearchScanResultMsg:
		if msg.Err != nil {
			m.copyFlash = msg.Err.Error()
			return m, nil
		}
		if msg.NeedConfirm {
			m.searchScanConfirmOpen = true
			m.searchScanResumeSeq = msg.ResumeSeq
			m.searchScanDir = msg.Direction
			m.searchScanScanned = msg.ScannedBytes
			m.copyFlash = fmt.Sprintf("search scanned %d MiB", msg.ScannedBytes>>20)
			return m, nil
		}
		if msg.FoundSeq > 0 {
			vh := m.viewportH()
			if m.filePartial && len(m.fileOffsets) > 0 {
				if msg.Direction > 0 {
					return m, m.cmdLoadFileWindowAroundBottom(msg.FoundSeq, vh)
				}
				return m, m.cmdLoadFileWindowAroundTop(msg.FoundSeq, vh)
			}
			// stdin + disk fallback: load a scrollback window around the found seq.
			if msg.Direction > 0 {
				return m, m.cmdLoadStdinScrollbackEndingAt(msg.FoundSeq)
			}
			return m, m.cmdLoadStdinScrollbackAt(msg.FoundSeq)
		}
		if msg.ReachedEnd {
			m.copyFlash = "no matching line"
		}
		return m, nil

	case FilterScanResultMsg:
		if msg.Err != nil {
			m.filterTopupActive = false
			m.copyFlash = msg.Err.Error()
			return m, nil
		}
		if len(msg.Records) > 0 {
			var focusSeq int64
			var oldLastMatchSeq int64
			if m.buf != nil && m.buf.Len() > 0 {
				oldFidx := m.filteredIndices()
				if len(oldFidx) > 0 && m.cursorIdx >= 0 && m.cursorIdx < len(oldFidx) {
					focusSeq = m.buf.At(oldFidx[m.cursorIdx]).Seq
				}
				if len(oldFidx) > 0 {
					oldLastMatchSeq = m.buf.At(oldFidx[len(oldFidx)-1]).Seq
				}
			}
			existing := make([]domain.Line, 0, m.buf.Len()+len(msg.Records))
			if msg.Direction < 0 {
				existing = append(existing, msg.Records...)
				for i := 0; i < m.buf.Len(); i++ {
					existing = append(existing, m.buf.At(i))
				}
			} else {
				for i := 0; i < m.buf.Len(); i++ {
					existing = append(existing, m.buf.At(i))
				}
				existing = append(existing, msg.Records...)
			}
			m.buf.ReplaceRecords(existing)
			if msg.Direction < 0 {
				m.fileWinFirst = msg.FirstLine
			}
			fidx := m.filteredIndices()

			// Nav-intent advance: j/k/PageUp/PageDown at a filter window edge delegated to filter-scan
			// with a step count in filterTopupNavAdvance (±1 for j/k, ±vh for Page keys). Consume as
			// many steps as the newly merged fidx can satisfy past focusSeq; leftover steps keep the
			// scan chain alive until satisfied or EOF. The underlying "next seq in fidx" lookup is the
			// Phase 3 SeqMatcher helper — fidx here is already filter-projected, so predicate=nil.
			navAdvanceSatisfied := false // cursor set by advance logic — skip default switch
			navAdvanceDone := false      // all requested steps delivered — stop chaining
			if focusSeq > 0 && m.filterTopupNavAdvance != 0 && msg.Direction*m.filterTopupNavAdvance > 0 {
				dir := +1
				remaining := m.filterTopupNavAdvance
				if remaining < 0 {
					dir = -1
					remaining = -remaining
				}
				advanced := 0
				lastTarget := -1
				cur := focusSeq
				for advanced < remaining {
					idx := m.nextMatchIdxInFidx(fidx, cur, dir, nil)
					if idx < 0 {
						break
					}
					lastTarget = idx
					advanced++
					cur = m.buf.At(fidx[idx]).Seq
				}
				if lastTarget >= 0 {
					m.cursorIdx = lastTarget
					navAdvanceSatisfied = true
					if advanced >= remaining {
						m.filterTopupNavAdvance = 0
						navAdvanceDone = true
					} else {
						// Keep sign to preserve direction for the chained scan.
						leftover := remaining - advanced
						if dir < 0 {
							m.filterTopupNavAdvance = -leftover
						} else {
							m.filterTopupNavAdvance = leftover
						}
					}
				}
			}

			if !navAdvanceSatisfied {
				switch {
				case len(fidx) == 0:
					m.cursorIdx = 0
				case msg.Direction > 0 && focusSeq > 0 && focusSeq == oldLastMatchSeq:
					// Forward top-up whose cursor was on the last-visible match: stay pinned to the tail match.
					m.cursorIdx = len(fidx) - 1
				case focusSeq > 0:
					if !m.remapCursorPreservingFilteredSeq(fidx, focusSeq) {
						m.clampCursor(fidx)
					}
				default:
					m.clampCursor(fidx)
				}
			}

			m.clampSelectionToBuffer(fidx)
			if navAdvanceSatisfied && len(fidx) > 0 && m.cursorIdx >= 0 && m.cursorIdx < len(fidx) {
				// Nav-advance: anchor the cursor to the expected screen edge via seq (Phase 4 —
				// replaces applyStickyFileScrollPin). Forward direction lands cursor on bottom row
				// (PageDown/j intent); backward lands cursor on top row (PageUp/k intent).
				m.cursorSeq = m.buf.At(fidx[m.cursorIdx]).Seq
				vh := m.viewportH()
				if vh < 1 {
					vh = 1
				}
				fallbackRow := 0
				if msg.Direction > 0 {
					// viewTopSeq must be the Seq of the record (vh-1) filtered
					// rows above the cursor, not `cursorSeq - (vh-1)` in raw
					// file-line space. When a filter is active (e.g. level:D),
					// fidx is sparser than the file line numbering, so raw-seq
					// arithmetic undercounts the distance and parks the cursor
					// in the middle of the viewport instead of the bottom row.
					topIdx := m.cursorIdx - (vh - 1)
					if topIdx < 0 {
						topIdx = 0
					}
					m.viewTopSeq = m.buf.At(fidx[topIdx]).Seq
					fallbackRow = vh - 1
				} else {
					m.viewTopSeq = m.cursorSeq
				}
				m.syncIdxFromSeq(fidx, fallbackRow)
			} else {
				m.syncScrollToCursor(fidx)
			}

			// All requested advance steps delivered: stop chaining.
			if navAdvanceDone {
				m.filterTopupActive = false
				m.filterTopupDir = 0
				return m, nil
			}
			// Partial advance and file has more to scan: continue the same direction.
			if navAdvanceSatisfied {
				if !msg.ReachedEnd {
					return m, m.cmdFindFilterTopupByDir()
				}
				// EOF with remaining steps: cursor rests at the last reachable match.
				m.filterTopupActive = false
				m.filterTopupDir = 0
				m.filterTopupNavAdvance = 0
				return m, nil
			}
			if len(fidx) >= m.viewportH() {
				m.filterTopupActive = false
				m.filterTopupDir = 0
				return m, nil
			}
			if !msg.ReachedEnd {
				return m, m.cmdFindFilterTopupByDir()
			}
			if cmd := m.maybeCmdFilterTopupBackwardAfterForwardSparse(fidx, msg.Direction); cmd != nil {
				return m, cmd
			}
		} else if msg.ReachedEnd {
			fidx := m.filteredIndices()
			if cmd := m.maybeCmdFilterTopupBackwardAfterForwardSparse(fidx, msg.Direction); cmd != nil {
				return m, cmd
			}
		}
		if msg.ReachedEnd {
			m.filterTopupActive = false
			m.filterTopupDir = 0
			m.filterTopupNavAdvance = 0
			if len(m.filteredIndices()) == 0 {
				m.copyFlash = "no lines"
			}
		}
		return m, nil

	case StdinClosedMsg:
		m.stdinClosed = true
		m.maybeResolveAutoFormat()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// applyIncomingLines persists each line to disk first when store is set (stdin input); then appends to the ring.
// wasAtTail is determined once before the batch (PRD: follow tail when new lines arrive in a burst).
//
// The filePartial guard is a safety rail: LineMsg / LineBatchMsg only fire from the stdin pump
// (cmd/logsee/main.go), and filePartial inputs never spawn that pump. Kept for defense in depth.
func (m *Model) applyIncomingLines(texts []string) {
	if m.filePartial {
		return
	}
	if len(texts) == 0 {
		return
	}
	fidxBefore := m.filteredIndices()
	wasAtTail := m.follow && m.tailAligned(fidxBefore)
	rsp, _ := m.windowProvider.(*RingStreamProvider)
	for _, text := range texts {
		var rotated bool
		if m.store != nil {
			if err := m.store.WriteLine(text); err != nil {
				fmt.Fprintf(os.Stderr, "logsee: write output %q: %v\n", m.outPath, err)
				continue
			}
			if err := m.store.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "logsee: flush output %q: %v\n", m.outPath, err)
				continue
			}
			rotated = m.store.ConsumeRotation()
		}
		var seq int64
		switch {
		case rsp != nil && m.stdinScrollback:
			seq = rsp.AssignSeq()
		case rsp != nil:
			seq = rsp.Push(text).Seq
		default:
			seq = m.buf.Push(text).Seq
		}
		if rotated && rsp != nil {
			rsp.NoteRotation(seq)
		}
		m.classifyIncoming(seq, text)
	}
	if rsp != nil && m.store != nil {
		if err := rsp.RefreshIndex(m.store.Size()); err != nil {
			fmt.Fprintf(os.Stderr, "logsee: refresh offset index: %v\n", err)
		}
	}
	fidx := m.filteredIndices()
	if m.follow && wasAtTail && len(fidx) > 0 {
		m.cursorIdx = len(fidx) - 1
	}
	m.clampSelectionToBuffer(fidx)
	m.syncScrollToCursor(fidx)
	m.maybeResolveAutoFormat()
}

func (m *Model) cmdFindFilterTopupByDir() tea.Cmd {
	if m.filterTopupDir < 0 {
		return m.cmdFindFilterMatchBackwardFromWindowStart()
	}
	return m.cmdFindFilterMatchForwardFromWindowEnd()
}

// pickFilterTopupDirWhenUndersized chooses forward vs backward filter scanning when the in-memory
// window has fewer matching rows than the viewport. At physical EOF there is nothing to load
// forward; prefer backward so keys like End/G still fill the filtered view.
//
// filePartial uses fileTotalLines; stdin+scrollback reads the provider's cumulative total. Either
// path flips to backward only when the ring already touches the tail and there is room above.
func (m *Model) pickFilterTopupDirWhenUndersized() int {
	if m.buf == nil || m.buf.Len() == 0 {
		return +1
	}
	total := int64(m.fileTotalLines)
	if total <= 0 && m.windowProvider != nil {
		total = m.windowProvider.TotalLines()
	}
	if total <= 0 {
		return +1
	}
	lastSeq := m.buf.At(m.buf.Len() - 1).Seq
	if lastSeq != total {
		return +1
	}
	if m.fileWinFirst <= 1 {
		return +1
	}
	return -1
}

// maybeCmdFilterTopupBackwardAfterForwardSparse schedules a backward file read when a forward
// filter scan reached EOF (or appended the last chunk) but the viewport still has too few rows.
func (m *Model) maybeCmdFilterTopupBackwardAfterForwardSparse(fidx []int, scanDirection int) tea.Cmd {
	if !m.canLazySearch() || !m.filterTopupActive || m.buf == nil || m.appliedFilter == "" {
		return nil
	}
	vh := m.viewportH()
	if vh < 1 {
		vh = 1
	}
	if len(fidx) >= vh {
		return nil
	}
	if scanDirection <= 0 {
		return nil
	}
	if m.fileWinFirst <= 1 {
		return nil
	}
	m.filterTopupDir = -1
	return m.cmdFindFilterMatchBackwardFromWindowStart()
}
