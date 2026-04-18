package filter

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Program is a compiled filter: plain substring rules (AND) plus per-tag clauses (AND between tag keys).
// If Alts is non-empty, Match succeeds if any element matches (OR of branches); each branch uses only Plain and Tags (no nested Alts from Parse).
type Program struct {
	Plain []Rule
	Tags  []tagClause
	Alts  []Program
}

type tagClause struct {
	keyLower string
	include  []string
	exclude  []string
}

// Rule is one plain-token match (substring on the full line).
type Rule struct {
	Include bool
	Needle  string
}

// tagToken: optional leading + or -; omitted sign means include (+). Tag chars: PRD §7.3.
var tagToken = regexp.MustCompile(`^([+-]?)([A-Za-z0-9_/.\-]+):(.+)$`)

var ErrUnclosedQuote = errors.New("unclosed double quote")

// reTagColonOnly matches a tag key and ':' with no value yet (split across Tokenize on space).
var reTagColonOnly = regexp.MustCompile(`^([+-]?)([A-Za-z0-9_/.\-]+):\s*$`)

// mergeSplitTagValues joins `over_speed:` + `false` into `over_speed: false` (PRD §7: unquoted space after ':').
// PRD §7.3: there is no space between tag chars and ':' in the filter; this only merges value split onto the next token.
// The continuation token must not contain ':' so values like `a:b` still use a single quoted token.
func mergeSplitTagValues(toks []string) []string {
	var out []string
	i := 0
	for i < len(toks) {
		if i+1 < len(toks) {
			t, u := toks[i], toks[i+1]
			// Do not merge across OR: `key:` + `|` must stay split so splitORBranches can see `|`.
			if reTagColonOnly.MatchString(t) && !strings.Contains(u, ":") && u != "|" {
				left := strings.TrimRight(t, " \t")
				out = append(out, left+" "+strings.TrimSpace(u))
				i += 2
				continue
			}
		}
		out = append(out, toks[i])
		i++
	}
	return out
}

