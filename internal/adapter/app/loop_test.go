package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"logsee/internal/adapter/cli"
	"logsee/internal/adapter/tui"
	"logsee/internal/usecase"
)

func TestInteractiveLoopRendersUntilQuit(t *testing.T) {
	logPath := writeLoopLog(t, "one\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("q"), &stdout, RunOptions{
		Width:       160,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	if count := strings.Count(output, "filter:\n"); count != 1 {
		t.Fatalf("render count = %d, want exactly initial render before quit; output %q", count, output)
	}
	if !strings.Contains(output, "1   one\n") {
		t.Fatalf("output %q does not contain initial log line", output)
	}
}

func TestInteractiveLoopNavigationRedrawsCursorStatus(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("jGq"), &stdout, RunOptions{
		Width:       180,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"lines:1/3",
		"lines:2/3",
		"lines:3/3",
		"follow:on",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestInteractiveLoopBookmarksAndJump(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("jmj1q"), &stdout, RunOptions{
		Width:       180,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"2 1 two\n",
		"bm:1",
		"lines:3/3",
		"lines:2/3",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestInteractiveLoopNamedNavigationKeys(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\nfour\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("G<Home><End>q"), &stdout, RunOptions{
		Width:       180,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"lines:4/4",
		"lines:1/4",
		"follow:on",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestInteractiveLoopNavigatesBeyondDefaultOutputCacheCapacity(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 5
	logPath := writeLoopLog(t, numberedLogLines(lineCount))

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("Gq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"10005   line-10005\n",
		"lines:10005/10005",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
}

func TestLoopStateUsesViewportWindowForLargeUnfilteredFile(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 5
	logPath := writeLoopLog(t, numberedLogLines(lineCount))
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}

	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 6, unboundedRecordLimit)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	if got, want := state.totalRawLines, lineCount; got != want {
		t.Fatalf("total raw lines = %d, want %d", got, want)
	}
	if got, want := len(state.rawLogs), state.listHeight; got != want {
		t.Fatalf("raw logs retained = %d, want viewport window %d", got, want)
	}
	if got := state.rawLogs[len(state.rawLogs)-1].RawLineNumber; got != state.listHeight {
		t.Fatalf("initial window last raw line = %d, want %d", got, state.listHeight)
	}
	if len(state.records) != len(state.rawLogs) {
		t.Fatalf("records retained = %d, want same as raw window %d", len(state.records), len(state.rawLogs))
	}
}

func TestLoopStateReloadsWindowAtTailWithoutRetainingAllRows(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 5
	logPath := writeLoopLog(t, numberedLogLines(lineCount))
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}

	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 6, unboundedRecordLimit)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	state.handleInput(context.Background(), eventLoopInput(loopEventLast))
	if _, err := state.refreshFromSOT(context.Background()); err != nil {
		t.Fatalf("refresh after G: %v", err)
	}

	if got, want := len(state.rawLogs), state.listHeight; got != want {
		t.Fatalf("raw logs retained after G = %d, want viewport window %d", got, want)
	}
	if got, want := state.rawLogs[0].RawLineNumber, lineCount-state.listHeight+1; got != want {
		t.Fatalf("tail window first raw line = %d, want %d", got, want)
	}
	if got, want := state.cursorRawLine(), lineCount; got != want {
		t.Fatalf("cursor raw line after G = %d, want %d", got, want)
	}
	frame := tui.FrameText(state.renderFrame())
	if !strings.Contains(frame, "10005   line-10005\n") {
		t.Fatalf("tail frame %q does not contain final line", frame)
	}
}

func TestLoopStateAppliesLargeFileFilterWithBoundedOutputCache(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 25
	logPath := writeLoopLog(t, matchingLogLines(lineCount, "match"))
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}

	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 6, unboundedRecordLimit)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	applyLoopInputs(t, state, ":match\n")

	if got, want := state.totalRawLines, lineCount; got != want {
		t.Fatalf("total raw lines = %d, want %d", got, want)
	}
	if got := len(state.rawLogs); got != 0 {
		t.Fatalf("raw logs retained with active filter = %d, want 0", got)
	}
	if got, max := len(state.records), usecase.DefaultOutputLogCacheCapacity; got != max {
		t.Fatalf("filtered records retained = %d, want bounded cache %d", got, max)
	}
	if got := state.windowStartOutputIndex; got != 0 {
		t.Fatalf("filtered window start = %d, want 0", got)
	}
	logs := state.visibleLogLines()
	if len(logs) == 0 {
		t.Fatal("visible logs = none, want first filtered records")
	}
	if got, want := logs[0].RawLineNumber, 1; got != want {
		t.Fatalf("first visible filtered raw line = %d, want %d", got, want)
	}
}

func TestLoopStateReloadsBoundedFilteredWindowAtTail(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 25
	logPath := writeLoopLog(t, matchingLogLines(lineCount, "match"))
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}

	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 6, unboundedRecordLimit)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	applyLoopInputs(t, state, ":match\n")
	state.handleInput(context.Background(), eventLoopInput(loopEventLast))
	if _, err := state.refreshFromSOT(context.Background()); err != nil {
		t.Fatalf("refresh after filtered G: %v", err)
	}

	if got := len(state.rawLogs); got != 0 {
		t.Fatalf("raw logs retained after filtered G = %d, want 0", got)
	}
	if got, max := len(state.records), usecase.DefaultOutputLogCacheCapacity; got > max {
		t.Fatalf("filtered records retained after G = %d, want <= %d", got, max)
	}
	if got, want := state.cursorRawLine(), lineCount; got != want {
		t.Fatalf("cursor raw line after filtered G = %d, want %d", got, want)
	}
	frame := tui.FrameText(state.renderFrame())
	if !strings.Contains(frame, "10025   match line-10025\n") {
		t.Fatalf("filtered tail frame %q does not contain final matching line", frame)
	}
	if !strings.Contains(frame, "lines:10025/10025") {
		t.Fatalf("filtered tail frame %q does not show final raw line status", frame)
	}
}

