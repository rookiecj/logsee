package ui

import (
	"fmt"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_filterMode_esc_preservesFocusedRingRow(t *testing.T) {
	// Given: 100 lines and a filter matching exactly one line (ring row 50)
	r := buffer.NewRing(200)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 100; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("id-%03d", i)})
	}
	m.filterEdit = true
	m.filterDraft = "+id-050"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	fidx := m.filteredIndices()
	if len(fidx) != 1 {
		t.Fatalf("Given: want 1 filtered line, got %d", len(fidx))
	}
	wantRing := fidx[0]
	m.cursorIdx = 0
	m.follow = false // Enter apply uses remap when follow is off (§6.4)
	// When: clear filter via filter input (Esc does not clear applied filter, PRD §6.6)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.filterDraft = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	// Then: same ring row under cursor
	fidx2 := m.filteredIndices()
	if len(fidx2) != 100 {
		t.Fatalf("want full 100 lines, got %d", len(fidx2))
	}
	if m.cursorIdx < 0 || m.cursorIdx >= len(fidx2) {
		t.Fatalf("cursorIdx out of range: %d", m.cursorIdx)
	}
	if fidx2[m.cursorIdx] != wantRing {
		t.Fatalf("When filter cleared, cursor should stay on ring row %d, got fidx slot %d -> ring %d",
			wantRing, m.cursorIdx, fidx2[m.cursorIdx])
	}
}

func TestModel_filterApply_enter_preservesRingRowWhenNotFollow(t *testing.T) {
	// Given: full list, follow off, cursor on line id-050, then apply filter that still includes that line
	r := buffer.NewRing(200)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	for i := 0; i < 100; i++ {
		m.applyIncomingLines([]string{fmt.Sprintf("id-%03d", i)})
	}
	m.follow = false
	fidx := m.filteredIndices()
	target := 50
	for i, ri := range fidx {
		if ri == target {
			m.cursorIdx = i
			break
		}
	}
	m.filterEdit = true
	m.filterDraft = "+id-05"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	fidx2 := m.filteredIndices()
	// When / Then: cursor still on ring row 50 (id-050 still matches +id-05)
	found := false
	for i, ri := range fidx2 {
		if ri == target && m.cursorIdx == i {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want cursor on ring %d after filter apply, cursorIdx=%d fidx=%v", target, m.cursorIdx, fidx2)
	}
}

func TestModel_FilterScanResultMsg_backwardPrepend_preservesCursorPhysicalSeq(t *testing.T) {
	// Given: filter top-up prepends older lines before the buffer; cursor was on physical EOF (G/End).
	// Without seq remap, cursorIdx would still point at a low filtered index (middle of the list).
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	p, err := filter.Parse("x")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "x"
	m.filterTopupActive = true
	m.filterTopupDir = -1
	m.buf.ReplaceRecords([]domain.Record{
		{Seq: 99, Text: "x"},
		{Seq: 100, Text: "x tail"},
	})
	m.cursorIdx = 1
	m.fileWinFirst = 99

	// When: backward chunk prepended
	next, _ := m.Update(FilterScanResultMsg{
		Records: []domain.Record{
			{Seq: 95, Text: "x"},
			{Seq: 96, Text: "x"},
			{Seq: 97, Text: "x"},
			{Seq: 98, Text: "x"},
		},
		FirstLine:  95,
		Direction:  -1,
		ReachedEnd: false,
	})
	m2 := next.(*Model)

	// Then: cursor stays on the same physical line (seq 100), now the last filtered row
	fidx := m2.filteredIndices()
	if len(fidx) < 2 {
		t.Fatalf("Then: want multiple filtered rows, got %d", len(fidx))
	}
	wantSeq := int64(100)
	ri := fidx[m2.cursorIdx]
	if m2.buf.At(ri).Seq != wantSeq {
		t.Fatalf("Then: want cursor on seq %d, got seq %d (cursorIdx=%d)", wantSeq, m2.buf.At(ri).Seq, m2.cursorIdx)
	}
	if m2.cursorIdx != len(fidx)-1 {
		t.Fatalf("Then: want cursor on last filtered index %d, got %d", len(fidx)-1, m2.cursorIdx)
	}
}

func TestModel_FilterScanResultMsg_forwardAppend_followsTailWhenCursorWasOldTail(t *testing.T) {
	// Given: forward top-up appends after the old buffer tail; cursor was on the last physical line.
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.width, m.height = 80, 24
	p, err := filter.Parse("x")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "x"
	m.filterTopupActive = true
	m.filterTopupDir = +1
	m.buf.ReplaceRecords([]domain.Record{
		{Seq: 99, Text: "x"},
		{Seq: 100, Text: "x tail"},
	})
	m.cursorIdx = 1
	m.fileWinFirst = 99

	next, _ := m.Update(FilterScanResultMsg{
		Records: []domain.Record{
			{Seq: 101, Text: "x"},
			{Seq: 102, Text: "x"},
		},
		Direction:  +1,
		ReachedEnd: true,
	})
	m2 := next.(*Model)
	fidx := m2.filteredIndices()
	if m2.buf.At(fidx[m2.cursorIdx]).Seq != 102 {
		t.Fatalf("Then: want cursor on new tail seq 102, got seq %d", m2.buf.At(fidx[m2.cursorIdx]).Seq)
	}
	if m2.cursorIdx != len(fidx)-1 {
		t.Fatalf("Then: want last filtered index, got cursorIdx=%d len=%d", m2.cursorIdx, len(fidx))
	}
}
