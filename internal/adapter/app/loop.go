package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"logsee/internal/adapter/tui"
	"logsee/internal/port"
	"logsee/internal/usecase"
)

type loopEvent int

const (
	loopEventUnknown loopEvent = iota
	loopEventQuit
	loopEventCtrlC
	loopEventUp
	loopEventDown
	loopEventPageUp
	loopEventPageDown
	loopEventHome
	loopEventEnd
	loopEventLast
	loopEventBookmarkToggle
	loopEventHelpToggle
	loopEventEsc
	loopEventBookmark1
	loopEventBookmark2
	loopEventBookmark3
	loopEventBookmark4
	loopEventBookmark5
	loopEventBookmark6
	loopEventBookmark7
	loopEventBookmark8
	loopEventBookmark9
	loopEventEnter
	loopEventFilterInput
	loopEventSearchInput
	loopEventSearchNext
	loopEventSearchPrevious
	loopEventBackspace
	loopEventText
	loopEventWrapToggle
	loopEventSpacePick
	loopEventCopy
	loopEventShiftUp
	loopEventShiftDown
	loopEventHorizontalLeft
	loopEventHorizontalRight
)

type loopInput struct {
	event loopEvent
	text  string
}

func runInteractiveLoop(ctx context.Context, session usecase.InputSession, sourcePath string, logType usecase.LogType, width, height int, keyInput io.Reader, output io.Writer, stream *stdioStream, clipboardWriter port.ClipboardWriter, homeDir string) error {
	state, err := newLoopState(ctx, session, sourcePath, logType, width, height, unboundedRecordLimit, homeDir)
	if err != nil {
		return err
	}
	state.clipboard = clipboardWriter
	if stream != nil {
		state.readState = tui.ReadStateRead
		defer stream.cancel()
	}
	if err := writeFrame(output, state.renderFrame()); err != nil {
		return err
	}

	var eventResults <-chan loopReadResult
	keyInputClosed := keyInput == nil
	if keyInput != nil {
		eventResults = startLoopEventReader(keyInput)
	}
	var refresh <-chan struct{}
	var streamDone <-chan error
	if stream != nil {
		refresh = stream.refresh
		streamDone = stream.done
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-refresh:
			if !ok {
				refresh = nil
				continue
			}
			redraw, err := state.refreshFromSOT(ctx)
			if err != nil {
				return renderLoopRuntimeError(output, state, err)
			}
			if redraw {
				if err := writeFrame(output, state.renderFrame()); err != nil {
					return err
				}
			}
		case err, ok := <-streamDone:
			streamDone = nil
			refresh = nil
			state.readState = tui.ReadStateEOF
			if !ok {
				if keyInputClosed {
					return nil
				}
				continue
			}
			if err != nil {
				return renderLoopRuntimeError(output, state, fmt.Errorf("stream stdio: %w", err))
			}
			redraw, refreshErr := state.refreshFromSOT(ctx)
			if refreshErr != nil {
				return renderLoopRuntimeError(output, state, refreshErr)
			}
			if redraw || session.Mode == usecase.InputModeStdio {
				if err := writeFrame(output, state.renderFrame()); err != nil {
					return err
				}
			}
			if keyInputClosed {
				return nil
			}
		case result, ok := <-eventResults:
			if !ok {
				eventResults = nil
				keyInputClosed = true
				if streamDone == nil {
					return nil
				}
				continue
			}
			ack := func() {
				if result.ack != nil {
					close(result.ack)
					result.ack = nil
				}
			}
			if result.err == io.EOF {
				ack()
				eventResults = nil
				keyInputClosed = true
				if streamDone == nil {
					return nil
				}
				continue
			}
			if result.err != nil {
				ack()
				return result.err
			}
			inputRedraw, quit := state.handleInput(ctx, result.input)
			redraw, err := state.refreshFromSOT(ctx)
			ack()
			if err != nil {
				return renderLoopRuntimeError(output, state, err)
			}
			if quit {
				return nil
			}
			if inputRedraw {
				redraw = true
			}
			if redraw {
				if err := writeFrame(output, state.renderFrame()); err != nil {
					return err
				}
			}
		}
	}
}

type loopReadResult struct {
	input loopInput
	err   error
	ack   chan struct{}
}