func TestLoopStateSkipsFilteredSOTRescanWhenViewportIsCached(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 25
	logPath := writeLoopLog(t, matchingLogLines(lineCount, "match"))
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}

	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 6, unboundedRecordLimit)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	applyLoopInputs(t, state, ":match\n")
	state.handleInput(context.Background(), eventLoopInput(loopEventDown))

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := state.refreshFromSOT(canceled); err != nil {
		t.Fatalf("cached filtered refresh should not rescan SOT with canceled context: %v", err)
	}
	if got, want := state.cursorRawLine(), 2; got != want {
		t.Fatalf("cursor raw line after cached filtered movement = %d, want %d", got, want)
	}
}

func TestLoopStateClearingFilterReturnsToUnfilteredFileWindow(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 25
	logPath := writeLoopLog(t, matchingLogLines(lineCount, "match"))
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}

	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 6, unboundedRecordLimit)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	applyLoopInputs(t, state, ":match\n:"+strings.Repeat("\x7f", len("match"))+"\n")
	if _, err := state.refreshFromSOT(context.Background()); err != nil {
		t.Fatalf("refresh after clearing filter: %v", err)
	}

	if got := state.filterText; got != "" {
		t.Fatalf("filter text = %q, want empty", got)
	}
	if got, want := len(state.rawLogs), state.listHeight; got != want {
		t.Fatalf("raw logs retained after clearing filter = %d, want viewport window %d", got, want)
	}
	if got, want := len(state.records), state.listHeight; got != want {
		t.Fatalf("records retained after clearing filter = %d, want viewport window %d", got, want)
	}
	if got := state.windowStartOutputIndex; got != 0 {
		t.Fatalf("unfiltered window start after clearing filter = %d, want 0", got)
	}
}

