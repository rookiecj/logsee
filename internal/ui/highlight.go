package ui

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"git.inpt.fr/42dottools/log/internal/config"
	"git.inpt.fr/42dottools/log/internal/filter"
	"github.com/charmbracelet/lipgloss"
)

const (
	// DefaultHighlightBG is the ANSI256 index used when a token has no #color suffix.
	DefaultHighlightBG = "214"
	// DefaultHighlightFG is the foreground paired with highlight backgrounds.
	DefaultHighlightFG = "0"
)

// HighlightNeedle is one search needle with optional ANSI256 background (empty BG → use default highlight style).
type HighlightNeedle struct {
	Text string
	BG   string // empty → use defaultStyle in HighlightFromNeedles; else "0".."255"
}

// SearchQueryTokens splits a committed highlight query into needles (PRD §8.2):
// ASCII space/tab-separated tokens; double-quoted spans keep inner spaces (same rules as filter.Tokenize).
// On tokenization error (e.g. unclosed quote), the whole trimmed string is one token.
func SearchQueryTokens(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	toks, err := filter.Tokenize(query)
	if err != nil {
		return []string{query}
	}
	return toks
}

// ParseHighlightNeedles splits the query into needles and optional per-token ANSI256 BG.
// names is the effective name→index map (e.g. config.MergeHighlightColorNames); if nil, built-in names only.
func ParseHighlightNeedles(query string, names map[string]string) []HighlightNeedle {
	effective := names
	if effective == nil {
		effective = config.MergeHighlightColorNames(nil)
	}
	toks := SearchQueryTokens(query)
	if len(toks) == 0 {
		return nil
	}
	var out []HighlightNeedle
	for _, t := range toks {
		if t == "" {
			continue
		}
		nd, ok := parseOneHighlightToken(t, effective)
		if !ok || nd.Text == "" {
			continue
		}
		out = append(out, nd)
	}
	return out
}

func parseOneHighlightToken(tok string, names map[string]string) (HighlightNeedle, bool) {
	idx := strings.LastIndex(tok, "#")
	if idx < 0 {
		return HighlightNeedle{Text: tok, BG: ""}, true
	}
	needle := tok[:idx]
	colorSpec := tok[idx+1:]
	if needle == "" || colorSpec == "" {
		return HighlightNeedle{}, false
	}
	bg, ok := parseColorSpec(colorSpec, names)
	if !ok {
		return HighlightNeedle{}, false
	}
	return HighlightNeedle{Text: needle, BG: bg}, true
}

func parseColorSpec(spec string, names map[string]string) (bg string, ok bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	if isAllASCIIDigits(spec) {
		n, err := strconv.Atoi(spec)
		if err != nil || n < 0 || n > 255 {
			return "", false
		}
		return strconv.Itoa(n), true
	}
	if !isColorNameIdent(spec) {
		return "", false
	}
	v, ok := names[strings.ToLower(spec)]
	return v, ok
}

func isAllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isColorNameIdent(s string) bool {
	rs := []rune(s)
	if len(rs) == 0 {
		return false
	}
	if !unicode.IsLetter(rs[0]) && rs[0] != '_' {
		return false
	}
	for _, r := range rs[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func highlightLayers(line string, needles []HighlightNeedle, ignoreCase bool) []int {
	layers := make([]int, len(line))
	for i := range layers {
		layers[i] = -1
	}
	for ni, nd := range needles {
		if nd.Text == "" {
			continue
		}
		rest := line
		offset := 0
		for rest != "" {
			start, end, ok := findNextMatch(rest, nd.Text, ignoreCase)
			if !ok {
				break
			}
			absStart := offset + start
			absEnd := offset + end
			for b := absStart; b < absEnd && b < len(layers); b++ {
				layers[b] = ni
			}
			rest = rest[end:]
			offset += end
		}
	}
	return layers
}

type paintSeg struct {
	start, end int
	ni         int // needle index, or -1 if plain (no highlight)
}

func paintSegmentsFromLayers(layers []int) []paintSeg {
	if len(layers) == 0 {
		return nil
	}
	var out []paintSeg
	i := 0
	for i < len(layers) {
		ni := layers[i]
		j := i + 1
		for j < len(layers) && layers[j] == ni {
			j++
		}
		out = append(out, paintSeg{i, j, ni})
		i = j
	}
	return out
}

func needleMatchStyle(nd HighlightNeedle, defaultStyle lipgloss.Style) lipgloss.Style {
	if nd.BG == "" {
		return defaultStyle
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(nd.BG)).
		Foreground(lipgloss.Color(DefaultHighlightFG))
}

// HighlightFromNeedles paints line using per-needle styles; defaultStyle applies when needle BG is empty.
func HighlightFromNeedles(line string, needles []HighlightNeedle, ignoreCase bool, defaultStyle lipgloss.Style) string {
	if len(needles) == 0 {
		return line
	}
	layers := highlightLayers(line, needles, ignoreCase)
	has := false
	for _, ni := range layers {
		if ni >= 0 {
			has = true
			break
		}
	}
	if !has {
		return line
	}
	segs := paintSegmentsFromLayers(layers)
	var b strings.Builder
	for _, sg := range segs {
		chunk := line[sg.start:sg.end]
		if sg.ni < 0 {
			b.WriteString(chunk)
			continue
		}
		st := needleMatchStyle(needles[sg.ni], defaultStyle)
		b.WriteString(st.Render(chunk))
	}
	return b.String()
}

// Highlight marks every occurrence of each query token on line using style for default-colored needles.
func Highlight(line, query string, ignoreCase bool, style lipgloss.Style) string {
	needles := ParseHighlightNeedles(query, nil)
	if len(needles) == 0 {
		return line
	}
	return HighlightFromNeedles(line, needles, ignoreCase, style)
}

// HighlightWithReverseStyles renders a cursor row with reverse plain segments and per-match highlight styles.
// defaultMatch is used for needles without an explicit #color (same as non-cursor default highlight).
func HighlightWithReverseStyles(line, query string, ignoreCase bool, plainSeg, defaultMatch lipgloss.Style, names map[string]string) string {
	needles := ParseHighlightNeedles(query, names)
	return HighlightReverseFromNeedles(line, needles, ignoreCase, plainSeg, defaultMatch)
}

// HighlightReverseFromNeedles is like HighlightWithReverseStyles but uses pre-parsed needles (avoids double parse).
func HighlightReverseFromNeedles(line string, needles []HighlightNeedle, ignoreCase bool, plainSeg, defaultMatch lipgloss.Style) string {
	if len(needles) == 0 {
		return plainSeg.Render(line)
	}
	layers := highlightLayers(line, needles, ignoreCase)
	has := false
	for _, ni := range layers {
		if ni >= 0 {
			has = true
			break
		}
	}
	if !has {
		return plainSeg.Render(line)
	}
	segs := paintSegmentsFromLayers(layers)
	var b strings.Builder
	for _, sg := range segs {
		chunk := line[sg.start:sg.end]
		if sg.ni < 0 {
			b.WriteString(plainSeg.Render(chunk))
			continue
		}
		st := matchSegForNeedle(needles[sg.ni], plainSeg, defaultMatch)
		b.WriteString(st.Render(chunk))
	}
	return b.String()
}

func matchSegForNeedle(nd HighlightNeedle, plainSeg, defaultMatch lipgloss.Style) lipgloss.Style {
	if nd.BG == "" {
		return defaultMatch
	}
	return plainSeg.Background(lipgloss.Color(nd.BG)).Foreground(lipgloss.Color(DefaultHighlightFG))
}

// HighlightWithReverseStylesSelected is like HighlightWithReverseStyles but uses fixed selection plain/match bases (cursor+range selection).
func HighlightWithReverseStylesSelected(line string, needles []HighlightNeedle, ignoreCase bool, plainSeg, defaultMatchSel lipgloss.Style) string {
	if len(needles) == 0 {
		return plainSeg.Render(line)
	}
	layers := highlightLayers(line, needles, ignoreCase)
	has := false
	for _, ni := range layers {
		if ni >= 0 {
			has = true
			break
		}
	}
	if !has {
		return plainSeg.Render(line)
	}
	segs := paintSegmentsFromLayers(layers)
	var b strings.Builder
	for _, sg := range segs {
		chunk := line[sg.start:sg.end]
		if sg.ni < 0 {
			b.WriteString(plainSeg.Render(chunk))
			continue
		}
		nd := needles[sg.ni]
		var st lipgloss.Style
		if nd.BG == "" {
			st = defaultMatchSel
		} else {
			st = lipgloss.NewStyle().Reverse(true).Background(lipgloss.Color(nd.BG)).Foreground(lipgloss.Color(DefaultHighlightFG))
		}
		b.WriteString(st.Render(chunk))
	}
	return b.String()
}

// SearchMatchesLine reports whether line matches any needle (OR), using the same rules as highlight painting.
func SearchMatchesLine(line, query string, ignoreCase bool) bool {
	return SearchMatchesLineWithNames(line, query, ignoreCase, nil)
}

// SearchMatchesLineWithNames uses the same name map as the UI (merged palette).
func SearchMatchesLineWithNames(line, query string, ignoreCase bool, names map[string]string) bool {
	needles := ParseHighlightNeedles(query, names)
	for _, nd := range needles {
		if nd.Text == "" {
			continue
		}
		if _, _, ok := findNextMatch(line, nd.Text, ignoreCase); ok {
			return true
		}
	}
	return false
}

func findNextMatch(s, q string, fold bool) (start, end int, ok bool) {
	if !fold {
		i := strings.Index(s, q)
		if i < 0 {
			return 0, 0, false
		}
		return i, i + len(q), true
	}
	sr := []rune(s)
	qr := []rune(q)
	if len(qr) == 0 {
		return 0, 0, false
	}
outer:
	for i := 0; i+len(qr) <= len(sr); i++ {
		for j := range qr {
			if unicode.ToLower(sr[i+j]) != unicode.ToLower(qr[j]) {
				continue outer
			}
		}
		byteStart := runePrefixLen(s, i)
		byteEnd := runePrefixLen(s, i+len(qr))
		return byteStart, byteEnd, true
	}
	return 0, 0, false
}

func runePrefixLen(s string, nRunes int) int {
	b := 0
	for i := 0; i < nRunes; i++ {
		_, w := utf8.DecodeRuneInString(s[b:])
		if w == 0 {
			break
		}
		b += w
	}
	return b
}
