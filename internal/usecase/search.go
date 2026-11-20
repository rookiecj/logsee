package usecase

import (
	"container/heap"
	"sort"
	"strings"
)

type SearchFocus int

const (
	SearchFocusLogList SearchFocus = iota
	SearchFocusFilterInput
	SearchFocusInput
)

type SearchKey int

const (
	SearchKeySlash SearchKey = iota
	SearchKeyEnter
	SearchKeyEsc
	SearchKeyUp
	SearchKeyDown
)

type SearchDirection int

const (
	SearchDirectionNext SearchDirection = iota
	SearchDirectionPrevious
)

type HighlightRange struct {
	Start int
	End   int
	Color string
}

type SearchState struct {
	focus         SearchFocus
	previousFocus SearchFocus
	searchText    string
	editingText   string
}

type SearchMatcher struct {
	tokens []SearchToken
}

type HighlightedOutputLogRecord struct {
	OutputLogRecord
	Highlights []HighlightRange
}

func NewSearchState() *SearchState {
	return &SearchState{
		focus:         SearchFocusLogList,
		previousFocus: SearchFocusLogList,
	}
}

func (s *SearchState) HandleKey(key SearchKey) {
	switch key {
	case SearchKeySlash:
		s.previousFocus = s.focus
		s.focus = SearchFocusInput
		s.editingText = s.searchText
	case SearchKeyEnter:
		if s.focus != SearchFocusInput {
			return
		}
		s.searchText = s.editingText
		s.focus = s.previousFocus
	case SearchKeyEsc:
		if s.focus != SearchFocusInput {
			return
		}
		s.editingText = s.searchText
		s.focus = s.previousFocus
	case SearchKeyUp:
		if s.focus == SearchFocusInput {
			s.focus = SearchFocusFilterInput
		}
	case SearchKeyDown:
		if s.focus == SearchFocusInput {
			s.focus = SearchFocusLogList
		}
	}
}

func (s *SearchState) SetEditingText(input string) {
	s.editingText = input
}

func (s *SearchState) Focus() SearchFocus {
	return s.focus
}

func (s *SearchState) SearchText() string {
	return s.searchText
}

func (s *SearchState) EditingText() string {
	return s.editingText
}

func TokenizeSearch(input string) []string {
	tokens, ok := tokenizeSearch(input)
	if !ok {
		if input == "" {
			return nil
		}
		text, _ := parseSearchColorSuffix(input)
		return []string{text}
	}
	texts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		texts = append(texts, token.Text)
	}
	return texts
}

func NewSearchMatcher(input string) SearchMatcher {
	tokens, ok := tokenizeSearch(input)
	if !ok {
		if input == "" {
			return SearchMatcher{}
		}
		text, color := parseSearchColorSuffix(input)
		return SearchMatcher{tokens: []SearchToken{{Text: text, Color: color}}}
	}
	return SearchMatcher{tokens: tokens}
}

func (m SearchMatcher) Match(line string) bool {
	for _, token := range m.tokens {
		if token.Text != "" && strings.Contains(line, token.Text) {
			return true
		}
	}
	return false
}

func (m SearchMatcher) HighlightRanges(line string) []HighlightRange {
	var ranges []HighlightRange
	for _, token := range m.tokens {
		if token.Text == "" {
			continue
		}
		for start := 0; start <= len(line)-len(token.Text); {
			index := strings.Index(line[start:], token.Text)
			if index < 0 {
				break
			}
			matchStart := start + index
			ranges = append(ranges, HighlightRange{
				Start: matchStart,
				End:   matchStart + len(token.Text),
				Color: token.Color,
			})
			start = matchStart + 1
		}
	}
	return mergeHighlightRanges(ranges)
}

func NavigateSearchMatch(records []OutputLogRecord, matcher SearchMatcher, currentOutputIndex int, direction SearchDirection) (int, bool) {
	if len(records) == 0 || len(matcher.tokens) == 0 {
		return currentOutputIndex, false
	}
	if currentOutputIndex < 0 {
		currentOutputIndex = 0
	}
	if currentOutputIndex >= len(records) {
		currentOutputIndex = len(records) - 1
	}

	if direction == SearchDirectionPrevious {
		for i := currentOutputIndex - 1; i >= 0; i-- {
			if matcher.Match(records[i].Text) {
				return i, true
			}
		}
		return currentOutputIndex, false
	}

	for i := currentOutputIndex + 1; i < len(records); i++ {
		if matcher.Match(records[i].Text) {
			return i, true
		}
	}
	return currentOutputIndex, false
}

