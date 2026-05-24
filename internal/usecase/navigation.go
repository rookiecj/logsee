package usecase

import "fmt"

type NavigationMove int

const (
	NavigationMoveUp NavigationMove = iota
	NavigationMoveDown
	NavigationMovePageUp
	NavigationMovePageDown
	NavigationMoveHome
	NavigationMoveEnd
	NavigationMoveLastAndFollow
)

func (m NavigationMove) String() string {
	switch m {
	case NavigationMoveUp:
		return "up"
	case NavigationMoveDown:
		return "down"
	case NavigationMovePageUp:
		return "page-up"
	case NavigationMovePageDown:
		return "page-down"
	case NavigationMoveHome:
		return "home"
	case NavigationMoveEnd:
		return "end"
	case NavigationMoveLastAndFollow:
		return "last-and-follow"
	default:
		return "unknown"
	}
}

type NavigationOptions struct {
	OutputCount       int
	ViewportHeight    int
	CursorOutputIndex int
	ScrollOffset      int
	Follow            bool
}

type NavigationState struct {
	outputCount       int
	viewportHeight    int
	cursorOutputIndex int
	scrollOffset      int
	follow            bool
}

func NewNavigationState(options NavigationOptions) (*NavigationState, error) {
	if options.OutputCount < 0 {
		return nil, fmt.Errorf("output count must be 0 or greater")
	}
	if options.ViewportHeight < 0 {
		return nil, fmt.Errorf("viewport height must be 0 or greater")
	}
	if options.CursorOutputIndex < 0 {
		return nil, fmt.Errorf("cursor output index must be 0 or greater")
	}
	if options.ScrollOffset < 0 {
		return nil, fmt.Errorf("scroll offset must be 0 or greater")
	}

	state := &NavigationState{
		outputCount:       options.OutputCount,
		viewportHeight:    options.ViewportHeight,
		cursorOutputIndex: options.CursorOutputIndex,
		scrollOffset:      options.ScrollOffset,
		follow:            options.Follow,
	}
	state.normalize()
	return state, nil
}

func (s *NavigationState) Move(move NavigationMove) {
	switch move {
	case NavigationMoveUp:
		s.moveLine(-1)
	case NavigationMoveDown:
		s.moveLine(1)
	case NavigationMovePageUp:
		s.movePage(-1)
	case NavigationMovePageDown:
		s.movePage(1)
	case NavigationMoveHome:
		s.moveHome()
	case NavigationMoveEnd, NavigationMoveLastAndFollow:
		s.moveToLast(true)
	}
}

func (s *NavigationState) MoveToBookmark(records []OutputLogRecord, bookmarks *BookmarkState, slot int) bool {
	if bookmarks == nil {
		return false
	}
	rawLineNumber, ok := bookmarks.RawLineForSlot(slot)
	if !ok {
		return false
	}
	for outputIndex, record := range records {
		if record.RawLineNumber == rawLineNumber {
			s.cursorOutputIndex = outputIndex
			s.follow = false
			s.normalize()
			return true
		}
	}
	return false
}

func (s *NavigationState) SetOutputCount(outputCount int) {
	if outputCount < 0 {
		outputCount = 0
	}
	s.outputCount = outputCount
	s.normalize()
}

func (s *NavigationState) CursorOutputIndex() int {
	return s.cursorOutputIndex
}

func (s *NavigationState) ScrollOffset() int {
	return s.scrollOffset
}

func (s *NavigationState) Follow() bool {
	return s.follow
}

func (s *NavigationState) moveLine(delta int) {
	if s.outputCount == 0 {
		s.normalize()
		return
	}

	oldCursor := s.cursorOutputIndex
	target := clamp(s.cursorOutputIndex+delta, 0, s.lastOutputIndex())
	s.cursorOutputIndex = target

	switch {
	case delta > 0 && target > oldCursor && oldCursor == s.bottomVisibleIndex():
		s.scrollOffset = clamp(s.scrollOffset+1, 0, s.maxScrollOffset())
	case delta < 0 && target < oldCursor && oldCursor == s.scrollOffset:
		s.scrollOffset = clamp(s.scrollOffset-1, 0, s.maxScrollOffset())
	}

	s.afterManualMove()
}

func (s *NavigationState) movePage(direction int) {
	if s.outputCount == 0 {
		s.normalize()
		return
	}

	pageSize := s.viewportHeight
	if pageSize < 1 {
		pageSize = 1
	}

	if direction > 0 {
		bottom := s.bottomVisibleIndex()
		if s.cursorOutputIndex < bottom {
			s.cursorOutputIndex = bottom
			s.afterManualMove()
			return
		}
		s.scrollOffset = clamp(s.scrollOffset+pageSize, 0, s.maxScrollOffset())
		s.cursorOutputIndex = clamp(s.cursorOutputIndex+pageSize, 0, s.lastOutputIndex())
		s.afterManualMove()
		return
	}

	if s.cursorOutputIndex > s.scrollOffset {
		s.cursorOutputIndex = s.scrollOffset
		s.afterManualMove()
		return
	}
	s.scrollOffset = clamp(s.scrollOffset-pageSize, 0, s.maxScrollOffset())
	s.cursorOutputIndex = clamp(s.cursorOutputIndex-pageSize, 0, s.lastOutputIndex())
	s.afterManualMove()
}

func (s *NavigationState) moveHome() {
	s.cursorOutputIndex = 0
	s.scrollOffset = 0
	s.afterManualMove()
}

func (s *NavigationState) moveToLast(enableFollow bool) {
	if s.outputCount == 0 {
		s.normalize()
		return
	}
	s.cursorOutputIndex = s.lastOutputIndex()
	s.scrollOffset = s.maxScrollOffset()
	s.follow = enableFollow
}

func (s *NavigationState) afterManualMove() {
	s.follow = s.outputCount > 0 && s.cursorOutputIndex == s.lastOutputIndex()
	s.normalize()
}

func (s *NavigationState) normalize() {
	if s.outputCount == 0 {
		s.cursorOutputIndex = 0
		s.scrollOffset = 0
		s.follow = false
		return
	}

	s.cursorOutputIndex = clamp(s.cursorOutputIndex, 0, s.lastOutputIndex())
	s.scrollOffset = clamp(s.scrollOffset, 0, s.maxScrollOffset())

	if s.follow {
		s.cursorOutputIndex = s.lastOutputIndex()
		s.scrollOffset = s.maxScrollOffset()
		return
	}

	s.keepCursorVisible()
}

func (s *NavigationState) keepCursorVisible() {
	if s.viewportHeight <= 0 {
		s.scrollOffset = 0
		return
	}
	if s.cursorOutputIndex < s.scrollOffset {
		s.scrollOffset = s.cursorOutputIndex
	}
	if s.cursorOutputIndex > s.bottomVisibleIndex() {
		s.scrollOffset = s.cursorOutputIndex - s.viewportHeight + 1
	}
	s.scrollOffset = clamp(s.scrollOffset, 0, s.maxScrollOffset())
}

func (s *NavigationState) bottomVisibleIndex() int {
	if s.outputCount == 0 {
		return 0
	}
	if s.viewportHeight <= 0 {
		return s.scrollOffset
	}
	return min(s.scrollOffset+s.viewportHeight-1, s.lastOutputIndex())
}

func (s *NavigationState) lastOutputIndex() int {
	return s.outputCount - 1
}

func (s *NavigationState) maxScrollOffset() int {
	if s.viewportHeight <= 0 || s.outputCount <= s.viewportHeight {
		return 0
	}
	return s.outputCount - s.viewportHeight
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
