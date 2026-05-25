package usecase

import (
	"reflect"
	"testing"
)

func TestSearchInputFocusAndApplyTransitions(t *testing.T) {
	state := NewSearchState()

	state.HandleKey(SearchKeySlash)
	if got, want := state.Focus(), SearchFocusInput; got != want {
		t.Fatalf("focus after / = %v, want %v", got, want)
	}

	state.SetEditingText("timeout")
	state.HandleKey(SearchKeyEnter)
	if got, want := state.SearchText(), "timeout"; got != want {
		t.Fatalf("search text after Enter = %q, want %q", got, want)
	}
	if got, want := state.Focus(), SearchFocusLogList; got != want {
		t.Fatalf("focus after Enter = %v, want %v", got, want)
	}

	state.HandleKey(SearchKeySlash)
	state.SetEditingText("discarded")
	state.HandleKey(SearchKeyEsc)
	if got, want := state.SearchText(), "timeout"; got != want {
		t.Fatalf("search text after Esc = %q, want previous %q", got, want)
	}
	if got, want := state.Focus(), SearchFocusLogList; got != want {
		t.Fatalf("focus after Esc = %v, want %v", got, want)
	}
}

func TestSearchInputUpAndDownMoveFocusWithoutApplyingEdit(t *testing.T) {
	state := NewSearchState()
	state.HandleKey(SearchKeySlash)
	state.SetEditingText("not applied")

	state.HandleKey(SearchKeyUp)
	if got, want := state.Focus(), SearchFocusFilterInput; got != want {
		t.Fatalf("focus after Up = %v, want %v", got, want)
	}
	if got := state.SearchText(); got != "" {
		t.Fatalf("search text after Up = %q, want unchanged empty", got)
	}

	state.HandleKey(SearchKeySlash)
	state.SetEditingText("still not applied")
	state.HandleKey(SearchKeyDown)
	if got, want := state.Focus(), SearchFocusLogList; got != want {
		t.Fatalf("focus after Down = %v, want %v", got, want)
	}
	if got := state.SearchText(); got != "" {
		t.Fatalf("search text after Down = %q, want unchanged empty", got)
	}
}

