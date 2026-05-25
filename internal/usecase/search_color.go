package usecase

import "strings"

// SearchColorNames lists supported search highlight color suffixes (keyword#color).
var SearchColorNames = []string{
	"red",
	"green",
	"blue",
	"yellow",
	"cyan",
	"magenta",
	"white",
	"black",
	"orange",
	"purple",
	"pink",
}

func IsSearchColorName(name string) bool {
	_, ok := searchColorNames[strings.ToLower(name)]
	return ok
}

var searchColorNames = func() map[string]struct{} {
	names := make(map[string]struct{}, len(SearchColorNames))
	for _, name := range SearchColorNames {
		names[name] = struct{}{}
	}
	return names
}()

type SearchToken struct {
	Text  string
	Color string
}

func parseSearchColorSuffix(token string) (text string, color string) {
	idx := strings.LastIndex(token, "#")
	if idx < 0 {
		return token, ""
	}
	candidate := token[idx+1:]
	if candidate == "" || !IsSearchColorName(candidate) {
		return token, ""
	}
	return token[:idx], strings.ToLower(candidate)
}

func consumeSearchColorSuffix(input string, pos int, token string) (string, int, string) {
	if pos >= len(input) || input[pos] != '#' {
		return token, pos, ""
	}
	end := pos + 1
	for end < len(input) && isSearchColorChar(input[end]) {
		end++
	}
	name := input[pos+1 : end]
	if !IsSearchColorName(name) {
		return token, pos, ""
	}
	return token, end, strings.ToLower(name)
}

func isSearchColorChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
