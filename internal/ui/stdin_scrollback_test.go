package ui

import (
	"os"
	"path/filepath"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/fileindex"
	"git.inpt.fr/42dottools/log/internal/filter"

	tea "github.com/charmbracelet/bubbletea"
)

// setupModelWithDiskFallback wires a Model + RingStreamProvider + disk fallback
// so the Home/End key paths and async load commands can be exercised end-to-end.
// The returned writer flushes a line to both the --out file and the ring in the
// same order that applyIncomingLines would.
func setupModelWithDiskFallback(t *testing.T, ringMax int) (*Model, func(string)) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	r := buffer.NewRing(ringMax)
	m := NewModel(r, nil, false, false, path, "stdin", "", nil, nil, nil)
	// Small viewport keeps 2*vh below the ring cap so ReplaceRecords retains the full window.
	m.width, m.height = 80, 7

	idx := fileindex.NewIncrementalOffsetIndex(path, 0)
	rsp := NewRingStreamProvider(r)
	rsp.SetDiskFallback(path, idx, 1)
	m.windowProvider = rsp

	write := func(text string) {
		t.Helper()
		if _, err := f.WriteString(text + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := f.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if m.stdinScrollback {
			rsp.AssignSeq()
		} else {
			rsp.Push(text)
		}
		st, err := f.Stat()
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if err := rsp.RefreshIndex(st.Size()); err != nil {
			t.Fatalf("refresh: %v", err)
		}
	}
	return m, write
}

func TestStdinScrollback_HomeLoadsEarliestWindow(t *testing.T) {
	m, write := setupModelWithDiskFallback(t, 10)
	for range 20 {
		write("L")
	}
	// Ring holds only the 5 most recent; seqs 1..15 are evicted.
	cmd := m.cmdLoadStdinScrollbackAt(1)
	if cmd == nil {
		t.Fatal("expected cmd from cmdLoadStdinScrollbackAt")
	}
	msg, ok := cmd().(stdinScrollbackMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T", cmd())
	}
	if msg.Err != nil {
		t.Fatalf("msg err: %v", msg.Err)
	}
	if msg.First != 1 {
		t.Fatalf("First=%d, want 1", msg.First)
	}
	if len(msg.Records) == 0 || msg.Records[0].Seq != 1 {
		t.Fatalf("records=%+v, want starting at seq 1", msg.Records)
	}

	m.applyStdinScrollbackLoaded(msg)
	if !m.stdinScrollback {
		t.Error("stdinScrollback should be true after Home load")
	}
	if m.follow {
		t.Error("follow should be false during scrollback")
	}
	// Ring now shows the loaded historical window.
	if got := m.buf.At(0).Seq; got != 1 {
		t.Fatalf("ring[0].Seq=%d, want 1", got)
	}
}

func TestStdinScrollback_PushesFrozenWhileActive(t *testing.T) {
	m, write := setupModelWithDiskFallback(t, 10)
	for range 20 {
		write("L")
	}
	cmd := m.cmdLoadStdinScrollbackAt(1)
	msg := cmd().(stdinScrollbackMsg)
	m.applyStdinScrollbackLoaded(msg)
	ringBefore := m.buf.Len()
	firstSeqBefore := m.buf.At(0).Seq

	// New lines arrive during scrollback: should NOT push into the ring.
	for range 10 {
		write("M")
	}

	if got := m.buf.Len(); got != ringBefore {
		t.Errorf("ring size during scrollback: got %d, want %d (frozen)", got, ringBefore)
	}
	if got := m.buf.At(0).Seq; got != firstSeqBefore {
		t.Errorf("ring[0].Seq drifted: got %d, want %d", got, firstSeqBefore)
	}
	// Cumulative receive count keeps growing.
	if got := m.windowProvider.TotalLines(); got != 30 {
		t.Errorf("TotalLines=%d, want 30 (20 before + 10 during scrollback)", got)
	}
}