// splitORBranches splits tokens on a whole-token "|". Each branch is non-empty; "|" at start/end or "||" yields an error.
func splitORBranches(toks []string) ([][]string, error) {
	var groups [][]string
	var cur []string
	for _, t := range toks {
		if t == "|" {
			if len(cur) == 0 {
				return nil, fmt.Errorf("empty OR branch")
			}
			groups = append(groups, cur)
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) == 0 {
		if len(groups) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("empty OR branch")
	}
	groups = append(groups, cur)
	return groups, nil
}

// Tokenize splits on ASCII spaces and tabs; double-quoted segments keep inner spaces.
func Tokenize(expr string) ([]string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}
	var out []string
	var b strings.Builder
	inQuote := false
	for _, r := range expr {
		switch {
		case r == '"' && !inQuote:
			inQuote = true
		case r == '"' && inQuote:
			inQuote = false
		case (r == ' ' || r == '\t') && !inQuote:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if inQuote {
		return nil, ErrUnclosedQuote
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out, nil
}

// Parse builds a Program from a filter expression string.
// Empty or whitespace-only expr yields an empty Program (matches all lines).
func Parse(expr string) (Program, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Program{}, nil
	}
	toks, err := Tokenize(expr)
	if err != nil {
		return Program{}, err
	}
	if len(toks) == 0 {
		return Program{}, nil
	}
	toks = mergeSplitTagValues(toks)
	branches, err := splitORBranches(toks)
	if err != nil {
		return Program{}, err
	}
	if len(branches) == 1 {
		return parseFlatProgram(branches[0])
	}
	alts := make([]Program, 0, len(branches))
	for _, b := range branches {
		p, err := parseFlatProgram(b)
		if err != nil {
			return Program{}, err
		}
		alts = append(alts, p)
	}
	return Program{Alts: alts}, nil
}

func parseFlatProgram(toks []string) (Program, error) {
	var plain []Rule
	type tagTok struct {
		key string
		val string
		inc bool
	}
	var tagSeq []tagTok
	for _, t := range toks {
		if m := tagToken.FindStringSubmatch(t); m != nil {
			sign, tag, val := m[1], m[2], m[3]
			if tag == "" {
				return Program{}, fmt.Errorf("token %q: empty tag", t)
			}
			inc := sign != "-"
			tagSeq = append(tagSeq, tagTok{
				key: strings.ToLower(tag),
				val: val,
				inc: inc,
			})
			continue
		}
		rule, err := parsePlainToken(t)
		if err != nil {
			return Program{}, fmt.Errorf("token %q: %w", t, err)
		}
		plain = append(plain, rule)
	}
	// Per (tag, value) the first token in filter string order wins; later duplicate (tag, value) tokens are ignored (PRD §7.2).
	perKey := make(map[string]map[string]bool)
	var keyOrder []string
	for _, e := range tagSeq {
		if _, ok := perKey[e.key]; !ok {
			perKey[e.key] = make(map[string]bool)
			keyOrder = append(keyOrder, e.key)
		}
		m := perKey[e.key]
		if _, exists := m[e.val]; !exists {
			m[e.val] = e.inc
		}
	}
	var tags []tagClause
	for _, k := range keyOrder {
		m := perKey[k]
		var inc, exc []string
		for v, isIncl := range m {
			if isIncl {
				inc = append(inc, v)
			} else {
				exc = append(exc, v)
			}
		}
		tags = append(tags, tagClause{keyLower: k, include: inc, exclude: exc})
	}
	return Program{Plain: plain, Tags: tags}, nil
}

func parsePlainToken(t string) (Rule, error) {
	if len(t) == 0 {
		return Rule{}, errors.New("empty token")
	}
	// Tokenize leaves "||" as one rune-sequence; OR uses whole-token "|" with spaces.
	if t == "|" || (len(t) >= 2 && strings.Trim(t, "|") == "") {
		return Rule{}, fmt.Errorf("token %q: put spaces around | between alternatives (e.g. a | b)", t)
	}
	switch t[0] {
	case '+':
		if len(t) == 1 {
			return Rule{}, errors.New("incomplete + token")
		}
		return Rule{Include: true, Needle: t[1:]}, nil
	case '-':
		if len(t) == 1 {
			return Rule{}, errors.New("incomplete - token")
		}
		return Rule{Include: false, Needle: t[1:]}, nil
	default:
		return Rule{Include: true, Needle: t}, nil
	}
}

// Empty reports whether the program matches every line.
func (p Program) Empty() bool {
	if len(p.Alts) > 0 {
		for _, a := range p.Alts {
			if !a.Empty() {
				return false
			}
		}
		return true
	}
	return len(p.Plain) == 0 && len(p.Tags) == 0
}

// Match returns whether line satisfies the program under ignoreCase and fmt (for reserved tag level).
func Match(line string, p Program, ignoreCase bool, fmt LogFormat) bool {
	if len(p.Alts) > 0 {
		for _, a := range p.Alts {
			if Match(line, a, ignoreCase, fmt) {
				return true
			}
		}
		return false
	}
	if p.Empty() {
		return true
	}
	l := line
	if ignoreCase {
		l = strings.ToLower(line)
	}
	for _, ru := range p.Plain {
		needle := ru.Needle
		if ignoreCase {
			needle = strings.ToLower(ru.Needle)
		}
		hit := strings.Contains(l, needle)
		if ru.Include && !hit {
			return false
		}
		if !ru.Include && hit {
			return false
		}
	}
	for _, tc := range p.Tags {
		if !evalTagClause(line, tc, ignoreCase, fmt) {
			return false
		}
	}
	return true
}

func evalTagClause(line string, tc tagClause, ignoreCase bool, fmt LogFormat) bool {
	if tc.keyLower == "level" {
		return evalLevelClause(line, tc, ignoreCase, fmt)
	}
	return evalGenericTagClause(line, tc, ignoreCase)
}

func evalLevelClause(line string, tc tagClause, ignoreCase bool, fmt LogFormat) bool {
	raw, ok := ExtractRawLevel(line, fmt)
	lev := ""
	if ok {
		lev = normalizeSeverity(raw)
	}
	for _, ex := range tc.exclude {
		if severityEquals(lev, ex, ignoreCase) {
			return false
		}
	}
	if len(tc.include) == 0 {
		return true
	}
	if lev == "" {
		return false
	}
	for _, in := range tc.include {
		if severityEquals(lev, in, ignoreCase) {
			return true
		}
	}
	return false
}

func evalGenericTagClause(line string, tc tagClause, ignoreCase bool) bool {
	var hitInc bool
	for _, v := range tc.include {
		if genericTagMatches(line, tc.keyLower, v, ignoreCase) {
			hitInc = true
			break
		}
	}
	var hitExc bool
	for _, v := range tc.exclude {
		if genericTagMatches(line, tc.keyLower, v, ignoreCase) {
			hitExc = true
			break
		}
	}
	if len(tc.include) > 0 && !hitInc {
		return false
	}
	if len(tc.exclude) > 0 && hitExc {
		return false
	}
	return true
}
