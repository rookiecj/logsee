package tui

import "strings"

func helpContentLines() []string {
	return []string{
		"Movement: j/k, Up/Down, PageUp/PageDown, Ctrl+F/Ctrl+B, Home/End",
		"Follow: G last line + follow on; k/Up/PgUp/p move up stops follow",
		"Search match: n/p or Ctrl+N/Ctrl+P next/previous match line",
		"Filter: : open; Enter apply; Esc cancel edit; Down history",
		"Search: / open; Enter apply; Esc cancel edit; Down history; Up to filter",
		"Bookmarks: m toggle line; 1-9 jump to visible bookmark slot",
		"Wrap: w toggle; h/l scroll log body left/right when wrap is off",
		"Selection: Shift+Up/Down extend range; Space pick line; c copy",
		"Modes: Esc pops search/selection stack; help open Esc closes help only",
		"Help: F1 from any focus; ? from log list; Esc/F1/? close help",
		"Quit: q on log list; Ctrl+C anywhere",
	}
}

func HelpContentHeight(viewportHeight int) int {
	if viewportHeight <= 2 {
		return 1
	}
	return viewportHeight - 2
}

func HelpMaxScrollOffset(viewportHeight int) int {
	contentHeight := HelpContentHeight(viewportHeight)
	lines := helpContentLines()
	if contentHeight <= 0 || len(lines) <= contentHeight {
		return 0
	}
	return len(lines) - contentHeight
}

func HelpViewportLines(scrollOffset, viewportHeight, width int) []string {
	if viewportHeight <= 0 {
		return nil
	}
	contentHeight := HelpContentHeight(viewportHeight)
	maxOffset := HelpMaxScrollOffset(viewportHeight)
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}

	lines := helpContentLines()
	visible := make([]string, 0, viewportHeight)
	visible = append(visible, helpOverlayTitleLine(width))
	for index := 0; index < contentHeight; index++ {
		line := ""
		sourceIndex := scrollOffset + index
		if sourceIndex < len(lines) {
			line = lines[sourceIndex]
		}
		visible = append(visible, truncateRunes(line, width))
	}
	for len(visible) < viewportHeight-1 {
		visible = append(visible, strings.Repeat(" ", width))
	}
	visible = append(visible, helpOverlayScrollStatusLine(scrollOffset, maxOffset, width))
	return visible
}

func helpOverlayTitleLine(width int) string {
	title := " Help "
	if width < len(title) {
		return truncateRunes(title, width)
	}
	fill := width - len(title)
	if fill < 0 {
		fill = 0
	}
	return "─" + title + strings.Repeat("─", fill)
}

func helpOverlayScrollStatusLine(scrollOffset, maxOffset, width int) string {
	status := "j/k scroll"
	if maxOffset > 0 {
		status += " " + scrollPositionLabel(scrollOffset, maxOffset)
	}
	status += " · Esc/F1/? close"
	return truncateRunes(status, width)
}

func scrollPositionLabel(scrollOffset, maxOffset int) string {
	if maxOffset <= 0 {
		return ""
	}
	return itoa(scrollOffset+1) + "/" + itoa(maxOffset+1)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