func startLoopEventReader(input io.Reader) <-chan loopReadResult {
	results := make(chan loopReadResult, 1)
	go func() {
		defer close(results)
		reader := newLoopEventReader(input)
		for {
			next, err := reader.Next()
			var ack chan struct{}
			if err == nil {
				ack = make(chan struct{})
			}
			results <- loopReadResult{input: next, err: err, ack: ack}
			if err != nil {
				return
			}
			<-ack
		}
	}()
	return results
}

func (s *loopState) handleInput(ctx context.Context, input loopInput) (bool, bool) {
	if input.event == loopEventCtrlC {
		return false, true
	}
	if s.helpOpen {
		switch input.event {
		case loopEventHelpToggle, loopEventEsc:
			s.helpOpen = false
			return true, false
		case loopEventText:
			if input.text == "?" {
				s.helpOpen = false
				return true, false
			}
			return false, false
		default:
			if s.handleHelpScroll(input) {
				return true, false
			}
			return false, false
		}
	}
	if s.historyPickerOpen {
		if s.handleHistoryPicker(input) {
			return true, false
		}
		return false, false
	}

	switch s.focus {
	case loopFocusFilterInput:
		return s.handleFilterInput(input), false
	case loopFocusSearchInput:
		return s.handleSearchInput(input), false
	default:
		return s.handleLogListInput(ctx, input)
	}
}

func (s *loopState) handleFilterInput(input loopInput) bool {
	switch input.event {
	case loopEventEnter:
		return s.applyFilterEditingText()
	case loopEventQuit:
		s.filterEditingText, s.filterCursor = insertTextAtCursor(s.filterEditingText, s.filterCursor, input.text)
		s.filterError = ""
	case loopEventEsc:
		s.filterEditingText = s.filterText
		s.filterCursor = runeCount(s.filterEditingText)
		s.filterError = ""
		s.focus = loopFocusLogList
	case loopEventDown:
		if input.text != "" {
			s.filterEditingText += input.text
			s.filterCursor = runeCount(s.filterEditingText)
			s.filterError = ""
			return true
		}
		s.openFilterHistoryPicker()
	case loopEventHorizontalLeft:
		if input.text != "" {
			s.filterEditingText, s.filterCursor = insertTextAtCursor(s.filterEditingText, s.filterCursor, input.text)
			s.filterError = ""
			return true
		}
		s.filterCursor = moveTextCursorLeft(s.filterEditingText, s.filterCursor)
	case loopEventHorizontalRight:
		if input.text != "" {
			s.filterEditingText, s.filterCursor = insertTextAtCursor(s.filterEditingText, s.filterCursor, input.text)
			s.filterError = ""
			return true
		}
		s.filterCursor = moveTextCursorRight(s.filterEditingText, s.filterCursor)
	case loopEventHelpToggle:
		s.openHelp()
	case loopEventBackspace:
		s.filterEditingText, s.filterCursor = deleteRuneBeforeCursor(s.filterEditingText, s.filterCursor)
		s.filterError = ""
	default:
		if input.text == "" {
			return false
		}
		s.filterEditingText, s.filterCursor = insertTextAtCursor(s.filterEditingText, s.filterCursor, input.text)
		s.filterError = ""
	}
	return true
}

