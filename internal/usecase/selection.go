package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"logsee/internal/port"
)

type RawLineRange struct {
	Start int
	End   int
}

type SelectionState struct {
	rangeAnchor int
	rangeCursor int
	picked      map[int]struct{}
}

type CopyTextResult struct {
	Text      string
	LineCount int
	Message   string
	Err       error
}

func NewSelectionState() *SelectionState {
	return &SelectionState{picked: map[int]struct{}{}}
}

func (s *SelectionState) StartOrUpdateRange(anchorRawLineNumber, cursorRawLineNumber int) {
	if anchorRawLineNumber < 1 || cursorRawLineNumber < 1 {
		return
	}
	s.rangeAnchor = anchorRawLineNumber
	s.rangeCursor = cursorRawLineNumber
}

func (s *SelectionState) Clear() {
	s.rangeAnchor = 0
	s.rangeCursor = 0
	s.picked = map[int]struct{}{}
}

func (s *SelectionState) ClearRange() {
	if s == nil {
		return
	}
	s.rangeAnchor = 0
	s.rangeCursor = 0
}

func (s *SelectionState) Range() (RawLineRange, bool) {
	if s == nil || s.rangeAnchor < 1 || s.rangeCursor < 1 {
		return RawLineRange{}, false
	}
	return RawLineRange{Start: s.rangeAnchor, End: s.rangeCursor}, true
}

func (s *SelectionState) TogglePicked(rawLineNumber int) bool {
	if rawLineNumber < 1 {
		return false
	}
	if s.picked == nil {
		s.picked = map[int]struct{}{}
	}
	if _, ok := s.picked[rawLineNumber]; ok {
		delete(s.picked, rawLineNumber)
		return false
	}
	s.picked[rawLineNumber] = struct{}{}
	return true
}

func (s *SelectionState) PickedCount() int {
	if s == nil {
		return 0
	}
	return len(s.picked)
}

func (s *SelectionState) PickedLines() []int {
	if s == nil || len(s.picked) == 0 {
		return nil
	}
	lines := make([]int, 0, len(s.picked))
	for rawLineNumber := range s.picked {
		lines = append(lines, rawLineNumber)
	}
	sort.Ints(lines)
	return lines
}

func (s *SelectionState) IsRangeSelected(rawLineNumber int) bool {
	rawRange, ok := s.Range()
	if !ok {
		return false
	}
	start, end := normalizeRawLineRange(rawRange)
	return rawLineNumber >= start && rawLineNumber <= end
}

func (s *SelectionState) IsPicked(rawLineNumber int) bool {
	if s == nil {
		return false
	}
	_, ok := s.picked[rawLineNumber]
	return ok
}

func (s *SelectionState) HasSelection() bool {
	if s == nil {
		return false
	}
	_, hasRange := s.Range()
	return hasRange || len(s.picked) > 0
}

func (s *SelectionState) PickRangeAndClear() {
	rawRange, ok := s.Range()
	if !ok {
		return
	}
	s.PickRawLineRange(rawRange.Start, rawRange.End)
	s.ClearRange()
}

func (s *SelectionState) PickRawLineRange(startRawLineNumber, endRawLineNumber int) {
	if s == nil || startRawLineNumber < 1 || endRawLineNumber < 1 {
		return
	}
	if s.picked == nil {
		s.picked = map[int]struct{}{}
	}
	start, end := normalizeRawLineRange(RawLineRange{Start: startRawLineNumber, End: endRawLineNumber})
	for rawLineNumber := start; rawLineNumber <= end; rawLineNumber++ {
		s.picked[rawLineNumber] = struct{}{}
	}
}

func (s *SelectionState) PickRecordRange(records []OutputLogRecord, startRawLineNumber, endRawLineNumber int) {
	if s == nil || startRawLineNumber < 1 || endRawLineNumber < 1 {
		return
	}
	if s.picked == nil {
		s.picked = map[int]struct{}{}
	}
	start, end := normalizeRawLineRange(RawLineRange{Start: startRawLineNumber, End: endRawLineNumber})
	for _, record := range records {
		if record.RawLineNumber >= start && record.RawLineNumber <= end {
			s.picked[record.RawLineNumber] = struct{}{}
		}
	}
}