func TestInteractiveLoopNoVisibleLogsKeepsStatusbarAtBottom(t *testing.T) {
	logPath := writeLoopLog(t, "alpha\nbeta\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":nomatch\n"), &stdout, RunOptions{
		Width:       120,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	if got, want := len(lines), 8; got != want {
		t.Fatalf("final frame lines = %#v, count = %d, want %d", lines, got, want)
	}
	if got, want := lines[0], "filter:nomatch"; got != want {
		t.Fatalf("filter row = %q, want %q", got, want)
	}
	if got, want := lines[1], "search:"; got != want {
		t.Fatalf("search row = %q, want %q", got, want)
	}
	for row := 2; row <= 6; row++ {
		if lines[row] != "" {
			t.Fatalf("empty log viewport row %d = %q, want blank row", row, lines[row])
		}
	}
	if !strings.HasPrefix(lines[7], "lines:") {
		t.Fatalf("bottom row = %q, want statusbar", lines[7])
	}
}

func TestInteractiveLoopHelpOpensAndClosesWithoutMovingCursor(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("<F1><Esc>q"), &stdout, RunOptions{
		Width:       160,
		Height:      12,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Help\n") {
		t.Fatalf("output %q does not contain help modal", output)
	}
	if strings.Count(output, "lines:1/2") < 3 {
		t.Fatalf("cursor status should remain on line 1 across help open/close; output %q", output)
	}
}

func TestInteractiveLoopQuestionMarkDoesNotOpenHelpFromLogList(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("?"), &stdout, RunOptions{
		Width:       160,
		Height:      12,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if output := stdout.String(); strings.Contains(output, "Help\n") {
		t.Fatalf("output %q unexpectedly contains help modal", output)
	}
}

func TestInteractiveLoopFilterInputQuestionMarkIsTextAndF1OpensPopup(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":?<F1><Esc>"), &stdout, RunOptions{
		Width:       160,
		Height:      12,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Help\n") {
		t.Fatalf("output %q does not contain help modal", output)
	}
	frame := lastRenderedFrame(output)
	if !strings.Contains(frame, "filter:> ?_\n") {
		t.Fatalf("final frame %q does not retain question mark in filter input", frame)
	}
	if strings.Contains(frame, "Help\n") {
		t.Fatalf("final frame %q still contains help modal", frame)
	}
}

func TestInteractiveLoopSearchInputQuestionMarkIsTextAndF1OpensPopup(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/?<F1><Esc>"), &stdout, RunOptions{
		Width:       160,
		Height:      12,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Help\n") {
		t.Fatalf("output %q does not contain help modal", output)
	}
	frame := lastRenderedFrame(output)
	if !strings.Contains(frame, "search:> ?_\n") {
		t.Fatalf("final frame %q does not retain question mark in search input", frame)
	}
	if strings.Contains(frame, "Help\n") {
		t.Fatalf("final frame %q still contains help modal", frame)
	}
}

func TestInteractiveLoopRuntimeFilterAppliesOnEnter(t *testing.T) {
	logPath := writeLoopLog(t, "info ready\nerror database\ndebug done\nerror timeout\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":error\nq"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"filter:error\n",
		"2   error database\n",
		"4   error timeout\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	for _, unwanted := range []string{
		"1   info ready\n",
		"3   debug done\n",
		"filter:on",
	} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("final frame %q contains filtered-out line %q", frame, unwanted)
		}
	}
}

func TestInteractiveLoopInvalidFilterPreservesPreviousFilterAndShowsError(t *testing.T) {
	logPath := writeLoopLog(t, "info ready\nerror database\ndebug done\nerror timeout\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":error\n:"+strings.Repeat("\x7f", len("error"))+"broken \"quote\n"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"filter:> broken \"quote_ ! unterminated quoted filter token\n",
		"2   error database\n",
		"4   error timeout\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	for _, unwanted := range []string{
		"1   info ready\n",
		"3   debug done\n",
		"msg:unterminated quoted filter token",
		"filter:on",
	} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("final frame %q contains %q", frame, unwanted)
		}
	}
}

func TestInteractiveLoopFilterEscCancelsEditingAndPreservesAppliedFilter(t *testing.T) {
	logPath := writeLoopLog(t, "info ready\nerror database\ndebug done\nerror timeout\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":error\n:debug<Esc>q"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"filter:error\n",
		"2   error database\n",
		"4   error timeout\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "3   debug done\n") {
		t.Fatalf("final frame %q applied canceled filter edit", frame)
	}
}

func TestInteractiveLoopFilterFocusRetainsAppliedValueWithCursorAtEnd(t *testing.T) {
	logPath := writeLoopLog(t, "info ready\nerror database\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":error\n:"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "filter:> error_\n") {
		t.Fatalf("final frame %q does not retain applied filter with cursor at end", frame)
	}
}

