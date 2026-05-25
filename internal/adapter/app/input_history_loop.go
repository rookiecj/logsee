package app

import (
	"logsee/internal/adapter/config"
	"logsee/internal/adapter/tui"
	"logsee/internal/usecase"
)

func (s *loopState) loadPersistedInputHistory() error {
	if s.homeDir == "" {
		return nil
	}
	s.inputHistoryPath = config.ResolveInputHistoryPath(s.homeDir)
	history, err := config.LoadInputHistory(s.inputHistoryPath)
	if err != nil {
		return err
	}
	s.inputHistory = history
	return s.restorePersistedInputs()
}

func (s *loopState) restorePersistedInputs() error {
	if s.inputHistory.Filter.Last != "" {
		s.filterEditingText = s.inputHistory.Filter.Last
		s.applyFilterEditingText()
	}
	if s.inputHistory.Search.Last != "" {
		s.searchEditingText = s.inputHistory.Search.Last
		s.applySearchEditingText()
	}
	return nil
}

func (s *loopState) persistInputHistory() {
	if s.inputHistoryPath == "" {
		return
	}
	_ = config.SaveInputHistory(s.inputHistoryPath, s.inputHistory)
}

func (s *loopState) recordFilterHistory() {
	s.inputHistory.Filter = usecase.RecordChannelHistory(s.inputHistory.Filter, s.filterText)
	s.persistInputHistory()
}

func (s *loopState) recordSearchHistory() {
	s.inputHistory.Search = usecase.RecordChannelHistory(s.inputHistory.Search, s.searchText)
	s.persistInputHistory()
}

func (s *loopState) openFilterHistoryPicker() {
	s.historyPickerOpen = true
	s.historyPickerFilter = true
	s.historyPickerIndex = 0
	s.historyPickerScroll = 0
	s.syncHistoryPickerScrollToIndex()
}

func (s *loopState) openSearchHistoryPicker() {
	s.historyPickerOpen = true
	s.historyPickerFilter = false
	s.historyPickerIndex = 0
	s.historyPickerScroll = 0
	s.syncHistoryPickerScrollToIndex()
}

func (s *loopState) closeHistoryPicker() {
	s.historyPickerOpen = false
}

func (s *loopState) historyPickerItems() []string {
	if s.historyPickerFilter {
		return append([]string(nil), s.inputHistory.Filter.History...)
	}
	return append([]string(nil), s.inputHistory.Search.History...)
}

func (s *loopState) historyPickerTitle() string {
	if s.historyPickerFilter {
		return "Filter history"
	}
	return "Search history"
}

func (s *loopState) applyHistoryPickerSelection() {
	items := s.historyPickerItems()
	if len(items) == 0 || s.historyPickerIndex < 0 || s.historyPickerIndex >= len(items) {
		s.closeHistoryPicker()
		return
	}
	value := items[s.historyPickerIndex]
	if s.historyPickerFilter {
		s.filterEditingText = value
		s.filterCursor = runeCount(value)
		s.filterError = ""
	} else {
		s.searchEditingText = value
		s.searchCursor = runeCount(value)
	}
	s.closeHistoryPicker()
}

func (s *loopState) handleHistoryPicker(input loopInput) bool {
	switch input.event {
	case loopEventEsc, loopEventHelpToggle:
		s.closeHistoryPicker()
		return true
	case loopEventEnter:
		s.applyHistoryPickerSelection()
		return true
	case loopEventDown:
		s.moveHistoryPickerIndex(1)
		return true
	case loopEventUp:
		s.moveHistoryPickerIndex(-1)
		return true
	case loopEventPageDown:
		page := tui.HistoryPickerContentHeight(s.listHeight)
		if page < 1 {
			page = 1
		}
		s.moveHistoryPickerIndex(page)
		return true
	case loopEventPageUp:
		page := tui.HistoryPickerContentHeight(s.listHeight)
		if page < 1 {
			page = 1
		}
		s.moveHistoryPickerIndex(-page)
		return true
	case loopEventHome:
		s.historyPickerIndex = 0
		s.syncHistoryPickerScrollToIndex()
		return true
	case loopEventEnd:
		items := s.historyPickerItems()
		if len(items) > 0 {
			s.historyPickerIndex = len(items) - 1
			s.syncHistoryPickerScrollToIndex()
		}
		return true
	case loopEventText:
		switch input.text {
		case "j":
			s.moveHistoryPickerIndex(1)
			return true
		case "k":
			s.moveHistoryPickerIndex(-1)
			return true
		}
	}
	return false
}

func (s *loopState) moveHistoryPickerIndex(delta int) {
	items := s.historyPickerItems()
	if len(items) == 0 {
		return
	}
	next := s.historyPickerIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= len(items) {
		next = len(items) - 1
	}
	s.historyPickerIndex = next
	s.syncHistoryPickerScrollToIndex()
}

func (s *loopState) syncHistoryPickerScrollToIndex() {
	items := s.historyPickerItems()
	contentHeight := tui.HistoryPickerContentHeight(s.listHeight)
	if contentHeight < 1 {
		contentHeight = 1
	}
	if s.historyPickerIndex < s.historyPickerScroll {
		s.historyPickerScroll = s.historyPickerIndex
	}
	if s.historyPickerIndex >= s.historyPickerScroll+contentHeight {
		s.historyPickerScroll = s.historyPickerIndex - contentHeight + 1
	}
	maxOffset := tui.HistoryPickerMaxScrollOffset(len(items), s.listHeight)
	if s.historyPickerScroll > maxOffset {
		s.historyPickerScroll = maxOffset
	}
	if s.historyPickerScroll < 0 {
		s.historyPickerScroll = 0
	}
}
