package ui

import "github.com/mattn/go-runewidth"

// wrapSeg is one visual row when line wrapping: logical filtered index Fi and [R0,R1) rune offsets into that line.
type wrapSeg struct {
	Fi     int
	R0, R1 int
}

// wrapLineRunes splits rs into segments each fitting maxCells terminal display width (go-runewidth).
func wrapLineRunes(rs []rune, maxCells int) [][2]int {
	if maxCells < 1 {
		return nil
	}
	if len(rs) == 0 {
		return [][2]int{{0, 0}}
	}
	var out [][2]int
	start := 0
	for start < len(rs) {
		end := start
		cells := 0
		for end < len(rs) {
			rw := runewidth.RuneWidth(rs[end])
			if rw <= 0 {
				rw = 1
			}
			if cells+rw > maxCells {
				break
			}
			cells += rw
			end++
		}
		if end == start {
			end = start + 1
		}
		out = append(out, [2]int{start, end})
		start = end
	}
	return out
}
