package tui

import (
	"fmt"
	"strings"
)

func HistoryPickerContentHeight(viewportHeight int) int {
	if viewportHeight <= 2 {
		return 1
	}
	return viewportHeight - 2
}

func HistoryPickerMaxScrollOffset(itemCount, viewportHeight int) int {
	contentHeight := HistoryPickerContentHeight(viewportHeight)
	if contentHeight <= 0 || itemCount <= contentHeight {
		return 0
	}
	return itemCount - contentHeight
}

func HistoryPickerViewportLines(title string, items []string, selectedIndex, scrollOffset, viewportHeight, width int) []string {
	if viewportHeight <= 0 {
		return nil
	}
	contentHeight := HistoryPickerContentHeight(viewportHeight)
	maxOffset := HistoryPickerMaxScrollOffset(len(items), viewportHeight)
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if len(items) > 0 && selectedIndex >= len(items) {
		selectedIndex = len(items) - 1
	}

	visible := make([]string, 0, viewportHeight)
	visible = append(visible, historyOverlayTitleLine(title, width))
	if len(items) == 0 {
		visible = append(visible, truncateRunes(" (no history) ", width))
		for len(visible) < viewportHeight-1 {
			visible = append(visible, strings.Repeat(" ", width))
		}
	} else {
		for index := 0; index < contentHeight; index++ {
			sourceIndex := scrollOffset + index
			line := ""
			if sourceIndex < len(items) {
				prefix := "  "
				if sourceIndex == selectedIndex {
					prefix = "> "
				}
				line = prefix + items[sourceIndex]
			}
			visible = append(visible, truncateRunes(line, width))
		}
		for len(visible) < viewportHeight-1 {
			visible = append(visible, strings.Repeat(" ", width))
		}
	}
	visible = append(visible, historyOverlayScrollStatusLine(selectedIndex, len(items), scrollOffset, maxOffset, width))
	return visible
}

func historyOverlayTitleLine(title string, width int) string {
	label := " " + title + " "
	if width < len(label) {
		return truncateRunes(label, width)
	}
	fill := width - len(label)
	if fill < 0 {
		fill = 0
	}
	return "─" + label + strings.Repeat("─", fill)
}

func historyOverlayScrollStatusLine(selectedIndex, itemCount, scrollOffset, maxOffset, width int) string {
	status := fmt.Sprintf(" %d/%d ", selectedIndex+1, itemCount)
	if maxOffset > 0 {
		status = fmt.Sprintf(" %d/%d scroll %d/%d ", selectedIndex+1, itemCount, scrollOffset+1, maxOffset+1)
	}
	if width < len(status) {
		return truncateRunes(status, width)
	}
	fill := width - len(status)
	if fill < 0 {
		fill = 0
	}
	return strings.Repeat("─", fill) + status
}
