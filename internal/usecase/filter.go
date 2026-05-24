package usecase

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type FilterOptions struct {
	LogType    LogType
	IgnoreCase bool
}

type RawLogLine struct {
	RawLineNumber int
	Text          string
}

type OutputLogRecord struct {
	RawLineNumber int
	Text          string
}

type CompiledFilter struct {
	options  FilterOptions
	branches []filterBranch
}

type filterBranch struct {
	simpleIncludes []string
	simpleExcludes []string
	tags           map[string]tagConditions
}

type tagConditions struct {
	includes []string
	excludes []string
	seen     map[string]struct{}
}

type parsedToken struct {
	negative bool
	tag      string
	value    string
}

type FilterState struct {
	options    FilterOptions
	filterText string
	searchText string
	filter     CompiledFilter
}

func TokenizeFilter(input string) ([]string, error) {
	var tokens []string
	for i := 0; i < len(input); {
		for i < len(input) && isFilterSpace(input[i]) {
			i++
		}
		if i >= len(input) {
			break
		}

		if input[i] == '"' {
			token, next, err := readQuotedFilterToken(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
			continue
		}

		start := i
		for i < len(input) && !isFilterSpace(input[i]) {
			if input[i] == '"' {
				return nil, fmt.Errorf("unexpected quote in filter token")
			}
			i++
		}
		tokens = append(tokens, input[start:i])
	}

	merged := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if strings.HasSuffix(token, ":") && i+1 < len(tokens) && canMergeTagValue(tokens[i+1]) {
			merged = append(merged, token+" "+tokens[i+1])
			i++
			continue
		}
		merged = append(merged, token)
	}
	return merged, nil
}

func CompileFilter(input string, options FilterOptions) (CompiledFilter, error) {
	tokens, err := TokenizeFilter(input)
	if err != nil {
		return CompiledFilter{}, err
	}
	if len(tokens) == 0 {
		return CompiledFilter{options: options}, nil
	}

	filter := CompiledFilter{options: options}
	current := newFilterBranch()
	expectToken := true
	for _, token := range tokens {
		if token == "||" {
			return CompiledFilter{}, fmt.Errorf("invalid OR token %q", token)
		}
		if token == "|" {
			if expectToken {
				return CompiledFilter{}, fmt.Errorf("empty filter branch before OR")
			}
			filter.branches = append(filter.branches, current)
			current = newFilterBranch()
			expectToken = true
			continue
		}
		parsed, err := parseFilterToken(token)
		if err != nil {
			return CompiledFilter{}, err
		}
		addParsedToken(&current, parsed, options)
		expectToken = false
	}
	if expectToken {
		return CompiledFilter{}, fmt.Errorf("empty filter branch after OR")
	}
	filter.branches = append(filter.branches, current)
	return filter, nil
}

func (f CompiledFilter) Match(line string) bool {
	if len(f.branches) == 0 {
		return true
	}
	for _, branch := range f.branches {
		if f.matchBranch(branch, line) {
			return true
		}
	}
	return false
}

func NewFilterState(options FilterOptions) *FilterState {
	return &FilterState{
		options: options,
		filter:  CompiledFilter{options: options},
	}
}

func (s *FilterState) ApplyFilter(input string) error {
	filter, err := CompileFilter(input, s.options)
	if err != nil {
		return err
	}
	s.filterText = input
	s.filter = filter
	return nil
}

func (s *FilterState) SetSearchText(input string) {
	s.searchText = input
}

func (s *FilterState) FilterText() string {
	return s.filterText
}

func (s *FilterState) SearchText() string {
	return s.searchText
}

func (s *FilterState) Match(line string) bool {
	return s.filter.Match(line)
}

func ApplyFilterToRawLogs(lines []RawLogLine, filter CompiledFilter) []OutputLogRecord {
	output := make([]OutputLogRecord, 0, len(lines))
	for _, line := range lines {
		if filter.Match(line.Text) {
			output = append(output, OutputLogRecord{
				RawLineNumber: line.RawLineNumber,
				Text:          line.Text,
			})
		}
	}
	return output
}

func (f CompiledFilter) matchBranch(branch filterBranch, line string) bool {
	for _, term := range branch.simpleExcludes {
		if containsFilterValue(line, term, f.options.IgnoreCase) {
			return false
		}
	}
	for _, term := range branch.simpleIncludes {
		if !containsFilterValue(line, term, f.options.IgnoreCase) {
			return false
		}
	}
	for tag, conditions := range branch.tags {
		for _, value := range conditions.excludes {
			if f.tagMatches(line, tag, value) {
				return false
			}
		}
		if len(conditions.includes) == 0 {
			continue
		}
		matchedInclude := false
		for _, value := range conditions.includes {
			if f.tagMatches(line, tag, value) {
				matchedInclude = true
				break
			}
		}
		if !matchedInclude {
			return false
		}
	}
	return true
}