func ApplySearchHighlights(records []OutputLogRecord, matcher SearchMatcher) []HighlightedOutputLogRecord {
	highlighted := make([]HighlightedOutputLogRecord, len(records))
	for i, record := range records {
		highlighted[i] = HighlightedOutputLogRecord{
			OutputLogRecord: record,
			Highlights:      matcher.HighlightRanges(record.Text),
		}
	}
	return highlighted
}

func (s *NavigationState) MoveToSearchMatch(records []OutputLogRecord, matcher SearchMatcher, direction SearchDirection) bool {
	target, moved := NavigateSearchMatch(records, matcher, s.cursorOutputIndex, direction)
	if !moved {
		return false
	}
	s.cursorOutputIndex = target
	s.follow = false
	s.keepCursorVisible()
	return true
}

func (s *NavigationState) MoveToSearchMatchWithSelection(records []OutputLogRecord, matcher SearchMatcher, direction SearchDirection, selection *SelectionState) bool {
	moved := s.MoveToSearchMatch(records, matcher, direction)
	if moved && selection != nil {
		selection.ClearRange()
	}
	return moved
}

func tokenizeSearch(input string) ([]SearchToken, bool) {
	var tokens []SearchToken
	for i := 0; i < len(input); {
		for i < len(input) && isSearchSpace(input[i]) {
			i++
		}
		if i >= len(input) {
			break
		}

		if input[i] == '"' {
			token, next, ok := readQuotedSearchToken(input, i)
			if !ok {
				return nil, false
			}
			token, next, color := consumeSearchColorSuffix(input, next, token)
			if token != "" {
				tokens = append(tokens, SearchToken{Text: token, Color: color})
			}
			i = next
			continue
		}

		start := i
		for i < len(input) && !isSearchSpace(input[i]) {
			if input[i] == '"' {
				return nil, false
			}
			i++
		}
		if raw := input[start:i]; raw != "" {
			text, color := parseSearchColorSuffix(raw)
			tokens = append(tokens, SearchToken{Text: text, Color: color})
		}
	}
	return tokens, true
}

func readQuotedSearchToken(input string, start int) (string, int, bool) {
	var builder strings.Builder
	for i := start + 1; i < len(input); i++ {
		if input[i] == '"' {
			return builder.String(), i + 1, true
		}
		builder.WriteByte(input[i])
	}
	return "", 0, false
}

func isSearchSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

// ResolveHighlightRanges partitions overlaps; the first supplied range wins.
func ResolveHighlightRanges(ranges []HighlightRange) []HighlightRange {
	type event struct {
		pos, id int
		start   bool
	}
	events := make([]event, 0, 2*len(ranges))
	for id, r := range ranges {
		if r.Start < r.End {
			events = append(events, event{r.Start, id, true}, event{r.End, id, false})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].pos < events[j].pos })
	active := make([]bool, len(ranges))
	h := &highlightHeap{}
	var out []HighlightRange
	for i := 0; i < len(events); {
		pos := events[i].pos
		for i < len(events) && events[i].pos == pos {
			e := events[i]
			active[e.id] = e.start
			if e.start {
				heap.Push(h, e.id)
			}
			i++
		}
		for h.Len() > 0 && !active[(*h)[0]] {
			heap.Pop(h)
		}
		if i == len(events) || h.Len() == 0 {
			continue
		}
		next := events[i].pos
		color := ranges[(*h)[0]].Color
		if len(out) > 0 && out[len(out)-1].End == pos && out[len(out)-1].Color == color {
			out[len(out)-1].End = next
		} else {
			out = append(out, HighlightRange{pos, next, color})
		}
	}
	return out
}
func mergeHighlightRanges(ranges []HighlightRange) []HighlightRange {
	return ResolveHighlightRanges(ranges)
}

type highlightHeap []int

func (h highlightHeap) Len() int           { return len(h) }
func (h highlightHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h highlightHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *highlightHeap) Push(v any)        { *h = append(*h, v.(int)) }
func (h *highlightHeap) Pop() any          { old := *h; n := len(old); v := old[n-1]; *h = old[:n-1]; return v }