func (s *SelectionState) PickCursorOrRange(cursorRawLineNumber int) bool {
	if rawRange, ok := s.Range(); ok {
		s.PickRawLineRange(rawRange.Start, rawRange.End)
		s.ClearRange()
	}
	return s.TogglePicked(cursorRawLineNumber)
}

func (s *NavigationState) MoveWithSelection(move NavigationMove, records []OutputLogRecord, selection *SelectionState, extend bool) {
	if !extend || selection == nil {
		s.Move(move)
		if selection != nil {
			selection.ClearRange()
		}
		return
	}

	anchor, ok := rawLineAtOutputIndex(records, s.cursorOutputIndex)
	if !ok {
		s.Move(move)
		return
	}
	if rawRange, hasRange := selection.Range(); hasRange {
		anchor = rawRange.Start
	}

	s.Move(move)
	cursor, ok := rawLineAtOutputIndex(records, s.cursorOutputIndex)
	if !ok {
		return
	}
	selection.StartOrUpdateRange(anchor, cursor)
	selection.PickRecordRange(records, anchor, cursor)
}

func BuildCopyText(records []OutputLogRecord, selection *SelectionState, cursorRawLineNumber int) CopyTextResult {
	selected := map[int]struct{}{}
	if selection != nil {
		if rawRange, ok := selection.Range(); ok {
			start, end := normalizeRawLineRange(rawRange)
			for rawLineNumber := start; rawLineNumber <= end; rawLineNumber++ {
				selected[rawLineNumber] = struct{}{}
			}
		}
		for _, rawLineNumber := range selection.PickedLines() {
			selected[rawLineNumber] = struct{}{}
		}
	}
	if len(selected) == 0 && cursorRawLineNumber >= 1 {
		selected[cursorRawLineNumber] = struct{}{}
	}

	linesByRawLine := make(map[int]string, len(records))
	for _, record := range records {
		if _, wanted := selected[record.RawLineNumber]; wanted {
			linesByRawLine[record.RawLineNumber] = record.Text
		}
	}

	rawLines := make([]int, 0, len(linesByRawLine))
	for rawLineNumber := range linesByRawLine {
		rawLines = append(rawLines, rawLineNumber)
	}
	sort.Ints(rawLines)

	lines := make([]string, len(rawLines))
	for i, rawLineNumber := range rawLines {
		lines[i] = linesByRawLine[rawLineNumber]
	}

	count := len(lines)
	return CopyTextResult{
		Text:      strings.Join(lines, "\n"),
		LineCount: count,
		Message:   copyResultMessage(count),
	}
}

func CopySelectedLines(ctx context.Context, clipboard port.ClipboardWriter, records []OutputLogRecord, selection *SelectionState, cursorRawLineNumber int) CopyTextResult {
	result := BuildCopyText(records, selection, cursorRawLineNumber)
	if result.LineCount == 0 {
		return result
	}
	if clipboard == nil {
		result.Err = fmt.Errorf("clipboard writer is required")
		result.Message = "clipboard error"
		return result
	}
	if err := clipboard.WriteText(ctx, result.Text); err != nil {
		result.Err = fmt.Errorf("write clipboard: %w", err)
		result.Message = "clipboard error"
		return result
	}
	return result
}

func rawLineAtOutputIndex(records []OutputLogRecord, outputIndex int) (int, bool) {
	if outputIndex < 0 || outputIndex >= len(records) {
		return 0, false
	}
	rawLineNumber := records[outputIndex].RawLineNumber
	return rawLineNumber, rawLineNumber >= 1
}

func normalizeRawLineRange(rawRange RawLineRange) (int, int) {
	start, end := rawRange.Start, rawRange.End
	if start > end {
		start, end = end, start
	}
	return start, end
}

func copyResultMessage(count int) string {
	switch count {
	case 0:
		return "no lines"
	case 1:
		return "1 line copied"
	default:
		return fmt.Sprintf("%d lines copied", count)
	}
}