func (s *loopState) handleSearchInput(input loopInput) bool {
	switch input.event {
	case loopEventEnter:
		return s.applySearchEditingText()
	case loopEventQuit:
		s.searchEditingText, s.searchCursor = insertTextAtCursor(s.searchEditingText, s.searchCursor, input.text)
	case loopEventEsc:
		s.searchEditingText = s.searchText
		s.searchCursor = runeCount(s.searchEditingText)
		s.focus = loopFocusLogList
	case loopEventUp:
		if input.text != "" {
			s.searchEditingText += input.text
			s.searchCursor = runeCount(s.searchEditingText)
			return true
		}
		s.filterEditingText = s.filterText
		s.filterCursor = runeCount(s.filterEditingText)
		s.filterError = ""
		s.focus = loopFocusFilterInput
	case loopEventDown:
		if input.text != "" {
			s.searchEditingText += input.text
			s.searchCursor = runeCount(s.searchEditingText)
			return true
		}
		s.openSearchHistoryPicker()
	case loopEventHorizontalLeft:
		if input.text != "" {
			s.searchEditingText, s.searchCursor = insertTextAtCursor(s.searchEditingText, s.searchCursor, input.text)
			return true
		}
		s.searchCursor = moveTextCursorLeft(s.searchEditingText, s.searchCursor)
	case loopEventHorizontalRight:
		if input.text != "" {
			s.searchEditingText, s.searchCursor = insertTextAtCursor(s.searchEditingText, s.searchCursor, input.text)
			return true
		}
		s.searchCursor = moveTextCursorRight(s.searchEditingText, s.searchCursor)
	case loopEventHelpToggle:
		s.openHelp()
	case loopEventBackspace:
		s.searchEditingText, s.searchCursor = deleteRuneBeforeCursor(s.searchEditingText, s.searchCursor)
	default:
		if input.text == "" {
			return false
		}
		s.searchEditingText, s.searchCursor = insertTextAtCursor(s.searchEditingText, s.searchCursor, input.text)
	}
	return true
}

func (s *loopState) handleLogListInput(ctx context.Context, input loopInput) (bool, bool) {
	switch input.event {
	case loopEventQuit:
		return false, true
	case loopEventUp:
		s.moveNavigation(usecase.NavigationMoveUp)
	case loopEventDown:
		s.moveNavigation(usecase.NavigationMoveDown)
	case loopEventShiftUp:
		s.moveNavigationWithSelection(usecase.NavigationMoveUp)
	case loopEventShiftDown:
		s.moveNavigationWithSelection(usecase.NavigationMoveDown)
	case loopEventPageUp:
		s.moveNavigation(usecase.NavigationMovePageUp)
	case loopEventPageDown:
		s.moveNavigation(usecase.NavigationMovePageDown)
	case loopEventHome:
		s.moveNavigation(usecase.NavigationMoveHome)
	case loopEventEnd:
		s.moveNavigation(usecase.NavigationMoveEnd)
	case loopEventLast:
		s.moveNavigation(usecase.NavigationMoveLastAndFollow)
	case loopEventBookmarkToggle:
		s.bookmarks.ToggleRawLine(s.cursorRawLine())
	case loopEventHelpToggle:
		s.openHelp()
	case loopEventText:
		if input.text == "?" {
			s.openHelp()
		}
	case loopEventFilterInput:
		s.clearSelectionMode()
		s.filterEditingText = s.filterText
		s.filterCursor = runeCount(s.filterEditingText)
		s.filterError = ""
		s.focus = loopFocusFilterInput
	case loopEventSearchInput:
		s.searchEditingText = s.searchText
		s.searchCursor = runeCount(s.searchEditingText)
		s.focus = loopFocusSearchInput
	case loopEventSearchNext:
		return s.moveToSearchMatchOrReportBoundary(ctx, usecase.SearchDirectionNext), false
	case loopEventSearchPrevious:
		return s.moveToSearchMatchOrReportBoundary(ctx, usecase.SearchDirectionPrevious), false
	case loopEventWrapToggle:
		s.wrap = !s.wrap
	case loopEventHorizontalLeft:
		return s.scrollHorizontal(-1), false
	case loopEventHorizontalRight:
		return s.scrollHorizontal(1), false
	case loopEventSpacePick:
		if s.cursorRawLine() >= 1 {
			s.selection.PickCursorOrRange(s.cursorRawLine())
			s.syncSelectionMode()
		}
	case loopEventCopy:
		s.copySelection(ctx)
	case loopEventEsc:
		return s.popMode(), false
	default:
		if slot, ok := bookmarkSlotForEvent(input.event); ok {
			return s.moveToBookmark(slot), false
		}
		return false, false
	}
	return true, false
}

func (s *loopState) moveNavigation(move usecase.NavigationMove) {
	s.navigation.Move(move)
	s.selection.ClearRange()
	s.syncSelectionMode()
}