func TestInteractiveLoopSearchFocusRetainsAppliedValueWithCursorAtEnd(t *testing.T) {
	logPath := writeLoopLog(t, "idle ready\ntimeout database\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/timeout\n/"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "search:> timeout_\n") {
		t.Fatalf("final frame %q does not retain applied search with cursor at end", frame)
	}
}

func TestInteractiveLoopFilterDownSeedsSearchInputFromAppliedValue(t *testing.T) {
	logPath := writeLoopLog(t, "idle ready\ntimeout database\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/timeout\n:<Down>"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "search:> timeout_\n") {
		t.Fatalf("final frame %q does not seed search input from applied value after Down", frame)
	}
}

func TestInteractiveLoopSearchUpSeedsFilterInputFromAppliedValue(t *testing.T) {
	logPath := writeLoopLog(t, "info ready\nerror database\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":error\n/<Up>"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "filter:> error_\n") {
		t.Fatalf("final frame %q does not seed filter input from applied value after Up", frame)
	}
}

func TestInteractiveLoopRuntimeSearchAppliesOnEnter(t *testing.T) {
	logPath := writeLoopLog(t, "idle ready\ntimeout database\nstill visible\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/timeout\nq"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"search:timeout\n",
		"1   idle ready\n",
		"2   timeout database\n",
		"3   still visible\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
}

func TestInteractiveLoopCopyCurrentLineWritesClipboardAndStatus(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")
	clipboard := &fakeLoopClipboard{}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("jcq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
		Clipboard:   clipboard,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if got, want := clipboard.text, "two"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "msg:1 line copied") {
		t.Fatalf("final frame %q does not contain copy status", frame)
	}
}

func TestInteractiveLoopSpacePickCopiesPickedLines(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")
	clipboard := &fakeLoopClipboard{}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(" jj cq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
		Clipboard:   clipboard,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if got, want := clipboard.text, "one\nthree"; got != want {
		t.Fatalf("clipboard text = %q, want picked raw-line order %q", got, want)
	}
	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"1   one\n",
		"3   three\n",
		"sel:2",
		"msg:2 lines copied",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "picked:") {
		t.Fatalf("final frame %q must combine picked status into sel", frame)
	}
	if strings.Contains(frame, "[picked]") {
		t.Fatalf("final frame %q must not include picked text prefix", frame)
	}
}

func TestInteractiveLoopShiftDownSelectsRangeAndCopiesSelectedLines(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")
	clipboard := &fakeLoopClipboard{}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("j<ShiftDown>c"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
		Clipboard:   clipboard,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if got, want := clipboard.text, "two\nthree"; got != want {
		t.Fatalf("clipboard text = %q, want selected range %q", got, want)
	}
	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"2   two\n",
		"3   three\n",
		"sel:2",
		"msg:2 lines copied",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "picked:") {
		t.Fatalf("final frame %q must not contain picked status field", frame)
	}
	if strings.Contains(frame, "[sel]") {
		t.Fatalf("final frame %q must not include range selection text prefix", frame)
	}
}

func TestInteractiveLoopShiftSelectionPersistsAfterNormalMovementAndCopiesSelectedLines(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\nfour\n")
	clipboard := &fakeLoopClipboard{}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("j<ShiftDown>jc"), &stdout, RunOptions{
		Width:       220,
		Height:      7,
		HomeDir:     t.TempDir(),
		Interactive: true,
		Clipboard:   clipboard,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if got, want := clipboard.text, "two\nthree"; got != want {
		t.Fatalf("clipboard text = %q, want persisted Shift selection %q", got, want)
	}
	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"2   two\n",
		"3   three\n",
		"sel:2",
		"msg:2 lines copied",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "picked:") {
		t.Fatalf("final frame %q must combine persisted Shift selection status into sel", frame)
	}
}

func TestInteractiveLoopSpaceReselectsShiftSelectedLineToDeselect(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")
	clipboard := &fakeLoopClipboard{}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("j<ShiftDown>k c"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
		Clipboard:   clipboard,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if got, want := clipboard.text, "three"; got != want {
		t.Fatalf("clipboard text = %q, want only the still-selected Shift line %q", got, want)
	}
	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"2   two\n",
		"3   three\n",
		"sel:1",
		"msg:1 line copied",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "picked:") {
		t.Fatalf("final frame %q must combine Space re-selection status into sel", frame)
	}
}

func TestInteractiveLoopCopyKeyInFilterInputEditsTextAndDoesNotWriteClipboard(t *testing.T) {
	logPath := writeLoopLog(t, "copy me\nother\n")
	clipboard := &fakeLoopClipboard{}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":c\nq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
		Clipboard:   clipboard,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	if clipboard.text != "" {
		t.Fatalf("clipboard text = %q, want no write from filter input", clipboard.text)
	}
	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "filter:c\n") {
		t.Fatalf("final frame %q does not preserve c as filter input", frame)
	}
	if strings.Contains(frame, "copied") {
		t.Fatalf("final frame %q contains unexpected copy status", frame)
	}
}