func TestTokenizeSearchParsesColorSuffixOnTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "unquoted color suffix",
			input: "error#red timeout#green",
			want:  []string{"error", "timeout"},
		},
		{
			name:  "quoted token with trailing color suffix",
			input: `timeout "over speed"#yellow db`,
			want:  []string{"timeout", "over speed", "db"},
		},
		{
			name:  "unknown suffix stays literal",
			input: "issue#123",
			want:  []string{"issue#123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenizeSearch(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tokens = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNewSearchMatcherAssignsPerTokenHighlightColors(t *testing.T) {
	matcher := NewSearchMatcher("foo#red bar#green")

	got := matcher.HighlightRanges("foo bar")
	want := []HighlightRange{
		{Start: 0, End: 3, Color: "red"},
		{Start: 4, End: 7, Color: "green"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("highlight ranges = %#v, want %#v", got, want)
	}
}

func TestSearchHighlightRangesMergeOverlapsOnlyForSameColor(t *testing.T) {
	matcher := NewSearchMatcher("error err")

	got := matcher.HighlightRanges("error err ERROR")
	want := []HighlightRange{
		{Start: 0, End: 5},
		{Start: 6, End: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("highlight ranges = %#v, want %#v", got, want)
	}
}

func TestTokenizeSearchSupportsWhitespaceQuotesAndMalformedQuoteFallback(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "whitespace tokens",
			input: "timeout\tretry db",
			want:  []string{"timeout", "retry", "db"},
		},
		{
			name:  "quoted token with spaces",
			input: `timeout "over speed" db`,
			want:  []string{"timeout", "over speed", "db"},
		},
		{
			name:  "malformed quote falls back to whole input",
			input: `timeout "over speed`,
			want:  []string{`timeout "over speed`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenizeSearch(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tokens = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSearchHighlightRangesSupportQuotedMultiTokenMatches(t *testing.T) {
	matcher := NewSearchMatcher(`"over speed" timeout`)

	got := matcher.HighlightRanges("over speed warning then timeout")
	want := []HighlightRange{
		{Start: 0, End: 10},
		{Start: 24, End: 31},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("highlight ranges = %#v, want %#v", got, want)
	}
}

func TestSearchMatcherRemainsCaseSensitiveAndIgnoresFilterIgnoreCase(t *testing.T) {
	filter := mustCompileFilter(t, "ERROR", FilterOptions{IgnoreCase: true})
	if !filter.Match("lower error matches filter") {
		t.Fatal("test setup: ignore-case filter should match lower-case line")
	}

	matcher := NewSearchMatcher("ERROR")
	if matcher.Match("lower error must not match search") {
		t.Fatal("search matcher used ignore-case behavior, want case-sensitive search")
	}
	if !matcher.Match("upper ERROR matches search") {
		t.Fatal("search matcher did not match exact-case token")
	}
}

func TestApplySearchHighlightsDoesNotFilterOutputLogs(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 1, Text: "idle"},
		{RawLineNumber: 2, Text: "timeout"},
		{RawLineNumber: 3, Text: "still visible"},
	}

	got := ApplySearchHighlights(records, NewSearchMatcher("timeout"))
	if len(got) != len(records) {
		t.Fatalf("highlighted records len = %d, want original len %d", len(got), len(records))
	}
	if len(got[0].Highlights) != 0 || len(got[2].Highlights) != 0 {
		t.Fatalf("non-matching records got highlights: %#v", got)
	}
	if want := []HighlightRange{{Start: 0, End: 7}}; !reflect.DeepEqual(got[1].Highlights, want) {
		t.Fatalf("matching record highlights = %#v, want %#v", got[1].Highlights, want)
	}
}

func TestNavigateSearchMatchMovesWithoutWrappingOverFilteredOutputLogs(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 10, Text: "raw match outside filter was removed"},
		{RawLineNumber: 20, Text: "visible idle"},
		{RawLineNumber: 30, Text: "visible timeout one"},
		{RawLineNumber: 40, Text: "visible idle"},
		{RawLineNumber: 50, Text: "visible timeout two"},
	}
	matcher := NewSearchMatcher("timeout")

	next, moved := NavigateSearchMatch(records[1:], matcher, 0, SearchDirectionNext)
	if !moved || next != 1 {
		t.Fatalf("next match over filtered records = %d,%v; want 1,true", next, moved)
	}

	next, moved = NavigateSearchMatch(records[1:], matcher, 3, SearchDirectionNext)
	if moved || next != 3 {
		t.Fatalf("next from last match = %d,%v; want 3,false", next, moved)
	}

	prev, moved := NavigateSearchMatch(records[1:], matcher, 3, SearchDirectionPrevious)
	if !moved || prev != 1 {
		t.Fatalf("previous match over filtered records = %d,%v; want 1,true", prev, moved)
	}

	prev, moved = NavigateSearchMatch(records[1:], matcher, 1, SearchDirectionPrevious)
	if moved || prev != 1 {
		t.Fatalf("previous from first match = %d,%v; want 1,false", prev, moved)
	}
}

func TestNavigationStateMovesToSearchMatchAndKeepsCursorVisible(t *testing.T) {
	state := mustNavigationState(t, NavigationOptions{
		OutputCount:       5,
		ViewportHeight:    2,
		CursorOutputIndex: 1,
		ScrollOffset:      0,
	})
	records := []OutputLogRecord{
		{Text: "idle"},
		{Text: "idle"},
		{Text: "idle"},
		{Text: "timeout"},
		{Text: "timeout"},
	}

	moved := state.MoveToSearchMatch(records, NewSearchMatcher("timeout"), SearchDirectionNext)
	if !moved {
		t.Fatal("MoveToSearchMatch did not move to next match")
	}
	assertNavigationState(t, state, 3, 2, false)
}
