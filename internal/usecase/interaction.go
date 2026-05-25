package usecase

import (
	"context"

	"logsee/internal/port"
)

type InteractionFocus int

const (
	InteractionFocusLogList InteractionFocus = iota
	InteractionFocusFilterInput
	InteractionFocusSearchInput
)

type InteractionKey int

const (
	InteractionKeyHelpF1 InteractionKey = iota
	InteractionKeyHelpQuestion
	InteractionKeyEsc
)

func (k InteractionKey) String() string {
	switch k {
	case InteractionKeyHelpF1:
		return "f1"
	case InteractionKeyHelpQuestion:
		return "question"
	case InteractionKeyEsc:
		return "esc"
	default:
		return "unknown"
	}
}

type InteractionSnapshot struct {
	Focus             InteractionFocus
	FilterEditingText string
	SearchEditingText string
	CursorRawLine     int
	SelectionRange    RawLineRange
	HasRange          bool
	PickedLines       []int
	StatusMessage     string
}

type InteractionState struct {
	focus             InteractionFocus
	filterEditingText string
	searchEditingText string
	cursorRawLine     int
	selection         *SelectionState
	statusMessage     string
	helpOpen          bool
}

func NewInteractionState() *InteractionState {
	return &InteractionState{
		focus:     InteractionFocusLogList,
		selection: NewSelectionState(),
	}
}

func (f InteractionFocus) String() string {
	switch f {
	case InteractionFocusLogList:
		return "log-list"
	case InteractionFocusFilterInput:
		return "filter-input"
	case InteractionFocusSearchInput:
		return "search-input"
	default:
		return "unknown"
	}
}

func (s *InteractionState) SetFocus(focus InteractionFocus) {
	s.focus = focus
}

func (s *InteractionState) HandleRune(r rune) {
	switch s.focus {
	case InteractionFocusFilterInput:
		s.filterEditingText += string(r)
	case InteractionFocusSearchInput:
		s.searchEditingText += string(r)
	case InteractionFocusLogList:
		if r == ' ' && s.cursorRawLine >= 1 {
			s.selection.PickCursorOrRange(s.cursorRawLine)
		}
	}
}

func (s *InteractionState) HandleKey(key InteractionKey) {
	if s.helpOpen {
		switch key {
		case InteractionKeyHelpF1, InteractionKeyHelpQuestion, InteractionKeyEsc:
			s.helpOpen = false
		}
		return
	}

	switch key {
	case InteractionKeyHelpF1:
		s.helpOpen = true
	case InteractionKeyHelpQuestion:
		if s.focus == InteractionFocusLogList {
			s.helpOpen = true
		}
	case InteractionKeyEsc:
		if s.focus == InteractionFocusLogList {
			s.selection.Clear()
		}
	}
}

func (s *InteractionState) CopyFromLogList(ctx context.Context, clipboard port.ClipboardWriter, records []OutputLogRecord) CopyTextResult {
	if s.focus != InteractionFocusLogList {
		return CopyTextResult{}
	}
	result := CopySelectedLines(ctx, clipboard, records, s.selection, s.cursorRawLine)
	s.statusMessage = result.Message
	return result
}

func (s *InteractionState) HelpOpen() bool {
	return s.helpOpen
}

func (s *InteractionState) Selection() *SelectionState {
	return s.selection
}

func (s *InteractionState) SetFilterEditingText(text string) {
	s.filterEditingText = text
}

func (s *InteractionState) FilterEditingText() string {
	return s.filterEditingText
}

func (s *InteractionState) SetSearchEditingText(text string) {
	s.searchEditingText = text
}

func (s *InteractionState) SearchEditingText() string {
	return s.searchEditingText
}

func (s *InteractionState) SetCursorRawLine(rawLineNumber int) {
	s.cursorRawLine = rawLineNumber
}

func (s *InteractionState) StatusMessage() string {
	return s.statusMessage
}

func (s *InteractionState) Snapshot() InteractionSnapshot {
	rawRange, hasRange := s.selection.Range()
	return InteractionSnapshot{
		Focus:             s.focus,
		FilterEditingText: s.filterEditingText,
		SearchEditingText: s.searchEditingText,
		CursorRawLine:     s.cursorRawLine,
		SelectionRange:    rawRange,
		HasRange:          hasRange,
		PickedLines:       s.selection.PickedLines(),
		StatusMessage:     s.statusMessage,
	}
}