func TestInteractiveLoopRuntimeSearchAddsHighlightMetadataOverFilteredRecords(t *testing.T) {
	logPath := writeLoopLog(t, "drop timeout\nkeep idle\nkeep timeout\n")
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}
	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 220, 8, 100)
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}

	applyLoopInputs(t, state, ":keep\n/timeout\n")

	logs := state.visibleLogLines()
	if len(logs) != 2 {
		t.Fatalf("visible logs = %#v, want 2 filtered records", logs)
	}
	if got, want := logs[0].RawLineNumber, 2; got != want {
		t.Fatalf("first filtered raw line = %d, want %d", got, want)
	}
	if len(logs[0].Highlights) != 0 {
		t.Fatalf("first filtered log highlights = %#v, want none", logs[0].Highlights)
	}
	wantHighlights := []usecase.HighlightRange{{Start: 5, End: 12}}
	if !reflect.DeepEqual(logs[1].Highlights, wantHighlights) {
		t.Fatalf("second filtered log highlights = %#v, want %#v", logs[1].Highlights, wantHighlights)
	}
}

func TestInteractiveLoopSearchEscCancelsEditingAndPreservesAppliedSearch(t *testing.T) {
	logPath := writeLoopLog(t, "idle ready\ntimeout database\nstill visible\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/timeout\n/nope<Esc>q"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "search:timeout\n") {
		t.Fatalf("final frame %q does not preserve applied search input", frame)
	}
	if strings.Contains(frame, "search:on") {
		t.Fatalf("final frame %q must not contain removed search:on status", frame)
	}
}

func TestInteractiveLoopModeStackEscPopsSelectionThenSearch(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/two\nj<ShiftDown><Esc><Esc>q"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "search:\n") {
		t.Fatalf("final frame %q did not clear search after second Esc", frame)
	}
	for _, unwanted := range []string{"search:two\n", "sel:"} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("final frame %q contains popped mode state %q", frame, unwanted)
		}
	}
}

func TestInteractiveLoopModeStackEscPopsSearchBeforeSelection(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("j<ShiftDown>/two\n<Esc>q"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"search:\n",
		"sel:2",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "search:two\n") {
		t.Fatalf("final frame %q did not clear search after top search mode pop", frame)
	}
}

func TestInteractiveLoopModeStackSecondEscClearsRemainingSelection(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("j<ShiftDown>/two\n<Esc><Esc>q"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "search:\n") {
		t.Fatalf("final frame %q did not clear search", frame)
	}
	for _, unwanted := range []string{"search:two\n", "sel:"} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("final frame %q contains popped mode state %q", frame, unwanted)
		}
	}
}

func TestInteractiveLoopSearchNavigationMovesThroughFilteredMatchesWithoutWrapping(t *testing.T) {
	logPath := writeLoopLog(t, "timeout first\nidle\nmatch timeout second\nidle\nmatch timeout third\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/timeout\nnnpppq"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"lines:3/5",
		"lines:5/5",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}

	frame := lastRenderedFrame(output)
	if !strings.Contains(frame, "lines:1/5") {
		t.Fatalf("final frame %q should remain at first match after previous search boundary", frame)
	}
	if !strings.Contains(frame, "1   timeout first\n") {
		t.Fatalf("final frame %q should show first matching line after p navigation", frame)
	}
}