func (s *loopState) moveNavigationWithSelection(move usecase.NavigationMove) {
	if s.selection == nil {
		s.navigation.Move(move)
		return
	}
	anchor := s.cursorRawLine()
	if rawRange, hasRange := s.selection.Range(); hasRange {
		anchor = rawRange.Start
	}
	s.navigation.Move(move)
	cursor := s.cursorRawLine()
	if anchor >= 1 && cursor >= 1 {
		s.selection.StartOrUpdateRange(anchor, cursor)
		s.selection.PickRecordRange(s.records, anchor, cursor)
	}
	s.syncSelectionMode()
}

func (s *loopState) pushMode(mode loopMode) {
	s.removeMode(mode)
	s.modeStack = append(s.modeStack, mode)
}

func (s *loopState) removeMode(mode loopMode) {
	if len(s.modeStack) == 0 {
		return
	}
	next := s.modeStack[:0]
	for _, existing := range s.modeStack {
		if existing != mode {
			next = append(next, existing)
		}
	}
	s.modeStack = next
}

func (s *loopState) openHelp() {
	s.helpOpen = true
	s.helpScrollOffset = 0
}

func (s *loopState) handleHelpScroll(input loopInput) bool {
	step := 1
	page := tui.HelpContentHeight(s.listHeight)
	if page < 1 {
		page = 1
	}
	switch input.event {
	case loopEventDown:
		s.scrollHelp(step)
	case loopEventUp:
		s.scrollHelp(-step)
	case loopEventPageDown:
		s.scrollHelp(page)
	case loopEventPageUp:
		s.scrollHelp(-page)
	case loopEventHome:
		s.helpScrollOffset = 0
	case loopEventEnd:
		s.helpScrollOffset = tui.HelpMaxScrollOffset(s.listHeight)
	default:
		return false
	}
	return true
}

func (s *loopState) scrollHelp(delta int) {
	maxOffset := tui.HelpMaxScrollOffset(s.listHeight)
	next := s.helpScrollOffset + delta
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	s.helpScrollOffset = next
}

func (s *loopState) popMode() bool {
	if len(s.modeStack) == 0 {
		if s.selection.HasSelection() {
			s.clearSelectionMode()
			return true
		}
		if strings.TrimSpace(s.searchText) != "" {
			s.clearSearchMode()
			return true
		}
		return false
	}

	mode := s.modeStack[len(s.modeStack)-1]
	s.modeStack = s.modeStack[:len(s.modeStack)-1]
	switch mode {
	case loopModeSelection:
		s.selection.Clear()
	case loopModeSearch:
		s.clearSearchMode()
	default:
		return false
	}
	return true
}

func (s *loopState) syncSelectionMode() {
	if s.selection.HasSelection() {
		s.pushMode(loopModeSelection)
		return
	}
	s.removeMode(loopModeSelection)
}

func (s *loopState) clearSelectionMode() {
	s.selection.Clear()
	s.removeMode(loopModeSelection)
}

func (s *loopState) clearSearchMode() {
	s.searchText = ""
	s.searchEditingText = ""
	s.removeMode(loopModeSearch)
}

func (s *loopState) scrollHorizontal(delta int) bool {
	if s.wrap || delta == 0 {
		return false
	}
	oldOffset := s.horizontalOffset
	if delta < 0 {
		s.horizontalOffset += delta
		if s.horizontalOffset < 0 {
			s.horizontalOffset = 0
		}
		return s.horizontalOffset != oldOffset
	}
	maxOffset := tui.MaxHorizontalOffset(s.visibleLogLines(), s.width)
	s.horizontalOffset += delta
	if s.horizontalOffset > maxOffset {
		s.horizontalOffset = maxOffset
	}
	return s.horizontalOffset != oldOffset
}

func (s *loopState) copySelection(ctx context.Context) {
	result := usecase.CopySelectedLines(ctx, s.clipboard, s.records, s.selection, s.cursorRawLine())
	s.setTransientRuntimeMessage(result.Message)
}

