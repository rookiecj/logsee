package filter

import (
	"testing"
)

func TestTokenize_QuotedSpaces(t *testing.T) {
	// Given: a filter string with a quoted token that contains spaces
	// When: Tokenize runs
	// Then: the quoted segment is a single token
	toks, err := Tokenize(`+foo +"bar baz" -x`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(toks) != 3 || toks[0] != "+foo" || toks[1] != "+bar baz" || toks[2] != "-x" {
		t.Fatalf("got %#v", toks)
	}
}

func TestParse_TagAndSimpleMixed(t *testing.T) {
	// Given: mixed simple and tag tokens (level uses bracket-shaped line)
	// When: Parse runs
	// Then: plain AND applies and level clause matches normalized severity
	prog, err := Parse(`+level:error api -noise`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(prog.Plain) != 2 || len(prog.Tags) != 1 {
		t.Fatalf("plain %d tags %d", len(prog.Plain), len(prog.Tags))
	}
	line := `[2026-01-01T00:00:00Z] error: api call`
	if !Match(line, prog, false, FormatBracket) {
		t.Fatal("expected match")
	}
	if Match(`[2026-01-01T00:00:00Z] error: noise`, prog, false, FormatBracket) {
		t.Fatal("expected no match due to noise")
	}
	if Match(`api only`, prog, false, FormatUnknown) {
		t.Fatal("expected no match due to missing level and pattern")
	}
}

func TestParse_EmptyMatchesAll(t *testing.T) {
	// Given: empty expression
	// When: Parse runs
	// Then: no rules and Match is always true
	prog, err := Parse("   ")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !prog.Empty() {
		t.Fatalf("expected empty program")
	}
	if !Match("anything", prog, false, FormatUnknown) {
		t.Fatal("expected match")
	}
}

func TestMatch_IgnoreCase(t *testing.T) {
	// Given: ignore-case flag
	// When: matching
	// Then: casing does not affect hit
	prog, err := Parse(`+ERR`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match("something err happened", prog, true, FormatUnknown) {
		t.Fatal("expected match")
	}
}

func TestTokenize_UnclosedQuote(t *testing.T) {
	// Given: an unclosed quote
	// When: Tokenize runs
	// Then: ErrUnclosedQuote
	_, err := Tokenize(`+"abc`)
	if err != ErrUnclosedQuote {
		t.Fatalf("got %v", err)
	}
}

func TestParse_InvalidBarePlus(t *testing.T) {
	// Given: incomplete token
	// When: Parse runs
	// Then: error
	_, err := Parse(`+`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMergeSplitTag_unquotedSpaceAfterColon(t *testing.T) {
	// Given: user types over_speed: false without quotes (Tokenize splits on space)
	// When: Parse merges tag-only + value token
	// Then: KV match against over_speed=false
	prog, err := Parse(`over_speed: false`)
	if err != nil {
		t.Fatal(err)
	}
	line := `off/set=280.3, over_speed=false,`
	if !Match(line, prog, false, FormatUnknown) {
		t.Fatal("expected unquoted over_speed: false to parse as one tag token")
	}
}

func TestGenericKV_overSpeedEquals_filterColon(t *testing.T) {
	// Given: filter tag token (value may use quotes if it contains spaces; see PRD §7)
	// When: over_speed:false matches over_speed=false in line
	// Then: line passes include
	prog, err := Parse(`over_speed:false`)
	if err != nil {
		t.Fatal(err)
	}
	line := `off/set=280.3, over_speed=false,`
	if !Match(line, prog, false, FormatUnknown) {
		t.Fatal("expected over_speed=false to match over_speed:false")
	}
}

func TestGenericKV_overSpeedQuotedFilterValueWithSpace(t *testing.T) {
	// Given: quoted single token so value can contain a space after ':' (Tokenize splits on space otherwise)
	// When: line uses = for the same key
	// Then: match
	prog, err := Parse(`"over_speed: false"`)
	if err != nil {
		t.Fatal(err)
	}
	line := `off/set=280.3, over_speed=false,`
	if !Match(line, prog, false, FormatUnknown) {
		t.Fatal("expected quoted filter value with space")
	}
}

func TestGenericKV_logSpaceAroundSeparator(t *testing.T) {
	// Given: PRD §7.4 allows spaces around = and : in the log line
	// When: same filter over_speed:false
	// Then: both lines match
	prog, err := Parse(`over_speed:false`)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`off/set=1, over_speed = false,`,
		`off/set=1, over_speed : false,`,
	} {
		if !Match(line, prog, false, FormatUnknown) {
			t.Fatalf("expected match for line %q", line)
		}
	}
}

func TestFilter_spaceBetweenTagAndColon_unsupported(t *testing.T) {
	// Given: filter written as tag-space-colon (not a single tag token per PRD §7.3)
	// When: line has KV over_speed=false without stray ':' substring for plain needles
	// Then: must not match as over_speed:false tag filter
	prog, err := Parse(`over_speed : false`)
	if err != nil {
		t.Fatal(err)
	}
	line := `off/set=1, over_speed=false,`
	if Match(line, prog, false, FormatUnknown) {
		t.Fatal("space between tag and colon in filter is not supported for KV")
	}
}

func TestGenericKV_overSpeedColonLine(t *testing.T) {
	// Given: line uses colon and space after colon
	// When: filter without space in value part
	// Then: match
	prog, err := Parse(`over_speed:false`)
	if err != nil {
		t.Fatal(err)
	}
	line := `off/set: 280.3, over_speed: false,`
	if !Match(line, prog, false, FormatUnknown) {
		t.Fatal("expected colon form in log line")
	}
}

func TestGenericKV_offsetSlash(t *testing.T) {
	// Given: tag name contains /
	// When: off/set:280.3 filter and = in line
	// Then: match
	prog, err := Parse(`+off/set:280.3`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match(`off/set=280.3, over_speed=false,`, prog, false, FormatUnknown) {
		t.Fatal("expected off/set=280.3")
	}
}

func TestGenericKV_ignoreCaseValue(t *testing.T) {
	// Given: ignore-case
	// When: line has FALSE
	// Then: match over_speed:false
	prog, err := Parse(`over_speed:false`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match(`over_speed=FALSE`, prog, true, FormatUnknown) {
		t.Fatal("expected EqualFold on value")
	}
}

func TestParse_plainORBranches(t *testing.T) {
	// Given: two plain needles separated by whole-token |
	// When: Parse and Match
	// Then: line matches if either substring appears (not both required)
	prog, err := Parse(`caching | current`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(prog.Alts) != 2 {
		t.Fatalf("expected 2 OR branches, got alts=%d plain=%d", len(prog.Alts), len(prog.Plain))
	}
	if !Match(`msg about caching`, prog, false, FormatUnknown) {
		t.Fatal("expected caching-only line")
	}
	if !Match(`msg about current`, prog, false, FormatUnknown) {
		t.Fatal("expected current-only line")
	}
	if !Match(`caching and current`, prog, false, FormatUnknown) {
		t.Fatal("expected both substrings still match")
	}
	if Match(`neither word`, prog, false, FormatUnknown) {
		t.Fatal("expected no match")
	}
}

func TestParse_plainORWithANDInsideBranch(t *testing.T) {
	// Given: OR of two AND-groups
	// When: Match
	// Then: (a AND b) OR c
	prog, err := Parse(`a b | c`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match(`x a y b z`, prog, false, FormatUnknown) {
		t.Fatal("expected a+b branch")
	}
	if !Match(`only c here`, prog, false, FormatUnknown) {
		t.Fatal("expected c branch")
	}
	if Match(`only a zee`, prog, false, FormatUnknown) {
		t.Fatal("expected no match: has a but not b, and no c")
	}
}

func TestParse_ORWithTag(t *testing.T) {
	// Given: tag filter OR plain needle
	// When: Match
	// Then: either branch suffices
	prog, err := Parse(`level:WARN | timeout`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match(`[2026-01-01T00:00:00Z] WARN: x`, prog, false, FormatBracket) {
		t.Fatal("expected WARN branch")
	}
	if !Match(`no bracket but timeout fired`, prog, false, FormatUnknown) {
		t.Fatal("expected plain timeout branch")
	}
	if Match(`[2026-01-01T00:00:00Z] INFO: no`, prog, false, FormatBracket) {
		t.Fatal("expected no match")
	}
}

func TestMergeSplit_doesNotAbsorbORSeparator(t *testing.T) {
	// Given: `key:` merge candidate where the next token is whole-token `|` (OR syntax)
	// When: Parse
	// Then: `|` is not merged into the tag value; OR splits into two branches
	prog, err := Parse(`kv: | plainneedle`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Alts) != 2 {
		t.Fatalf("expected 2 OR branches, got %d (merge may have swallowed |)", len(prog.Alts))
	}
}

func TestParse_ORMalformed(t *testing.T) {
	// Given: leading, trailing, or double |
	// When: Parse
	// Then: error
	for _, expr := range []string{`| a`, `a |`, `a || b`} {
		if _, err := Parse(expr); err == nil {
			t.Fatalf("expected error for %q", expr)
		}
	}
}

func TestTagExclude(t *testing.T) {
	// Given: negative tag filter
	// When: line contains that tag:value
	// Then: excluded
	prog, err := Parse(`-service:db ok`)
	if err != nil {
		t.Fatal(err)
	}
	if Match(`ok service:db down`, prog, false, FormatUnknown) {
		t.Fatal("expected exclude")
	}
	if !Match(`ok service:cache`, prog, false, FormatUnknown) {
		t.Fatal("expected match")
	}
}

func TestLevel_OR_sameTag(t *testing.T) {
	// Given: two include levels for the same tag
	// When: line severity is one of them
	// Then: match (OR within tag)
	prog, err := Parse(`+level:ERROR +level:WARN`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match(`[2026-01-01T00:00:00Z] WARN: x`, prog, false, FormatBracket) {
		t.Fatal("expected WARN match")
	}
	if !Match(`[2026-01-01T00:00:00Z] ERROR: x`, prog, false, FormatBracket) {
		t.Fatal("expected ERROR match")
	}
	if Match(`[2026-01-01T00:00:00Z] INFO: x`, prog, false, FormatBracket) {
		t.Fatal("expected INFO no match")
	}
}

func TestLevel_firstTokenWinsSameTagValue(t *testing.T) {
	// Given: +level:ERROR then -level:ERROR (same tag:value pair; first token wins)
	// When: line is ERROR level
	// Then: first + still applies; trailing - for same pair is ignored
	prog, err := Parse(`+level:ERROR -level:ERROR`)
	if err != nil {
		t.Fatal(err)
	}
	if !Match(`[2026-01-01T00:00:00Z] ERROR: x`, prog, false, FormatBracket) {
		t.Fatal("expected first + to win over later - for same (tag, value)")
	}
}

func TestLevel_laterSamePairIgnoredWhenFirstWasMinus(t *testing.T) {
	// Given: -level:ERROR then +level:ERROR
	// When: line is ERROR
	// Then: first - wins; later + for same pair ignored
	prog, err := Parse(`-level:ERROR +level:ERROR`)
	if err != nil {
		t.Fatal(err)
	}
	if Match(`[2026-01-01T00:00:00Z] ERROR: x`, prog, false, FormatBracket) {
		t.Fatal("expected first - to win; ERROR line excluded")
	}
}

func TestTag_omittedSignMeansInclude(t *testing.T) {
	// Given: tag token without leading +/-
	// When: Parse and Match
	// Then: treated as include
	prog, err := Parse(`level:ERROR`)
	if err != nil {
		t.Fatal(err)
	}
	line := `[2026-01-01T00:00:00Z] ERROR: x`
	if !Match(line, prog, false, FormatBracket) {
		t.Fatal("expected bare level: to be include")
	}
}

func TestLevel_androidLetterMapsToDEBUG(t *testing.T) {
	// Given: Android-shaped line and filter level:DEBUG
	// When: Match with FormatAndroid
	// Then: single letter D matches DEBUG
	prog, err := Parse(`+level:DEBUG`)
	if err != nil {
		t.Fatal(err)
	}
	line := `2022-12-29 04:00:18.823 30249-30321 ProfileInstaller com.google.samples.apps.sunflower D Installing profile`
	if !Match(line, prog, false, FormatAndroid) {
		t.Fatal("expected D to match level:DEBUG")
	}
}

func TestLevel_androidThreadtimeLetterMapsToINFO(t *testing.T) {
	// Given: Android threadtime-shaped line and filter level:INFO
	// When: Match with FormatAndroid
	// Then: single letter I matches INFO
	prog, err := Parse(`+level:INFO`)
	if err != nil {
		t.Fatal(err)
	}
	line := `12-18 19:50:15.581  2852  2852 I Tile.RotationLockTile: refreshState=rotation`
	if !Match(line, prog, false, FormatAndroid) {
		t.Fatal("expected I to match level:INFO for threadtime")
	}
}

func TestDetectLogFormat_android(t *testing.T) {
	// Given: sample lines that look like logcat
	// When: DetectLogFormat
	// Then: FormatAndroid
	lines := []string{
		`2022-12-29 04:00:18.823 1-2 Tag com.example D msg`,
		`noise`,
	}
	if g := DetectLogFormat(lines); g != FormatAndroid {
		t.Fatalf("got %v", g)
	}
}

func TestDetectLogFormat_androidThreadtime(t *testing.T) {
	// Given: sample lines that look like logcat -v threadtime
	// When: DetectLogFormat
	// Then: FormatAndroid
	lines := []string{
		`12-18 19:50:15.581  2852  2852 I Tile.RotationLockTile: refreshState=rotation`,
		`noise`,
	}
	if g := DetectLogFormat(lines); g != FormatAndroid {
		t.Fatalf("got %v", g)
	}
}

func TestDetectLogFormat_bracket(t *testing.T) {
	// Given: bracket timestamp lines
	// When: DetectLogFormat
	// Then: FormatBracket
	lines := []string{
		`[2026-01-01T00:00:00Z] INFO: hello`,
	}
	if g := DetectLogFormat(lines); g != FormatBracket {
		t.Fatalf("got %v", g)
	}
}
