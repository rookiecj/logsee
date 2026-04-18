package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

func visiblePrefixRunes(s string, maxCells int) []rune {
	if maxCells <= 0 {
		return nil
	}
	var out []rune
	cells := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw <= 0 {
			rw = 1
		}
		if cells+rw > maxCells {
			break
		}
		out = append(out, r)
		cells += rw
	}
	return out
}

func visibleByCells(s string, maxCells int) string {
	return string(visiblePrefixRunes(s, maxCells))
}

// padDisplayWidth pads or truncates s to exactly maxCells terminal display width (runewidth).
func padDisplayWidth(s string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	sw := runewidth.StringWidth(s)
	if sw > maxCells {
		return visibleByCells(s, maxCells)
	}
	if sw < maxCells {
		return s + strings.Repeat(" ", maxCells-sw)
	}
	return s
}

func truncateStatusPath(path string, maxRunes int) string {
	if path == "" {
		return ""
	}
	r := []rune(path)
	if len(r) <= maxRunes {
		return path
	}
	head := 8
	tail := maxRunes - head - 1
	if tail < 4 {
		tail = 4
		head = maxRunes - tail - 1
	}
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
