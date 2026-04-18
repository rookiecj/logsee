package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/config"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestHighlight_MultipleOccurrences(t *testing.T) {
	// Given: a line with repeated needle
	// When: Highlight runs
	// Then: both occurrences are wrapped with style markers
	hi := lipgloss.NewStyle().Bold(true)
	out := Highlight("foo bar foo", "foo", false, hi)
	if strings.Count(out, "foo") < 2 {
		t.Fatalf("expected both occurrences preserved: %q", out)
	}
}

func TestSearchMatchesLine_Fold(t *testing.T) {
	// Given: ignore-case search
	// When: SearchMatchesLine
	// Then: matches folded substring
	if !SearchMatchesLine("Hello", "ell", true) {
		t.Fatal("expected match")
	}
	if SearchMatchesLine("Hello", "xyz", true) {
		t.Fatal("expected no match")
	}
}

func TestSearchMatchesLine_caseSensitive(t *testing.T) {
	// Given: case-sensitive match (same API flag as TUI highlight)
	// When: SearchMatchesLine with ignoreCase false
	// Then: exact case must match
	if !SearchMatchesLine("Hello", "ell", false) {
		t.Fatal("expected substring ell in Hello")
	}
	if SearchMatchesLine("Hello", "ELL", false) {
		t.Fatal("expected no match for wrong case")
	}
}

func TestHighlight_FoldMatch(t *testing.T) {
	// Given: ignore-case enabled
	// When: Highlight runs
	// Then: folded match is highlighted
	hi := lipgloss.NewStyle().Bold(true)
	out := Highlight("Hello", "ell", true, hi)
	if !strings.Contains(out, "ell") {
		t.Fatalf("expected highlight region: %q", out)
	}
}

func TestSearchQueryTokens_quotedPhrase(t *testing.T) {
	// Given: space-separated tokens and a quoted phrase with spaces
	// When: SearchQueryTokens
	// Then: quoted segment is one token; unquoted are split
	toks := SearchQueryTokens(`foo "bar baz" qux`)
	if len(toks) != 3 || toks[0] != "foo" || toks[1] != "bar baz" || toks[2] != "qux" {
		t.Fatalf("tokens: %#v", toks)
	}
}

func TestSearchQueryTokens_unclosedQuoteFallsBackToSingleNeedle(t *testing.T) {
	// Given: unclosed quote (tokenize error)
	// When: SearchQueryTokens
	// Then: whole trimmed string is one needle
	q := `"broken`
	toks := SearchQueryTokens(q)
	if len(toks) != 1 || toks[0] != q {
		t.Fatalf("want single fallback token, got %#v", toks)
	}
}

func TestSearchMatchesLine_multiToken_OR(t *testing.T) {
	// Given: OR semantics — line matches if any token matches
	// When: SearchMatchesLine
	// Then:
	if !SearchMatchesLine("only alpha here", "bravo alp", true) {
		t.Fatal("expected match on token alp")
	}
	if SearchMatchesLine("none", "x y", true) {
		t.Fatal("expected no match")
	}
}

func TestHighlight_multiToken_mergesOverlaps(t *testing.T) {
	// Given: line where two needles overlap in highlight span
	// When: Highlight
	// Then: strip ANSI recovers plain text
	hi := lipgloss.NewStyle().Bold(true)
	out := Highlight("abab", "ab a", true, hi)
	if got := ansi.Strip(out); got != "abab" {
		t.Fatalf("strip: want abab got %q", got)
	}
}

func TestParseHighlightNeedles_numericAndNamed(t *testing.T) {
	// Given: query with #ANSI and #name tokens
	// When: ParseHighlightNeedles
	// Then: needles have Text and BG resolved
	names := config.MergeHighlightColorNames(map[string]string{"mine": "39"})
	q := `foo#214 "a b"#red mineWord#mine`
	nd := ParseHighlightNeedles(q, names)
	if len(nd) != 3 {
		t.Fatalf("want 3 needles, got %d %#v", len(nd), nd)
	}
	if nd[0].Text != "foo" || nd[0].BG != "214" {
		t.Fatalf("foo: %#v", nd[0])
	}
	wantRed := config.MergeHighlightColorNames(nil)["red"]
	if nd[1].Text != "a b" || nd[1].BG != wantRed {
		t.Fatalf("phrase: %#v", nd[1])
	}
	if nd[2].Text != "mineWord" || nd[2].BG != "39" {
		t.Fatalf("custom name: %#v", nd[2])
	}
}

func TestParseHighlightNeedles_invalidColorSkipsToken(t *testing.T) {
	// Given: one token has invalid #color
	// When: ParseHighlightNeedles
	// Then: that token is omitted; other needles remain
	nd := ParseHighlightNeedles(`ok#999 good#40`, nil)
	if len(nd) != 1 || nd[0].Text != "good" || nd[0].BG != "40" {
		t.Fatalf("got %#v", nd)
	}
}

func TestHighlightFromNeedles_overlapLaterTokenWins(t *testing.T) {
	// Given: overlapping needles on the same line
	// When: HighlightFromNeedles
	// Then: strip recovers plain text
	hi := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("0"))
	q := `a#196 ab#40`
	nd := ParseHighlightNeedles(q, nil)
	out := HighlightFromNeedles("xaby", nd, false, hi)
	if got := ansi.Strip(out); got != "xaby" {
		t.Fatalf("strip: want xaby got %q", got)
	}
}

func TestHighlightWithReverseStyles_stripsToPlain(t *testing.T) {
	// Given: cursor-style plain and match segments (reverse + highlight colors)
	// When: multiple matches and text after the last match
	// Then: stripping ANSI recovers the original visible string (full line content preserved)
	hi := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("0"))
	plainSeg := lipgloss.NewStyle().Reverse(true)
	matchSeg := plainSeg.Inherit(hi)
	in := "LEFTmidRIGHTtail"
	out := HighlightWithReverseStyles(in, "mid", false, plainSeg, matchSeg, nil)
	if got := ansi.Strip(out); got != in {
		t.Fatalf("strip: want %q got %q", in, got)
	}
	out2 := HighlightWithReverseStyles(in, "RIGHT", false, plainSeg, matchSeg, nil)
	if got := ansi.Strip(out2); got != in {
		t.Fatalf("strip (match not at end): want %q got %q", in, got)
	}
}