func (s *loopState) moveToSearchMatch(ctx context.Context, direction usecase.SearchDirection) bool {
	if strings.TrimSpace(s.searchText) == "" {
		return false
	}
	matcher := usecase.NewSearchMatcher(s.searchText)
	if s.fileWindowActive() && s.lineIndex != nil {
		target, moved, err := findRawSearchMatch(ctx, s.sourcePath, *s.lineIndex, matcher, s.navigation.CursorOutputIndex(), direction)
		if err != nil {
			s.setPersistentRuntimeMessage(fmt.Sprintf("search SOT: %v", err))
			return true
		}
		if !moved {
			return false
		}
		return s.moveToOutputIndex(target)
	}
	if s.filteredWindowActive() {
		target, moved, err := findFilteredSearchMatch(ctx, s.sourcePath, s.filter, matcher, s.navigation.CursorOutputIndex(), direction)
		if err != nil {
			s.setPersistentRuntimeMessage(fmt.Sprintf("search SOT: %v", err))
			return true
		}
		if !moved {
			return false
		}
		return s.moveToOutputIndex(target)
	}
	if !s.recordsWindowActive() {
		moved := s.navigation.MoveToSearchMatch(s.records, matcher, direction)
		if moved {
			s.selection.ClearRange()
			s.syncSelectionMode()
		}
		return moved
	}

	current := s.navigation.CursorOutputIndex() - s.windowStartOutputIndex
	target, moved := usecase.NavigateSearchMatch(s.records, matcher, current, direction)
	if !moved {
		return false
	}
	return s.moveToOutputIndex(s.windowStartOutputIndex + target)
}

func (s *loopState) moveToSearchMatchOrReportBoundary(ctx context.Context, direction usecase.SearchDirection) bool {
	moved := s.moveToSearchMatch(ctx, direction)
	if moved {
		return true
	}
	if strings.TrimSpace(s.searchText) == "" {
		return false
	}
	s.setTransientRuntimeMessage("no match")
	return true
}

func (s *loopState) moveToBookmark(slot int) bool {
	if !s.recordsWindowActive() {
		moved := s.navigation.MoveToBookmark(s.records, s.bookmarks, slot)
		if moved {
			s.selection.ClearRange()
			s.syncSelectionMode()
		}
		return moved
	}
	if s.bookmarks == nil {
		return false
	}
	rawLineNumber, ok := s.bookmarks.RawLineForSlot(slot)
	if !ok {
		return false
	}
	for index, record := range s.records {
		if record.RawLineNumber == rawLineNumber {
			return s.moveToOutputIndex(s.windowStartOutputIndex + index)
		}
	}
	return false
}

func (s *loopState) moveToOutputIndex(outputIndex int) bool {
	nav, err := usecase.NewNavigationState(usecase.NavigationOptions{
		OutputCount:       s.outputCount(),
		ViewportHeight:    s.listHeight,
		CursorOutputIndex: outputIndex,
		ScrollOffset:      s.navigation.ScrollOffset(),
		Follow:            false,
	})
	if err != nil {
		return false
	}
	if nav.CursorOutputIndex() == s.navigation.CursorOutputIndex() &&
		nav.ScrollOffset() == s.navigation.ScrollOffset() &&
		nav.Follow() == s.navigation.Follow() {
		return false
	}
	s.navigation = nav
	s.selection.ClearRange()
	s.syncSelectionMode()
	return true
}

func bookmarkSlotForEvent(event loopEvent) (int, bool) {
	switch event {
	case loopEventBookmark1:
		return 1, true
	case loopEventBookmark2:
		return 2, true
	case loopEventBookmark3:
		return 3, true
	case loopEventBookmark4:
		return 4, true
	case loopEventBookmark5:
		return 5, true
	case loopEventBookmark6:
		return 6, true
	case loopEventBookmark7:
		return 7, true
	case loopEventBookmark8:
		return 8, true
	case loopEventBookmark9:
		return 9, true
	default:
		return 0, false
	}
}

func writeFrame(output io.Writer, frame tui.Frame) error {
	if _, err := io.WriteString(output, tui.FrameText(frame)); err != nil {
		return fmt.Errorf("write TUI frame: %w", err)
	}
	return nil
}

func renderLoopRuntimeError(output io.Writer, state *loopState, err error) error {
	state.setPersistentRuntimeMessage(err.Error())
	if writeErr := writeFrame(output, state.renderFrame()); writeErr != nil {
		return fmt.Errorf("%v; render runtime error: %w", err, writeErr)
	}
	return err
}

type loopEventReader struct {
	reader *bufio.Reader
}

