package filter

import (
	"strings"
	"unicode"
)

// tagKeyRune reports whether r may appear inside a filter tag name (PRD §7.3).
func tagKeyRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '_', r == '/', r == '.', r == '-':
		return true
	default:
		return false
	}
}

func tagKeyByte(b byte) bool {
	return tagKeyRune(rune(b))
}

// genericTagMatches reports whether line contains key=value or key: value (or key:value)
// for the given ASCII filter key (already lowercased) and want value (trimmed compare).
// PRD §7.4: spaces/tabs before/after '=' or ':' in the log line are ignored.
func genericTagMatches(line, keyLower, want string, ignoreCase bool) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	lower := strings.ToLower(line)
	klen := len(keyLower)
	if klen == 0 {
		return false
	}
	start := 0
	for {
		i := strings.Index(lower[start:], keyLower)
		if i < 0 {
			return false
		}
		i += start
		if i > 0 && tagKeyByte(line[i-1]) {
			start = i + 1
			continue
		}
		j := i + klen
		if j > len(line) {
			return false
		}
		rest := line[j:]
		rest = strings.TrimLeft(rest, " \t")
		if len(rest) == 0 {
			start = i + 1
			continue
		}
		if rest[0] != '=' && rest[0] != ':' {
			start = i + 1
			continue
		}
		rest = strings.TrimLeft(rest[1:], " \t")
		got := extractKVValueToken(rest)
		if kvValueEqual(got, want, ignoreCase) {
			return true
		}
		start = i + 1
	}
}

func extractKVValueToken(s string) string {
	for i, r := range s {
		if r == ',' || unicode.IsSpace(r) {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

func kvValueEqual(got, want string, ignoreCase bool) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if ignoreCase {
		return strings.EqualFold(got, want)
	}
	return got == want
}