func (f CompiledFilter) tagMatches(line, tag, value string) bool {
	if equalFilterValue(tag, "level", f.options.IgnoreCase) {
		level, ok := ExtractLogLevel(f.options.LogType, line)
		return ok && equalFilterValue(level, value, f.options.IgnoreCase)
	}
	values := extractKVValues(line, tag, f.options.IgnoreCase)
	for _, got := range values {
		if equalFilterValue(strings.TrimSpace(got), strings.TrimSpace(value), f.options.IgnoreCase) {
			return true
		}
	}
	return false
}

func readQuotedFilterToken(input string, start int) (string, int, error) {
	var builder strings.Builder
	for i := start + 1; i < len(input); i++ {
		if input[i] == '"' {
			return builder.String(), i + 1, nil
		}
		builder.WriteByte(input[i])
	}
	return "", 0, fmt.Errorf("unterminated quoted filter token")
}

func isFilterSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

func canMergeTagValue(next string) bool {
	return next != "|" && !strings.Contains(next, ":")
}

func newFilterBranch() filterBranch {
	return filterBranch{tags: map[string]tagConditions{}}
}

func parseFilterToken(token string) (parsedToken, error) {
	if token == "+" || token == "-" || token == "" {
		return parsedToken{}, fmt.Errorf("incomplete filter token %q", token)
	}
	if strings.Contains(token, "|") {
		return parsedToken{}, fmt.Errorf("invalid OR token %q", token)
	}
	parsed := parsedToken{value: token}
	if token[0] == '+' || token[0] == '-' {
		parsed.negative = token[0] == '-'
		token = token[1:]
		if token == "" {
			return parsedToken{}, fmt.Errorf("incomplete filter token")
		}
	}

	if strings.Contains(token, ":") {
		parts := strings.SplitN(token, ":", 2)
		if !validFilterTag(parts[0]) || strings.TrimSpace(parts[1]) == "" {
			return parsedToken{}, fmt.Errorf("invalid tag filter token %q", token)
		}
		parsed.tag = parts[0]
		parsed.value = strings.TrimSpace(parts[1])
		return parsed, nil
	}

	parsed.value = token
	return parsed, nil
}

func addParsedToken(branch *filterBranch, token parsedToken, options FilterOptions) {
	if token.tag == "" {
		if token.negative {
			branch.simpleExcludes = append(branch.simpleExcludes, token.value)
			return
		}
		branch.simpleIncludes = append(branch.simpleIncludes, token.value)
		return
	}

	tagKey := normalizeComparable(token.tag, options.IgnoreCase)
	conditions := branch.tags[tagKey]
	if conditions.seen == nil {
		conditions.seen = map[string]struct{}{}
	}
	seenKey := normalizeComparable(token.value, options.IgnoreCase)
	if _, ok := conditions.seen[seenKey]; ok {
		branch.tags[tagKey] = conditions
		return
	}
	conditions.seen[seenKey] = struct{}{}
	if token.negative {
		conditions.excludes = append(conditions.excludes, token.value)
	} else {
		conditions.includes = append(conditions.includes, token.value)
	}
	branch.tags[tagKey] = conditions
}

func validFilterTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, r := range tag {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '_', '/', '.', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func containsFilterValue(line, term string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.Contains(strings.ToLower(line), strings.ToLower(term))
	}
	return strings.Contains(line, term)
}

func equalFilterValue(a, b string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func normalizeComparable(value string, ignoreCase bool) string {
	if ignoreCase {
		return strings.ToLower(value)
	}
	return value
}

func extractKVValues(line, key string, ignoreCase bool) []string {
	var values []string
	searchLine := line
	searchKey := key
	if ignoreCase {
		searchLine = strings.ToLower(line)
		searchKey = strings.ToLower(key)
	}

	for offset := 0; offset < len(searchLine); {
		index := strings.Index(searchLine[offset:], searchKey)
		if index < 0 {
			break
		}
		start := offset + index
		end := start + len(searchKey)
		offset = end
		if start > 0 && isFilterIdentifierRune(rune(searchLine[start-1])) {
			continue
		}
		if end >= len(searchLine) || (searchLine[end] != '=' && searchLine[end] != ':' && !isFilterSpace(searchLine[end])) {
			continue
		}

		sep := end
		for sep < len(searchLine) && isFilterSpace(searchLine[sep]) {
			sep++
		}
		if sep >= len(searchLine) || (searchLine[sep] != '=' && searchLine[sep] != ':') {
			continue
		}
		valueStart := sep + 1
		for valueStart < len(searchLine) && isFilterSpace(searchLine[valueStart]) {
			valueStart++
		}
		valueEnd := valueStart
		for valueEnd < len(line) {
			r, size := utf8.DecodeRuneInString(line[valueEnd:])
			if r == ',' || unicode.IsSpace(r) {
				break
			}
			valueEnd += size
		}
		values = append(values, line[valueStart:valueEnd])
		offset = valueEnd
	}
	return values
}

func isFilterIdentifierRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' ||
		r == '_' || r == '/' || r == '.' || r == '-'
}
