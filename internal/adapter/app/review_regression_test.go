package app

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"logsee/internal/adapter/cli"
	"logsee/internal/adapter/tui"
	"logsee/internal/usecase"
	"os"
	"strings"
	"testing"
	"time"
)

func reviewState(t *testing.T) *loopState {
	t.Helper()
	p := writeLoopLog(t, "one\ntwo\nthree\nfour\nfive\n")
	s, e := newLoopState(context.Background(), usecase.InputSession{Mode: usecase.InputModeFile, SOTPath: p}, p, usecase.LogTypePlain, 100, 5, unboundedRecordLimit, "")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestReviewCopyOutsideViewport(t *testing.T) {
	s := reviewState(t)
	s.handleInput(context.Background(), eventLoopInput(loopEventSpacePick))
	s.handleInput(context.Background(), eventLoopInput(loopEventLast))
	if _, e := s.refreshFromSOT(context.Background()); e != nil {
		t.Fatal(e)
	}
	clipboard := &fakeLoopClipboard{}
	s.clipboard = clipboard
	s.copySelection(context.Background())
	got := usecase.CopyTextResult{Text: clipboard.text, Message: s.runtimeMessage}
	if got.Text != "one" {
		t.Fatalf("selected raw line 1, copied %q (%s); cached records=%v", got.Text, got.Message, s.records)
	}
}
func TestReviewPartialLineAppend(t *testing.T) {
	p := writeLoopLog(t, "hello")
	idx, e := buildFileLineIndex(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, []byte("hello world\nnext\n"), 0600); e != nil {
		t.Fatal(e)
	}
	idx, e = extendFileLineIndex(context.Background(), p, idx)
	if e != nil {
		t.Fatal(e)
	}
	got, e := readRawLogWindow(context.Background(), p, idx, 1, 1)
	if e != nil {
		t.Fatal(e)
	}
	if idx.totalLines() != 2 || len(got) != 1 || got[0].Text != "next" {
		t.Fatalf("after completing partial line: count=%d, raw line 2=%v", idx.totalLines(), got)
	}
}
func TestReviewFileFollowHasRefreshSource(t *testing.T) {
	s := reviewState(t)
	s.handleInput(context.Background(), eventLoopInput(loopEventLast))
	m := teaLoopModel{ctx: context.Background(), state: s}
	if m.Init() == nil {
		t.Fatal("file follow schedules no refresh command; only key input can refresh SOT")
	}
}

type reviewGatedInput struct {
	ready <-chan struct{}
	input io.Reader
}

func (r reviewGatedInput) Read(p []byte) (int, error) { <-r.ready; return r.input.Read(p) }

type reviewGatedOutput struct {
	ready  chan struct{}
	wrote  bool
	output strings.Builder
}

func (w *reviewGatedOutput) Write(p []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		close(w.ready)
	}
	return w.output.Write(p)
}
func TestReviewDetectTypeBeforeFirstStdioBatch(t *testing.T) {
	ready := make(chan struct{})
	output := &reviewGatedOutput{ready: ready}
	input := reviewGatedInput{ready: ready, input: strings.NewReader("11-20 12:34:56.789  123  456 E Tag: error\n")}
	err := Run(context.Background(), cli.Options{OutPath: t.TempDir() + "/sot.log"}, input, output, RunOptions{Interactive: true, HomeDir: t.TempDir(), Width: 200, Height: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.output.String(), "type:adb") {
		t.Fatalf("ADB log received after initial frame, but type stayed plain through EOF: %q", output.output.String())
	}
}
func TestReviewResizeLoadsExpandedViewport(t *testing.T) {
	s := reviewState(t)
	m := teaLoopModel{ctx: context.Background(), state: s}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	state := updated.(teaLoopModel).state
	if len(state.visibleLogLines()) != 5 {
		t.Fatalf("after resize viewport=%d, visible lines=%d, cache lines=%d", state.listHeight, len(state.visibleLogLines()), len(state.records))
	}
}

func TestReviewShiftHomeEndSelect(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyShiftHome, tea.KeyShiftEnd} {
		s := reviewState(t)
		s.handleInput(context.Background(), eventLoopInput(loopEventDown))
		s.handleInput(context.Background(), teaKeyToLoopInput(tea.KeyMsg(tea.Key{Type: key})))
		if !s.selection.HasSelection() {
			t.Errorf("%v does not select; mapped event=%v", key, teaKeyToLoopInput(tea.KeyMsg(tea.Key{Type: key})).event)
		}
	}
}

func TestWrappedSingleLineCanReachEnd(t *testing.T) {
	p := writeLoopLog(t, "ABCDEFGHIJKLMNO_END\n")
	s, err := newLoopState(context.Background(), usecase.InputSession{Mode: usecase.InputModeFile, SOTPath: p}, p, usecase.LogTypePlain, 9, 5, unboundedRecordLimit, "")
	if err != nil {
		t.Fatal(err)
	}
	s.handleInput(context.Background(), eventLoopInput(loopEventWrapToggle))
	for i := 0; i < 4; i++ {
		s.handleInput(context.Background(), eventLoopInput(loopEventDown))
	}
	if !strings.Contains(tui.FrameText(s.renderFrame()), "END") {
		t.Fatalf("cannot reach wrapped tail: %q", tui.FrameText(s.renderFrame()))
	}
}
func TestSelectionCountEqualsCopiedLines(t *testing.T) {
	s := reviewState(t)
	s.filterEditingText = "one | three"
	s.applyFilterEditingText()
	s.refreshFromSOT(context.Background())
	s.handleInput(context.Background(), eventLoopInput(loopEventShiftEnd))
	s.handleInput(context.Background(), eventLoopInput(loopEventSpacePick))
	clip := &fakeLoopClipboard{}
	s.clipboard = clip
	s.copySelection(context.Background())
	if clip.text != "one" || s.selection.PickedCount() != 1 {
		t.Fatalf("copy=%q selected=%d", clip.text, s.selection.PickedCount())
	}
}

func TestLiveTypeWaitsForCompleteSample(t *testing.T) {
	session, _, closeFile, err := openInputSession(usecase.InputRequest{OutPath: t.TempDir() + "/sot.log"})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFile()
	input, writer := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- session.ConsumeStdio(context.Background(), input, nil) }()
	writer.Write([]byte("E/Tag: pending"))
	deadline := time.Now().Add(time.Second)
	for session.Progress().PublishedBytes == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cfg := usecase.LogTypeConfig{ProbeLines: 1}
	got, err := session.DetectLogType(context.Background(), cfg)
	if err != nil || got != usecase.LogTypeAuto {
		t.Errorf("partial sample finalized: %s %v", got, err)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err = session.DetectLogType(context.Background(), cfg)
	if err != nil || got != usecase.LogTypeADB {
		t.Fatalf("EOF sample: %s %v", got, err)
	}
}
