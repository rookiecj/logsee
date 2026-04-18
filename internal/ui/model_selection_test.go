package ui

import "testing"

func TestModel_selectionRange_order(t *testing.T) {
	// Given: anchor below cursor
	// When: selectionRange
	// Then: lo/hi are ordered inclusive
	m := &Model{selAnchor: 7, cursorIdx: 2}
	lo, hi, ok := m.selectionRange()
	if !ok || lo != 2 || hi != 7 {
		t.Fatalf("got %d-%d ok=%v", lo, hi, ok)
	}
}

func TestModel_lineSelected(t *testing.T) {
	m := &Model{selAnchor: 1, cursorIdx: 3}
	for _, fi := range []int{0, 4} {
		if m.lineSelected(fi) {
			t.Fatalf("fi %d should not be selected", fi)
		}
	}
	for _, fi := range []int{1, 2, 3} {
		if !m.lineSelected(fi) {
			t.Fatalf("fi %d should be selected", fi)
		}
	}
}

func TestModel_clearAllSelection(t *testing.T) {
	m := &Model{selAnchor: 0, cursorIdx: 2, picked: map[int]struct{}{0: {}, 5: {}}}
	m.clearAllSelection()
	if _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected no range selection")
	}
	if len(m.picked) != 0 {
		t.Fatalf("expected picks cleared, got %d", len(m.picked))
	}
}