func TestStdinScrollback_PageDownLoadsNextWindow(t *testing.T) {
	// Ring max 10, viewportH=4 (height=7), 2*vh=8 → Home loads seqs 1..8. PageDown at
	// cursor=last should advance the window toward the tail instead of stalling.
	m, write := setupModelWithDiskFallback(t, 10)
	for range 50 {
		write("L")
	}
	// Home loads [1, 8].
	entry := m.cmdLoadStdinScrollbackAt(1)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))
	if first := m.buf.At(0).Seq; first != 1 {
		t.Fatalf("initial window first seq=%d, want 1", first)
	}

	fidx := m.filteredIndices()
	vh := m.viewportH()
	// Simulate "cursor at bottom of loaded window" and trigger PageDown load.
	m.cursorIdx = len(fidx) - 1
	cmd := m.maybeStdinScrollbackLoadNext(fidx, vh)
	if cmd == nil {
		t.Fatal("expected next-window load cmd")
	}
	msg := cmd().(stdinScrollbackMsg)
	m.applyStdinScrollbackLoaded(msg)

	// The window should have shifted forward by vh; last ring seq should be target (lastPrev+vh).
	// Previous last was 8; target = min(8+4, 50) = 12.
	if last := m.buf.At(m.buf.Len() - 1).Seq; last != 12 {
		t.Errorf("after page-down load: last ring seq=%d, want 12", last)
	}
	if !m.stdinScrollback {
		t.Error("still expected to be in scrollback mode after page-down load")
	}
}

func TestStdinScrollback_PageUpLoadsPreviousWindow(t *testing.T) {
	m, write := setupModelWithDiskFallback(t, 10)
	for range 50 {
		write("L")
	}
	// Start scrollback in the middle so PageUp has room to go back.
	entry := m.cmdLoadStdinScrollbackAt(20)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))
	if first := m.buf.At(0).Seq; first != 20 {
		t.Fatalf("initial window first seq=%d, want 20", first)
	}

	fidx := m.filteredIndices()
	vh := m.viewportH()
	m.cursorIdx = 0
	cmd := m.maybeStdinScrollbackLoadPrev(fidx, vh)
	if cmd == nil {
		t.Fatal("expected prev-window load cmd")
	}
	msg := cmd().(stdinScrollbackMsg)
	m.applyStdinScrollbackLoaded(msg)

	// Previous first was 20; target = max(20-4, 1) = 16.
	if first := m.buf.At(0).Seq; first != 16 {
		t.Errorf("after page-up load: first ring seq=%d, want 16", first)
	}
}

func TestStdinScrollback_LoadNextStopsAtTotalLines(t *testing.T) {
	m, write := setupModelWithDiskFallback(t, 10)
	for range 10 {
		write("L")
	}
	// Load a window that already includes seq 10 (the last received).
	entry := m.cmdLoadStdinScrollbackEndingAt(10)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))
	if last := m.buf.At(m.buf.Len() - 1).Seq; last != 10 {
		t.Fatalf("initial window last seq=%d, want 10", last)
	}

	fidx := m.filteredIndices()
	m.cursorIdx = len(fidx) - 1
	// ring tail already == totalRecv; no next window to load.
	if cmd := m.maybeStdinScrollbackLoadNext(fidx, 4); cmd != nil {
		t.Error("expected nil cmd when ring tail == totalRecv")
	}
}

func TestStdinScrollback_LazySearchScansDiskPastRingWindow(t *testing.T) {
	// Given: stdin + disk fallback with a match that lives outside the Home-loaded window.
	m, _ := setupModelWithDiskFallback(t, 10)
	dir := filepath.Dir(m.outPath)
	path := filepath.Join(dir, "session.log")
	// Write 30 lines; highlight-target "HIT" only at seqs 3 and 25.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	rsp := m.windowProvider.(*RingStreamProvider)
	for i := 1; i <= 30; i++ {
		line := "plain"
		if i == 3 || i == 25 {
			line = "HIT"
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		rsp.Push(line)
	}
	_ = f.Sync()
	st, _ := f.Stat()
	if err := rsp.RefreshIndex(st.Size()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Enter scrollback at seq 1; ring holds [1..8].
	entry := m.cmdLoadStdinScrollbackAt(1)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))

	// canLazySearch must be true for stdin + disk fallback.
	if !m.canLazySearch() {
		t.Fatal("canLazySearch should be true with stdin + disk fallback")
	}

	// Configure search and scan forward from seq 4 (i.e., after the first in-window hit).
	m.searchBuf = "HIT"
	cmd := m.cmdScanSearchInFile(+1, 4, 0, false)
	if cmd == nil {
		t.Fatal("cmdScanSearchInFile returned nil; lazy disk scan disabled?")
	}
	msg, ok := cmd().(SearchScanResultMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T", cmd())
	}
	if msg.Err != nil {
		t.Fatalf("scan err: %v", msg.Err)
	}
	if msg.FoundSeq != 25 {
		t.Fatalf("FoundSeq=%d, want 25", msg.FoundSeq)
	}
}

