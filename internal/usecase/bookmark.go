package usecase

import "sort"

const MaxBookmarkSlots = 9

type BookmarkKey rune

const BookmarkKeyToggle BookmarkKey = 'm'

type BookmarkState struct {
	slotToRawLine map[int]int
	rawLineToSlot map[int]int
}

func NewBookmarkState() *BookmarkState {
	return &BookmarkState{
		slotToRawLine: make(map[int]int),
		rawLineToSlot: make(map[int]int),
	}
}

func (s *BookmarkState) HandleLogListKey(key BookmarkKey, currentRawLineNumber int) (int, bool) {
	if key != BookmarkKeyToggle {
		return 0, false
	}
	return s.ToggleRawLine(currentRawLineNumber)
}

func (s *BookmarkState) ToggleRawLine(rawLineNumber int) (int, bool) {
	if rawLineNumber < 1 {
		return 0, false
	}
	if slot, ok := s.rawLineToSlot[rawLineNumber]; ok {
		delete(s.rawLineToSlot, rawLineNumber)
		delete(s.slotToRawLine, slot)
		return slot, false
	}

	slot, ok := s.lowestAvailableSlot()
	if !ok {
		return 0, false
	}
	s.rawLineToSlot[rawLineNumber] = slot
	s.slotToRawLine[slot] = rawLineNumber
	return slot, true
}

func (s *BookmarkState) SlotForRawLine(rawLineNumber int) (int, bool) {
	slot, ok := s.rawLineToSlot[rawLineNumber]
	return slot, ok
}

func (s *BookmarkState) RawLineForSlot(slot int) (int, bool) {
	rawLineNumber, ok := s.slotToRawLine[slot]
	return rawLineNumber, ok
}

func (s *BookmarkState) Slots() []int {
	slots := make([]int, 0, len(s.slotToRawLine))
	for slot := range s.slotToRawLine {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	return slots
}

func (s *BookmarkState) lowestAvailableSlot() (int, bool) {
	for slot := 1; slot <= MaxBookmarkSlots; slot++ {
		if _, exists := s.slotToRawLine[slot]; !exists {
			return slot, true
		}
	}
	return 0, false
}
