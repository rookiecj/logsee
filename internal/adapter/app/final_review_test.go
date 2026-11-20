package app

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"logsee/internal/adapter/filesystem"
	"logsee/internal/port"
	"logsee/internal/usecase"
	"os"
	"reflect"
	"testing"
)

func reviewModel(t *testing.T) teaLoopModel {
	s := reviewState(t)
	s.background = true
	j := &sourceJobs{ctx: context.Background(), requests: make(chan sourceRequest, 1)}
	return teaLoopModel{ctx: context.Background(), state: s, jobs: j, requestID: 1, busy: true}
}
func TestFinalReviewSearchEntryRejectsOldSelection(t *testing.T) {
	m := reviewModel(t)
	m.state.pendingSelect = true
	selected := usecase.NewSelectionState()
	selected.TogglePicked(1)
	n, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = n.(teaLoopModel)
	n, _ = m.Update(sourceResult{ID: 1, Snapshot: port.LogSnapshot{Generation: 1, ByteLimit: 1}, Selection: selected})
	m = n.(teaLoopModel)
	if m.state.selection.HasSelection() {
		t.Fatal("old selection restored after / cleared selection")
	}
}
func TestFinalReviewRapidShiftRetainsAnchor(t *testing.T) {
	m := reviewModel(t)
	n, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m = n.(teaLoopModel)
	n, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m = n.(teaLoopModel)
	req := <-m.jobs.requests
	if req.Anchor != 1 {
		t.Fatalf("rapid ShiftDown twice lost raw line 1: anchor=%d", req.Anchor)
	}
}
func TestFinalReviewCanceledRealMetadataScan(t *testing.T) {
	p := writeLoopLog(t, "one\ntwo\n")
	src, _ := usecase.NewLogSource(filesystem.NewFileSource(p))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.Refresh(ctx, port.LogSnapshot{Generation: 1, ByteLimit: 8})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan completed: %v", err)
	}
}
func TestFinalReviewReplacementSurvivesSupersession(t *testing.T) {
	p := writeLoopLog(t, "old\n")
	src, _ := usecase.NewLogSource(filesystem.NewFileSource(p))
	j := &sourceJobs{path: p, source: src, filtered: usecase.NewFilteredSource(src), generation: 1}
	req := sourceRequest{ID: 1, LogType: usecase.LogTypePlain, Selection: usecase.NewSelectionState(), Navigation: usecase.NavigationOptions{ViewportHeight: 2}}
	first := j.execute(context.Background(), req)
	req.AcknowledgedGeneration = first.Snapshot.Generation
	replacement := p + ".new"
	os.WriteFile(replacement, []byte("new\n"), 0600)
	os.Rename(replacement, p)
	req.ID = 2
	dropped := j.execute(context.Background(), req)
	if !dropped.Replaced {
		t.Fatal("fixture")
	}
	// Request 2 was superseded before its result reached UI; execute request 3.
	req.ID = 3
	latest := j.execute(context.Background(), req)
	m := reviewModel(t)
	m.requestID = 3
	m.appliedGeneration = first.Snapshot.Generation
	m.state.selection.TogglePicked(1)
	n, _ := m.Update(latest)
	m = n.(teaLoopModel)
	if m.state.selection.HasSelection() {
		t.Fatal("selection from old inode retained after generation changed")
	}
}
func TestFinalReviewEscRejectsOldSearch(t *testing.T) {
	m := reviewModel(t)
	m.state.searchText = "two"
	m.state.pushMode(loopModeSearch)
	m.state.pendingSearch = true
	before := m.requestID
	n, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = n.(teaLoopModel)
	if m.requestID == before || m.state.pendingSearch {
		t.Fatal("Esc did not cancel outstanding search intent")
	}
}
func TestFinalReviewRapidShiftThenMoveKeepsOriginalSelection(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m = next.(teaLoopModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(teaLoopModel)
	req := <-m.jobs.requests
	worker := &sourceJobs{path: m.state.sourcePath, source: m.state.lineIndex.source, filtered: m.state.lineIndex.filtered, generation: 1}
	result := worker.execute(context.Background(), req)
	if result.Err != nil || result.Selection == nil || result.Selection.PickedCount() != 2 || result.Selection.IsPicked(3) {
		t.Fatalf("selection expanded on normal move: %+v err=%v", result.Selection, result.Err)
	}
	if _, active := result.Selection.Range(); active {
		t.Fatal("normal move retained active range")
	}
}
func TestFinalReviewRapidShiftThenSpaceTogglesAfterRange(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m = next.(teaLoopModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(teaLoopModel)
	req := <-m.jobs.requests
	worker := &sourceJobs{path: m.state.sourcePath, source: m.state.lineIndex.source, filtered: m.state.lineIndex.filtered, generation: 1}
	result := worker.execute(context.Background(), req)
	if result.Err != nil || result.Selection == nil || result.Selection.PickedCount() != 1 || !result.Selection.IsPicked(1) {
		t.Fatalf("lost pending range before Space: %+v err=%v", result.Selection, result.Err)
	}
}
func TestFinalReviewRapidSelectionOperationsKeepOrder(t *testing.T) {
	cases := []struct {
		name string
		keys []tea.KeyType
		want []int
	}{
		{"contract", []tea.KeyType{tea.KeyShiftDown, tea.KeyShiftUp}, []int{1, 2}},
		{"toggle_then_extend", []tea.KeyType{tea.KeyShiftDown, tea.KeySpace, tea.KeyShiftDown}, []int{1, 2, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := reviewModel(t)
			for _, key := range tc.keys {
				next, _ := m.Update(tea.KeyMsg{Type: key})
				m = next.(teaLoopModel)
			}
			req := <-m.jobs.requests
			worker := &sourceJobs{path: m.state.sourcePath, source: m.state.lineIndex.source, filtered: m.state.lineIndex.filtered, generation: 1}
			result := worker.execute(context.Background(), req)
			if result.Err != nil || result.Selection == nil || !reflect.DeepEqual(result.Selection.PickedLines(), tc.want) {
				t.Fatalf("got=%+v want=%v err=%v", result.Selection, tc.want, result.Err)
			}
		})
	}
}
