package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
)

func TestCmdExpandFileWindowIfUndersized_returnsReloadWhenAtFirstLine(t *testing.T) {
	// Given: only the first two lines are buffered (e.g. Home ran with vh=1 before WindowSizeMsg) —
	// same undersize class as jumping to EOF with G.
	r := buffer.NewRing(100_000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileOffsets = make([]int64, 100)
	m.fileTotalLines = 100
	m.height = 25
	m.width = 80
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 1, Text: "a"},
		{Seq: 2, Text: "b"},
	})
	m.fileWinFirst = 1
	m.cursorIdx = 0
	m.follow = false
	// When:
	cmd := m.cmdExpandFileWindowIfUndersized()
	// Then:
	if cmd == nil {
		t.Fatal("Then: expected non-nil cmd to expand undersized window from file head")
	}
}

func TestCmdExpandFileWindowIfUndersized_returnsReloadWhenBufferSmallerThanTarget(t *testing.T) {
	// Given: file partial mode with 100 lines on disk but only 2 lines buffered (e.g. End ran with vh=1),
	// terminal now sized so viewport wants ~40+ lines in the window.
	r := buffer.NewRing(100_000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileOffsets = make([]int64, 100)
	m.fileTotalLines = 100
	m.height = 25 // viewportH = 22 → want min(44,100)=44 lines
	m.width = 80
	m.buf.ReplaceRecords([]domain.Line{
		{Seq: 99, Text: "a"},
		{Seq: 100, Text: "b"},
	})
	m.fileWinFirst = 99
	m.cursorIdx = 1
	m.follow = true
	// When:
	cmd := m.cmdExpandFileWindowIfUndersized()
	// Then: schedule a reload to widen the window
	if cmd == nil {
		t.Fatal("Then: expected non-nil cmd to expand undersized window")
	}
}

func TestCmdExpandFileWindowIfUndersized_noopWhenBufferAlreadyFull(t *testing.T) {
	// Given: buffer already holds the full canonical window for current viewport
	r := buffer.NewRing(100_000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileOffsets = make([]int64, 100)
	m.fileTotalLines = 100
	m.height = 25
	recs := make([]domain.Line, 44)
	for i := range recs {
		recs[i] = domain.Line{Seq: int64(i + 1), Text: "x"}
	}
	m.buf.ReplaceRecords(recs)
	m.fileWinFirst = 1
	m.cursorIdx = 0
	// When:
	cmd := m.cmdExpandFileWindowIfUndersized()
	// Then:
	if cmd != nil {
		t.Fatal("Then: expected nil when buffer already satisfies target size")
	}
}

func TestCmdExpandFileWindowIfUndersized_noopWhenEntireFileIsLoaded(t *testing.T) {
	// Given: small file — every line is already in the buffer
	r := buffer.NewRing(100_000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileOffsets = make([]int64, 5)
	m.fileTotalLines = 5
	m.height = 25
	recs := make([]domain.Line, 5)
	for i := range recs {
		recs[i] = domain.Line{Seq: int64(i + 1), Text: "x"}
	}
	m.buf.ReplaceRecords(recs)
	m.cursorIdx = 0
	// When:
	cmd := m.cmdExpandFileWindowIfUndersized()
	// Then:
	if cmd != nil {
		t.Fatal("Then: expected nil when entire file fits in buffer")
	}
}

func TestCmdLoadFileWindowAroundBottom_setsSeqAnchorsForTailFocus(t *testing.T) {
	// Given: End/G on file partial anchors cursor to the tail seq with viewTopSeq=cursorSeq-(vh-1).
	// applyFileWindowLoaded infers preferBottom from viewTopSeq < cursorSeq.
	r := buffer.NewRing(100_000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileOffsets = make([]int64, 100)
	m.fileTotalLines = 100
	m.width = 80
	m.height = 25
	// When:
	cmd := m.cmdLoadFileWindowAroundBottom(100, 22)
	// Then:
	if cmd == nil {
		t.Fatal("Then: expected load cmd")
	}
	if m.cursorSeq != 100 {
		t.Fatalf("Then: want cursorSeq=100 (tail), got %d", m.cursorSeq)
	}
	wantTop := int64(100 - (22 - 1))
	if m.viewTopSeq != wantTop {
		t.Fatalf("Then: want viewTopSeq=%d (bottom pin), got %d", wantTop, m.viewTopSeq)
	}
	if !(m.viewTopSeq < m.cursorSeq) {
		t.Fatal("Then: viewTopSeq<cursorSeq must hold (applyFileWindowLoaded's preferBottom signal)")
	}
}