func TestInteractiveLoopSearchNavigationShowsNoMatchAtBoundary(t *testing.T) {
	logPath := writeLoopLog(t, "timeout first\nidle\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/timeout\nn"), &stdout, RunOptions{
		Width:       220,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	if !strings.Contains(frame, "msg:no match") {
		t.Fatalf("final frame %q does not show no match status message", frame)
	}
}

func TestInteractiveLoopFileSearchFindsNextMatchOutsideVisibleWindow(t *testing.T) {
	logPath := writeLoopLog(t, strings.Join([]string{
		"line one",
		"line two",
		"line three",
		"line four",
		"line five",
		"target line six",
		"line seven",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/target\nnq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"6   target line six\n",
		"lines:6/7",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "msg:no match") {
		t.Fatalf("final frame %q reported no match despite off-screen file match", frame)
	}
}

func TestInteractiveLoopFileSearchFindsPreviousMatchOutsideVisibleWindow(t *testing.T) {
	logPath := writeLoopLog(t, strings.Join([]string{
		"line one",
		"target line two",
		"line three",
		"line four",
		"line five",
		"line six",
		"line seven",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("G/target\npq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"2   target line two\n",
		"lines:2/7",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "msg:no match") {
		t.Fatalf("final frame %q reported no match despite previous off-screen file match", frame)
	}
}

func TestInteractiveLoopFilteredFileSearchFindsNextMatchOutsideCachedWindow(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 5
	logPath := writeLoopLog(t, matchingLogLinesWithTarget(lineCount, "keep", lineCount))

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":keep\n/target\nnq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"10005   keep target line-10005\n",
		"lines:10005/10005",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "msg:no match") {
		t.Fatalf("final frame %q reported no match despite off-cache filtered file match", frame)
	}
}

func TestInteractiveLoopFilteredFileSearchFindsPreviousMatchOutsideCachedWindow(t *testing.T) {
	const lineCount = usecase.DefaultOutputLogCacheCapacity + 5
	logPath := writeLoopLog(t, matchingLogLinesWithTarget(lineCount, "keep", 2))

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":keep\nG/target\npq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"2   keep target line-00002\n",
		"lines:2/10005",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "msg:no match") {
		t.Fatalf("final frame %q reported no match despite previous off-cache filtered file match", frame)
	}
}

func TestInteractiveLoopWrapToggleRendersWrappedRowsAndStatus(t *testing.T) {
	longLine := strings.Repeat("a", 80)
	logPath := writeLoopLog(t, longLine+"\nsecond\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("wq"), &stdout, RunOptions{
		Width:       80,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"1   " + strings.Repeat("a", 76) + "\n",
		"    aaaa\n",
		"2   second\n",
		"wrap:on",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
}

func TestInteractiveLoopHorizontalScrollRightAndLeftWhenWrapOff(t *testing.T) {
	logPath := writeLoopLog(t, "abcdefghij\nsecond\nxy\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("llhq"), &stdout, RunOptions{
		Width:       10,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"1   bcdefg\n",
		"2   econd\n",
		"3   y\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "1   abcdef\n") || strings.Contains(frame, "1   cdefgh\n") {
		t.Fatalf("final frame %q did not settle at horizontal offset 1 after llh", frame)
	}
}

func TestInteractiveLoopHorizontalScrollIgnoredWhenWrapOn(t *testing.T) {
	logPath := writeLoopLog(t, "abcdefghij\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("wlq"), &stdout, RunOptions{
		Width:       10,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"1   abcdef\n",
		"    ghij\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "1   bcdefg\n") {
		t.Fatalf("final frame %q horizontally scrolled while wrap was on", frame)
	}
}

func TestInteractiveLoopHorizontalScrollKeysInFilterAndSearchInputEditText(t *testing.T) {
	logPath := writeLoopLog(t, "hl value\nother\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":hl\n/hl\nq"), &stdout, RunOptions{
		Width:       40,
		Height:      8,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"filter:hl\n",
		"search:hl\n",
		"1   hl value\n",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
}

func TestInteractiveLoopWrapKeyInFilterInputEditsTextInsteadOfToggling(t *testing.T) {
	logPath := writeLoopLog(t, "w value\nother\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(":w\nq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"filter:w\n",
		"1   w value\n",
		"wrap:off",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "wrap:on") {
		t.Fatalf("final frame %q toggled wrap while filter input had focus", frame)
	}
}

func TestInteractiveLoopWrapKeyInSearchInputEditsTextInsteadOfToggling(t *testing.T) {
	logPath := writeLoopLog(t, "w value\nother\n")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader("/w\nq"), &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"search:w\n",
		"wrap:off",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	if strings.Contains(frame, "wrap:on") {
		t.Fatalf("final frame %q toggled wrap while search input had focus", frame)
	}
}

func TestRunInteractiveSTDIOStreamsNewLinesThroughSOTBeforeRedraw(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "session.log")
	stdin := newControlledReader()
	keyInput := &scriptedReader{
		data: []byte("?q"),
		before: map[int]func() error{
			0: func() error {
				stdin.WriteString("alpha\nbeta\n")
				stdin.Close()
				return nil
			},
			1: func() error {
				return waitForFileContent(outPath, "alpha\nbeta\n")
			},
		},
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{
		InputPath: "-",
		OutPath:   outPath,
		LogType:   "plain",
	}, stdin, &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		WorkDir:     dir,
		Interactive: true,
		KeyInput:    keyInput,
	})
	if err != nil {
		t.Fatalf("run interactive stdio: %v", err)
	}

	persisted, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read SOT: %v", err)
	}
	if got, want := string(persisted), "alpha\nbeta\n"; got != want {
		t.Fatalf("persisted SOT = %q, want %q", got, want)
	}

	frames := renderedFrames(stdout.String())
	if len(frames) == 0 {
		t.Fatalf("rendered frames = none; output %q", stdout.String())
	}
	if strings.Contains(frames[0], "alpha") || strings.Contains(frames[0], "beta") {
		t.Fatalf("initial frame %q showed STDIO before incremental SOT append", frames[0])
	}
	if !strings.Contains(stdout.String(), "1   alpha\n") || !strings.Contains(stdout.String(), "2   beta\n") {
		t.Fatalf("output %q does not show streamed SOT-backed lines", stdout.String())
	}
}

func TestRunInteractiveSTDIOWithDefaultKeyInputWaitsForStreamEOFAndFinalRefresh(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "session.log")
	stdin := newControlledReader()
	defer stdin.Close()

	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), cli.Options{
			InputPath: "-",
			OutPath:   outPath,
			LogType:   "plain",
		}, stdin, &stdout, RunOptions{
			Width:       220,
			Height:      6,
			HomeDir:     t.TempDir(),
			WorkDir:     dir,
			Interactive: true,
		})
	}()

	assertRunStillActive(t, done, 100*time.Millisecond)

	stdin.WriteString("alpha\nbeta\n")
	stdin.Close()
	if err := waitRunDone(done, 2*time.Second); err != nil {
		t.Fatalf("run interactive stdio: %v", err)
	}

	persisted, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read SOT: %v", err)
	}
	if got, want := string(persisted), "alpha\nbeta\n"; got != want {
		t.Fatalf("persisted SOT = %q, want %q", got, want)
	}

	output := stdout.String()
	for _, want := range []string{
		"1   alpha\n",
		"2   beta\n",
		"in:stdio:eof",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestInteractiveLoopRefreshWithFollowOnPinsToNewestOutputLine(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\n")
	keyInput := &scriptedReader{
		data: []byte("G?q"),
		before: map[int]func() error{
			1: func() error {
				return appendToLoopLog(logPath, "four\nfive\n")
			},
		},
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, keyInput, &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"3   three\n",
		"5   five\n",
		"lines:5/5",
		"follow:on",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestInteractiveLoopRefreshWithFollowOffPreservesViewportAndCursor(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\nthree\nfour\nfive\n")
	keyInput := &scriptedReader{
		data: []byte("Gk?q"),
		before: map[int]func() error{
			2: func() error {
				return appendToLoopLog(logPath, "six\nseven\n")
			},
		},
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, keyInput, &stdout, RunOptions{
		Width:       220,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	frame := lastRenderedFrame(stdout.String())
	for _, want := range []string{
		"3   three\n",
		"4   four\n",
		"5   five\n",
		"lines:4/7",
		"follow:off",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("final frame %q does not contain %q", frame, want)
		}
	}
	for _, unwanted := range []string{
		"6   six\n",
		"7   seven\n",
		"lines:7/7",
	} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("final frame %q unexpectedly contains %q", frame, unwanted)
		}
	}
}

func TestInteractiveLoopRefreshReadErrorIsReportedAndStops(t *testing.T) {
	logPath := writeLoopLog(t, "one\n")
	keyInput := &scriptedReader{
		data: []byte("?"),
		before: map[int]func() error{
			0: func() error {
				return os.Remove(logPath)
			},
		},
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, keyInput, &stdout, RunOptions{
		Width:       1000,
		Height:      6,
		HomeDir:     t.TempDir(),
		Interactive: true,
	})
	if err == nil {
		t.Fatal("run interactive error = nil, want runtime read error")
	}
	for _, want := range []string{"refresh SOT", "rendered SOT"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
	for _, want := range []string{"msg:refresh SOT", "rendered SOT"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output %q does not contain visible runtime error %q", stdout.String(), want)
		}
	}
}

func writeLoopLog(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "loop.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write loop log: %v", err)
	}
	return path
}

