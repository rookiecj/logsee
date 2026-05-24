package usecase

import (
	"reflect"
	"testing"
)

func TestTokenizeFilterSupportsQuotesAndTagValueMerge(t *testing.T) {
	tokens, err := TokenizeFilter(`timeout "over speed" over_speed: false tag:value`)
	if err != nil {
		t.Fatalf("tokenize filter: %v", err)
	}

	want := []string{"timeout", "over speed", "over_speed: false", "tag:value"}
	if !reflect.DeepEqual(tokens, want) {
		t.Fatalf("tokens = %#v, want %#v", tokens, want)
	}
}

func TestTokenizeFilterRejectsUnclosedQuotes(t *testing.T) {
	if _, err := TokenizeFilter(`timeout "over speed`); err == nil {
		t.Fatal("tokenize filter succeeded, want quote parse error")
	}
}

func TestFilterProgramMatchesBooleanCombinations(t *testing.T) {
	filter := mustCompileFilter(t, `timeout db | level:ERROR -ignored`, FilterOptions{LogType: LogTypeADB})

	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "and branch matches both simple terms",
			line: "request timeout while opening db",
			want: true,
		},
		{
			name: "and branch rejects missing simple term",
			line: "request timeout while opening cache",
			want: false,
		},
		{
			name: "or branch matches adb level",
			line: "01-02 03:04:05.000 123 456 E MyTag: failed request",
			want: true,
		},
		{
			name: "exclude term wins inside branch",
			line: "01-02 03:04:05.000 123 456 E MyTag: ignored failure",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filter.Match(tt.line); got != tt.want {
				t.Fatalf("Match(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestEmptyFilterMatchesEveryLine(t *testing.T) {
	filter := mustCompileFilter(t, " \t ", FilterOptions{})

	if !filter.Match("anything at all") {
		t.Fatal("empty filter did not match line")
	}
}

func TestCompileFilterRejectsInvalidSyntax(t *testing.T) {
	for _, input := range []string{
		`| timeout`,
		`timeout |`,
		`timeout || db`,
		`timeout|db`,
		`tag:`,
		`+`,
		`-`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := CompileFilter(input, FilterOptions{}); err == nil {
				t.Fatal("CompileFilter succeeded, want parse error")
			}
		})
	}
}

func TestFilterProgramUsesDetectedLogTypeLevelExtraction(t *testing.T) {
	tests := []struct {
		name    string
		logType LogType
		filter  string
		line    string
		want    bool
	}{
		{
			name:    "adb level is normalized",
			logType: LogTypeADB,
			filter:  "level:ERROR",
			line:    "01-02 03:04:05.000 123 456 E MyTag: failed",
			want:    true,
		},
		{
			name:    "kernel bracket level is extracted",
			logType: LogTypeKernel,
			filter:  "level:WARN",
			line:    "[  12.345] WARN: thermal throttling",
			want:    true,
		},
		{
			name:    "plain has no structural level",
			logType: LogTypePlain,
			filter:  "level:ERROR",
			line:    "level:ERROR in plain text",
			want:    false,
		},
		{
			name:    "negative level does not exclude lines without level",
			logType: LogTypePlain,
			filter:  "-level:ERROR",
			line:    "plain text",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := mustCompileFilter(t, tt.filter, FilterOptions{LogType: tt.logType})
			if got := filter.Match(tt.line); got != tt.want {
				t.Fatalf("Match(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestFilterProgramMatchesKVEquivalence(t *testing.T) {
	for _, line := range []string{
		"over_speed=false",
		"over_speed:false",
		"over_speed: false",
		"over_speed = false",
		"over_speed : false",
	} {
		t.Run(line, func(t *testing.T) {
			filter := mustCompileFilter(t, `over_speed:false`, FilterOptions{})
			if !filter.Match(line) {
				t.Fatalf("filter did not match equivalent KV line %q", line)
			}
		})
	}
}

func TestFilterProgramHonorsIgnoreCaseForFilters(t *testing.T) {
	filter := mustCompileFilter(t, `LEVEL:error TAG:VALUE`, FilterOptions{
		LogType:    LogTypeADB,
		IgnoreCase: true,
	})

	line := "01-02 03:04:05.000 123 456 E MyTag: tag=value"
	if !filter.Match(line) {
		t.Fatalf("ignore-case filter did not match %q", line)
	}
}

func TestFilterStateParseFailureKeepsPreviousFilterAndSearchText(t *testing.T) {
	state := NewFilterState(FilterOptions{})
	state.SetSearchText("needle")

	if err := state.ApplyFilter("timeout"); err != nil {
		t.Fatalf("apply initial filter: %v", err)
	}
	if err := state.ApplyFilter(`broken "quote`); err == nil {
		t.Fatal("apply invalid filter succeeded, want error")
	}

	if got, want := state.FilterText(), "timeout"; got != want {
		t.Fatalf("filter text = %q, want previous valid %q", got, want)
	}
	if got, want := state.SearchText(), "needle"; got != want {
		t.Fatalf("search text = %q, want unchanged %q", got, want)
	}
	if !state.Match("request timeout") || state.Match("request success") {
		t.Fatal("state did not keep previous compiled filter")
	}
}

func TestApplyFilterToRawLogsPreservesRawLineNumbers(t *testing.T) {
	filter := mustCompileFilter(t, "keep", FilterOptions{})
	lines := []RawLogLine{
		{RawLineNumber: 7, Text: "drop this"},
		{RawLineNumber: 8, Text: "keep this"},
		{RawLineNumber: 42, Text: "also keep"},
	}

	got := ApplyFilterToRawLogs(lines, filter)
	want := []OutputLogRecord{
		{RawLineNumber: 8, Text: "keep this"},
		{RawLineNumber: 42, Text: "also keep"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output logs = %#v, want %#v", got, want)
	}
}

func mustCompileFilter(t *testing.T, input string, options FilterOptions) CompiledFilter {
	t.Helper()

	filter, err := CompileFilter(input, options)
	if err != nil {
		t.Fatalf("compile filter %q: %v", input, err)
	}
	return filter
}