func newLoopEventReader(input io.Reader) *loopEventReader {
	if input == nil {
		input = strings.NewReader("")
	}
	return &loopEventReader{reader: bufio.NewReader(input)}
}

func (r *loopEventReader) Next() (loopInput, error) {
	b, err := r.reader.ReadByte()
	if err != nil {
		return eventLoopInput(loopEventUnknown), err
	}
	if b != '<' {
		return byteLoopInput(b), nil
	}
	var builder strings.Builder
	builder.WriteByte(b)
	for {
		next, err := r.reader.ReadByte()
		if err != nil {
			return eventLoopInput(loopEventUnknown), nil
		}
		builder.WriteByte(next)
		if next == '>' {
			return eventLoopInput(namedLoopEvent(builder.String())), nil
		}
	}
}

func eventLoopInput(event loopEvent) loopInput {
	return loopInput{event: event}
}

func namedLoopEvent(token string) loopEvent {
	switch token {
	case "<Esc>":
		return loopEventEsc
	case "<F1>":
		return loopEventHelpToggle
	case "<Up>":
		return loopEventUp
	case "<Down>":
		return loopEventDown
	case "<Enter>":
		return loopEventEnter
	case "<ShiftUp>":
		return loopEventShiftUp
	case "<ShiftDown>":
		return loopEventShiftDown
	case "<PageUp>":
		return loopEventPageUp
	case "<PageDown>":
		return loopEventPageDown
	case "<Home>":
		return loopEventHome
	case "<End>":
		return loopEventEnd
	case "<Left>":
		return loopEventHorizontalLeft
	case "<Right>":
		return loopEventHorizontalRight
	default:
		return loopEventUnknown
	}
}

func byteLoopInput(b byte) loopInput {
	switch b {
	case 0x03:
		return eventLoopInput(loopEventCtrlC)
	case '\n', '\r':
		return eventLoopInput(loopEventEnter)
	case 0x7f, 0x08:
		return eventLoopInput(loopEventBackspace)
	case 'q':
		return loopInput{event: loopEventQuit, text: string(b)}
	case 'j':
		return loopInput{event: loopEventDown, text: string(b)}
	case 'k':
		return loopInput{event: loopEventUp, text: string(b)}
	case 'G':
		return loopInput{event: loopEventLast, text: string(b)}
	case 'h':
		return loopInput{event: loopEventHorizontalLeft, text: string(b)}
	case 'l':
		return loopInput{event: loopEventHorizontalRight, text: string(b)}
	case 'm':
		return loopInput{event: loopEventBookmarkToggle, text: string(b)}
	case 'w':
		return loopInput{event: loopEventWrapToggle, text: string(b)}
	case ' ':
		return loopInput{event: loopEventSpacePick, text: string(b)}
	case 'c':
		return loopInput{event: loopEventCopy, text: string(b)}
	case ':':
		return loopInput{event: loopEventFilterInput, text: string(b)}
	case '/':
		return loopInput{event: loopEventSearchInput, text: string(b)}
	case 'n':
		return loopInput{event: loopEventSearchNext, text: string(b)}
	case 'p':
		return loopInput{event: loopEventSearchPrevious, text: string(b)}
	case '1':
		return loopInput{event: loopEventBookmark1, text: string(b)}
	case '2':
		return loopInput{event: loopEventBookmark2, text: string(b)}
	case '3':
		return loopInput{event: loopEventBookmark3, text: string(b)}
	case '4':
		return loopInput{event: loopEventBookmark4, text: string(b)}
	case '5':
		return loopInput{event: loopEventBookmark5, text: string(b)}
	case '6':
		return loopInput{event: loopEventBookmark6, text: string(b)}
	case '7':
		return loopInput{event: loopEventBookmark7, text: string(b)}
	case '8':
		return loopInput{event: loopEventBookmark8, text: string(b)}
	case '9':
		return loopInput{event: loopEventBookmark9, text: string(b)}
	default:
		if b >= 0x20 {
			return loopInput{event: loopEventText, text: string(b)}
		}
		return eventLoopInput(loopEventUnknown)
	}
}

func dropLastRune(text string) string {
	if text == "" {
		return ""
	}
	last := 0
	for index := range text {
		last = index
	}
	return text[:last]
}