// TestStdinScrollback_CtrlN_EndToEnd simulates the user flow: after Home,
// pressing Ctrl+n must find a match that lives past the loaded ring window,
// trigger a disk scan, then load a new scrollback window around the found seq
// with the cursor on it.
func TestStdinScrollback_CtrlN_EndToEnd(t *testing.T) {
	m, _ := setupModelWithDiskFallback(t, 100)
	dir := filepath.Dir(m.outPath)
	path := filepath.Join(dir, "session.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	rsp := m.windowProvider.(*RingStreamProvider)
	for i := 1; i <= 100; i++ {
		line := "plain"
		if i == 3 || i == 80 {
			line = "HIT"
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		rsp.Push(line)
	}
	_ = f.Sync()
	st, _ := f.Stat()
	if err := rsp.RefreshIndex(st.Size()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Home: load scrollback [1..8] (2*vh with vh=4).
	entry := m.cmdLoadStdinScrollbackAt(1)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))

	// Active search needle.
	m.searchBuf = "HIT"

	// Press Ctrl+n — first hit at seq 3 is in the ring.
	fidx := m.filteredIndices()
	prev := m.cursorIdx
	m.gotoNextSearchHit(fidx)
	if m.cursorIdx == prev {
		t.Fatal("first Ctrl+n should move cursor to seq 3 inside ring")
	}
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 3 {
		t.Fatalf("cursor seq after 1st Ctrl+n: got %d, want 3", got)
	}

	// Press Ctrl+n again — next hit (seq 80) lives past the ring. gotoNextSearchHit
	// won't move; we should fall through to the disk scan.
	prev = m.cursorIdx
	m.gotoNextSearchHit(fidx)
	if m.cursorIdx != prev {
		t.Fatalf("expected no ring movement for 2nd Ctrl+n; cursor moved to idx %d", m.cursorIdx)
	}
	if !m.canLazySearch() {
		t.Fatal("canLazySearch must be true")
	}
	cmd := m.cmdStartLazySearch(+1, fidx)
	if cmd == nil {
		t.Fatal("cmdStartLazySearch returned nil")
	}
	scanMsg, ok := cmd().(SearchScanResultMsg)
	if !ok {
		t.Fatalf("expected SearchScanResultMsg, got %T", cmd())
	}
	if scanMsg.Err != nil {
		t.Fatalf("scan err: %v", scanMsg.Err)
	}
	if scanMsg.FoundSeq != 80 {
		t.Fatalf("FoundSeq=%d, want 80", scanMsg.FoundSeq)
	}

	// Feed the result back through Update. The handler should dispatch a stdin
	// scrollback load ending at seq 80.
	next, nextCmd := m.Update(scanMsg)
	m = next.(*Model)
	if nextCmd == nil {
		t.Fatal("Update(SearchScanResultMsg) returned nil cmd; expected scrollback load")
	}
	loadMsg, ok := nextCmd().(stdinScrollbackMsg)
	if !ok {
		t.Fatalf("expected stdinScrollbackMsg from load cmd, got %T", nextCmd())
	}
	if loadMsg.Err != nil {
		t.Fatalf("load err: %v", loadMsg.Err)
	}
	if loadMsg.TargetSeq != 80 || !loadMsg.PreferBottom {
		t.Fatalf("load msg: TargetSeq=%d PreferBottom=%v, want 80 true", loadMsg.TargetSeq, loadMsg.PreferBottom)
	}

	next, _ = m.Update(loadMsg)
	m = next.(*Model)
	fidx = m.filteredIndices()
	if m.cursorIdx < 0 || m.cursorIdx >= len(fidx) {
		t.Fatalf("cursorIdx=%d out of fidx len=%d", m.cursorIdx, len(fidx))
	}
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 80 {
		t.Fatalf("cursor seq after flow: got %d, want 80", got)
	}
}

// TestStdinScrollback_CtrlN_ViaKeyMsg drives Ctrl+n through the real Update(KeyMsg)
// path (handleKey → tryBrowseKey) to catch any divergence between the helper-level
// flow and the key-dispatch flow.
func TestStdinScrollback_CtrlN_ViaKeyMsg(t *testing.T) {
	m, _ := setupModelWithDiskFallback(t, 100)
	dir := filepath.Dir(m.outPath)
	path := filepath.Join(dir, "session.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	rsp := m.windowProvider.(*RingStreamProvider)
	for i := 1; i <= 100; i++ {
		line := "plain"
		if i == 3 || i == 80 {
			line = "HIT"
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		rsp.Push(line)
	}
	_ = f.Sync()
	st, _ := f.Stat()
	if err := rsp.RefreshIndex(st.Size()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Home → scrollback load.
	entry := m.cmdLoadStdinScrollbackAt(1)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))

	m.searchBuf = "HIT"

	// First Ctrl+n via Update: should land on seq 3 in-ring. No follow-up cmd.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(*Model)
	if cmd != nil {
		t.Fatalf("first Ctrl+n returned a cmd (%T) — in-ring hit should be synchronous", cmd())
	}
	fidx := m.filteredIndices()
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 3 {
		t.Fatalf("after first Ctrl+n: cursor seq=%d, want 3", got)
	}

	// Second Ctrl+n via Update: exhausts in-ring → must return a lazy-search cmd.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("second Ctrl+n returned nil cmd — lazy search not triggered")
	}
	scanMsg, ok := cmd().(SearchScanResultMsg)
	if !ok {
		t.Fatalf("expected SearchScanResultMsg, got %T", cmd())
	}
	if scanMsg.FoundSeq != 80 {
		t.Fatalf("FoundSeq=%d, want 80", scanMsg.FoundSeq)
	}
	// Feed scan result back to Update; it should dispatch the scrollback load.
	next, cmd = m.Update(scanMsg)
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("Update(SearchScanResultMsg) returned nil cmd — scrollback load not dispatched")
	}
	loadMsg := cmd().(stdinScrollbackMsg)
	next, _ = m.Update(loadMsg)
	m = next.(*Model)
	fidx = m.filteredIndices()
	if got := m.buf.At(fidx[m.cursorIdx]).Seq; got != 80 {
		t.Fatalf("after lazy search flow: cursor seq=%d, want 80", got)
	}
}