func numberedLogLines(count int) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&builder, "line-%05d\n", i)
	}
	return builder.String()
}

func matchingLogLines(count int, prefix string) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&builder, "%s line-%05d\n", prefix, i)
	}
	return builder.String()
}

func matchingLogLinesWithTarget(count int, prefix string, targetLine int) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		if i == targetLine {
			fmt.Fprintf(&builder, "%s target line-%05d\n", prefix, i)
			continue
		}
		fmt.Fprintf(&builder, "%s line-%05d\n", prefix, i)
	}
	return builder.String()
}

func appendToLoopLog(path string, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open loop log for append: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("append loop log: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync loop log: %w", err)
	}
	return nil
}

func applyLoopInputs(t *testing.T, state *loopState, input string) {
	t.Helper()

	reader := newLoopEventReader(strings.NewReader(input))
	for {
		next, err := reader.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("read loop input: %v", err)
		}
		_, quit := state.handleInput(context.Background(), next)
		if quit {
			t.Fatal("loop input unexpectedly requested quit")
		}
	}
}

type scriptedReader struct {
	data   []byte
	before map[int]func() error
	index  int
}

func (r *scriptedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.index >= len(r.data) {
		return 0, io.EOF
	}
	if action := r.before[r.index]; action != nil {
		if err := action(); err != nil {
			return 0, err
		}
	}
	p[0] = r.data[r.index]
	r.index++
	return 1, nil
}