// TestStdinScrollback_FilterTopupChainsUntilViewportFilled covers issue 1:
// after a Home load in filter mode, the 2*vh raw records typically yield very
// few filter matches. The scrollback msg handler must chain topup loads until
// the filtered fidx reaches viewport height (or the log ends).
func TestStdinScrollback_FilterTopupChainsUntilViewportFilled(t *testing.T) {
	m, _ := setupModelWithDiskFallback(t, 500)
	dir := filepath.Dir(m.outPath)
	path := filepath.Join(dir, "session.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	rsp := m.windowProvider.(*RingStreamProvider)
	// 200 lines; 1 in 20 matches "HIT" → 10 matches in total.
	for i := 1; i <= 200; i++ {
		line := "plain"
		if i%20 == 0 {
			line = "HIT"
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		rsp.Push(line)
	}
	_ = f.Sync()
	st, _ := f.Stat()
	if err := rsp.RefreshIndex(st.Size()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Apply filter "HIT".
	p, err := filter.Parse("HIT")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "HIT"

	vh := m.viewportH()

	// Home loads [1..2*vh]. Without topup the filtered fidx would be very small.
	entryMsg := m.cmdLoadStdinScrollbackAt(1)().(stdinScrollbackMsg)
	// Feed through Update so the topup trigger fires.
	next, cmd := m.Update(entryMsg)
	m = next.(*Model)

	// Topup should keep chaining until we have vh+ matches or EOF.
	for cmd != nil {
		msg := cmd()
		next, cmd = m.Update(msg)
		m = next.(*Model)
	}

	fidx := m.filteredIndices()
	if len(fidx) < vh && int64(len(fidx)) < rsp.TotalLines()/20 {
		// We expect either viewport-full, or all 10 matches if log is small.
		t.Fatalf("after topup chain: fidx len=%d, want >= %d (viewport) or all 10 matches", len(fidx), vh)
	}
}

// TestStdinScrollback_DownAtEdgeFilterAware covers issue 2: Down at the last
// filtered match should advance cursor to the NEXT filter-matching seq (not
// fall back to the top row because raw lastRingSeq+1 is a non-match).
func TestStdinScrollback_DownAtEdgeFilterAware(t *testing.T) {
	m, _ := setupModelWithDiskFallback(t, 500)
	dir := filepath.Dir(m.outPath)
	path := filepath.Join(dir, "session.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	rsp := m.windowProvider.(*RingStreamProvider)
	// Matches at seqs 5, 50, 120.
	for i := 1; i <= 200; i++ {
		line := "plain"
		if i == 5 || i == 50 || i == 120 {
			line = "HIT"
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		rsp.Push(line)
	}
	_ = f.Sync()
	st, _ := f.Stat()
	if err := rsp.RefreshIndex(st.Size()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	p, err := filter.Parse("HIT")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "HIT"

	// Home: load + topup chain.
	entryMsg := m.cmdLoadStdinScrollbackAt(1)().(stdinScrollbackMsg)
	next, cmd := m.Update(entryMsg)
	m = next.(*Model)
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(*Model)
	}

	// Move cursor to the last in-ring filter match (the 3rd one, seq 120, if the
	// topup loaded enough; else the last one we have).
	fidx := m.filteredIndices()
	if len(fidx) == 0 {
		t.Fatal("expected at least one filter match in ring after topup")
	}
	m.cursorIdx = len(fidx) - 1
	lastMatchSeq := m.buf.At(fidx[m.cursorIdx]).Seq
	// If we already have all 3 matches loaded, manually position at match #2 to leave #3 unloaded.
	if lastMatchSeq >= 120 {
		for i := range fidx {
			if s := m.buf.At(fidx[i]).Seq; s == 50 {
				m.cursorIdx = i
				lastMatchSeq = 50
				break
			}
		}
	}

	// Press ↓ at the filter edge. In filter mode, maybeStdinScrollbackLoadNext should
	// delegate to cmdFindFilterMatchForwardFromWindowEnd with nav-advance=+1.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(*Model)
	}

	fidx = m.filteredIndices()
	if len(fidx) == 0 {
		t.Fatal("fidx empty after nav")
	}
	curSeq := m.buf.At(fidx[m.cursorIdx]).Seq
	if curSeq <= lastMatchSeq {
		t.Fatalf("cursor seq=%d must have advanced past %d (next filter match expected)", curSeq, lastMatchSeq)
	}
}

func TestStdinScrollback_EndReloadsLiveTail(t *testing.T) {
	m, write := setupModelWithDiskFallback(t, 10)
	for range 20 {
		write("L")
	}
	// Enter scrollback.
	entry := m.cmdLoadStdinScrollbackAt(1)
	m.applyStdinScrollbackLoaded(entry().(stdinScrollbackMsg))

	// More lines arrive.
	for range 10 {
		write("M")
	}

	// End exits scrollback.
	cmd := m.cmdExitStdinScrollback()
	if cmd == nil {
		t.Fatal("expected cmd from cmdExitStdinScrollback")
	}
	exitMsg := cmd().(stdinScrollbackMsg)
	if !exitMsg.Exit {
		t.Error("expected Exit=true on exit command")
	}
	m.applyStdinScrollbackLoaded(exitMsg)

	if m.stdinScrollback {
		t.Error("stdinScrollback should be false after End")
	}
	if !m.follow {
		t.Error("follow should be true after End")
	}
	// The ring now shows recent seqs including the 30th (last written).
	last := m.buf.At(m.buf.Len() - 1)
	if last.Seq != 30 {
		t.Errorf("tail seq=%d, want 30", last.Seq)
	}
}