type controlledReader struct {
	chunks    chan []byte
	buffer    []byte
	closeOnce sync.Once
}

type fakeLoopClipboard struct {
	text string
}

func (w *fakeLoopClipboard) WriteText(_ context.Context, text string) error {
	w.text = text
	return nil
}

func newControlledReader() *controlledReader {
	return &controlledReader{chunks: make(chan []byte, 16)}
}

func (r *controlledReader) WriteString(text string) {
	r.chunks <- []byte(text)
}

func (r *controlledReader) Close() {
	r.closeOnce.Do(func() {
		close(r.chunks)
	})
}

func (r *controlledReader) Read(p []byte) (int, error) {
	for len(r.buffer) == 0 {
		chunk, ok := <-r.chunks
		if !ok {
			return 0, io.EOF
		}
		r.buffer = chunk
	}
	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func assertRunStillActive(t *testing.T, done <-chan error, duration time.Duration) {
	t.Helper()

	select {
	case err := <-done:
		t.Fatalf("run returned before STDIO EOF with error %v", err)
	case <-time.After(duration):
	}
}

func waitRunDone(done <-chan error, duration time.Duration) error {
	select {
	case err := <-done:
		return err
	case <-time.After(duration):
		return fmt.Errorf("timed out waiting for run to finish")
	}
}

func waitForFileContent(path string, want string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(content), want) {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %q in %s", want, path)
}

func renderedFrames(output string) []string {
	prefixed := "\n" + output
	var frames []string
	for {
		start := strings.Index(prefixed, "\nfilter:")
		if start < 0 {
			return frames
		}
		prefixed = prefixed[start+1:]
		next := strings.Index(prefixed[1:], "\nfilter:")
		if next < 0 {
			frames = append(frames, prefixed)
			return frames
		}
		frames = append(frames, prefixed[:next+1])
		prefixed = prefixed[next+1:]
	}
}

func lastRenderedFrame(output string) string {
	prefixed := "\n" + output
	index := strings.LastIndex(prefixed, "\nfilter:")
	if index < 0 {
		return output
	}
	return prefixed[index+1:]
}
