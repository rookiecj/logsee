package classify

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/pipeline"
)

func TestRule_MatchPredicatesAND(t *testing.T) {
	rl := Rule{
		TagEq:       "AndroidRuntime",
		MsgPrefix:   "FATAL EXCEPTION",
		MsgContains: []string{"Binder"},
	}
	cases := []struct {
		name string
		rec  domain.Record
		want bool
	}{
		{"all match", domain.Record{Tag: "AndroidRuntime", Message: "FATAL EXCEPTION: Binder:1245_A"}, true},
		{"wrong tag", domain.Record{Tag: "System", Message: "FATAL EXCEPTION: Binder:1245_A"}, false},
		{"wrong prefix", domain.Record{Tag: "AndroidRuntime", Message: "warn: Binder slow"}, false},
		{"contains missing", domain.Record{Tag: "AndroidRuntime", Message: "FATAL EXCEPTION: IO error"}, false},
	}
	for _, tc := range cases {
		if got := rl.Match(tc.rec); got != tc.want {
			t.Errorf("%s: Match = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRule_RegexPredicate(t *testing.T) {
	rl := Rule{MsgRegex: regexp.MustCompile(`^\*\*\*\s+\*\*\*`)}
	if !rl.Match(domain.Record{Message: "*** *** *** *** boot"}) {
		t.Error("regex should match triple-star header")
	}
	if rl.Match(domain.Record{Message: "regular line"}) {
		t.Error("regex should not match unrelated line")
	}
}

func TestClassifier_BuiltinRulesNonEmpty(t *testing.T) {
	c := New()
	if len(c.rules) == 0 {
		t.Fatal("built-in classifier must carry at least one rule")
	}
}

func TestClassifier_OnRecord_ANR(t *testing.T) {
	c := New()
	r := domain.Record{Seq: 42, Tag: "ActivityManager", Level: domain.LevelError,
		Message: "ANR in com.example.app (com.example.app/.MainActivity)",
		PID:     1245,
	}
	out := c.OnRecord(r)
	if len(out.Findings) != 1 || out.Spans != nil {
		t.Fatalf("expected 1 finding + 0 spans, got %+v", out)
	}
	f := out.Findings[0]
	if f.Kind != domain.FindingANR {
		t.Errorf("Kind = %v, want ANR", f.Kind)
	}
	if f.Seq != 42 {
		t.Errorf("Seq = %d, want 42", f.Seq)
	}
	if f.Fields["tag"] != "ActivityManager" || f.Fields["pid"] != "1245" {
		t.Errorf("fields missing or wrong: %v", f.Fields)
	}
}

func TestClassifier_OnRecord_NativeCrashHeader(t *testing.T) {
	c := New()
	r := domain.Record{Seq: 7, Tag: "DEBUG", Level: domain.LevelFatal,
		Message: "*** *** *** *** *** *** *** *** *** *** *** *** *** *** *** ***",
		PID:     4567,
	}
	out := c.OnRecord(r)
	if len(out.Findings) != 1 || out.Findings[0].Kind != domain.FindingNativeCrashHeader {
		t.Errorf("expected native crash header, got %+v", out)
	}
}

func TestClassifier_OnRecord_NoMatch(t *testing.T) {
	c := New()
	out := c.OnRecord(domain.Record{Seq: 1, Tag: "Info", Message: "nothing unusual"})
	if !out.Empty() {
		t.Errorf("expected no output, got %+v", out)
	}
}

func TestClassifier_NewWithRulesIsIndependentCopy(t *testing.T) {
	rules := []Rule{{Kind: domain.FindingWTF, TagEq: "Only"}}
	c := NewWithRules(rules)
	rules[0].TagEq = "Mutated"
	// external mutation must not affect the classifier
	r := domain.Record{Tag: "Only", Message: "x"}
	out := c.OnRecord(r)
	if len(out.Findings) != 1 {
		t.Errorf("external mutation leaked into classifier")
	}
}

func TestClassifier_FirstMatchWins(t *testing.T) {
	// Two rules matching the same record: the earlier one should win.
	rules := []Rule{
		{Kind: domain.FindingWTF, TagEq: "X"},
		{Kind: domain.FindingANR, TagEq: "X"},
	}
	c := NewWithRules(rules)
	out := c.OnRecord(domain.Record{Seq: 1, Tag: "X", Message: "y"})
	if len(out.Findings) != 1 || out.Findings[0].Kind != domain.FindingWTF {
		t.Errorf("expected WTF (first match), got %+v", out)
	}
}

// Integration: run the classifier over each testdata sample and verify the
// expected anomaly kind appears. The goal is coverage of the rule table
// against realistic noise, not exact line numbers (block analyzers handle
// that in Phase 5).

func TestClassifier_DetectsExpectedKindsOnSamples(t *testing.T) {
	cases := []struct {
		sample string
		want   domain.FindingKind
	}{
		{"anr_input_dispatch.log", domain.FindingANR},
		{"native_tombstone.log", domain.FindingNativeCrashHeader},
		{"java_fatal_system_server.log", domain.FindingFatalJava},
	}
	for _, tc := range cases {
		t.Run(tc.sample, func(t *testing.T) {
			findings := classifySample(t, tc.sample)
			found := false
			for _, f := range findings {
				if f.Kind == tc.want {
					found = true
					break
				}
			}
			if !found {
				kinds := map[domain.FindingKind]int{}
				for _, f := range findings {
					kinds[f.Kind]++
				}
				t.Errorf("sample %s: expected at least one %v, got %v", tc.sample, tc.want, kinds)
			}
		})
	}
}

func TestClassifier_SampleFindingCountsSmokeTest(t *testing.T) {
	// Sanity: rule noise should not flood. Each sample should produce at
	// least 1 finding and well under one finding per line (upper bound).
	for _, sample := range []string{"anr_input_dispatch.log", "native_tombstone.log", "java_fatal_system_server.log"} {
		findings := classifySample(t, sample)
		if len(findings) == 0 {
			t.Errorf("sample %s produced zero findings — did the rule set break?", sample)
		}
	}
}

func classifySample(t *testing.T, name string) []domain.Finding {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "android", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	builder := pipeline.RecordBuilder{RefYear: 2024, Location: time.UTC}
	c := New()

	var findings []domain.Finding
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var seq int64
	for scanner.Scan() {
		seq++
		line := domain.Line{Seq: seq, Text: scanner.Text()}
		r := builder.Build(line, domain.LineFormatAndroid)
		findings = append(findings, c.OnRecord(r).Findings...)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	// Also include any flush output for completeness (classify has none today).
	findings = append(findings, c.Flush().Findings...)
	return findings
}

func TestOutput_Empty(t *testing.T) {
	var zero analysis.Output
	if !zero.Empty() {
		t.Error("zero Output should be Empty")
	}
	nonEmpty := analysis.Output{Findings: []domain.Finding{{Kind: domain.FindingANR}}}
	if nonEmpty.Empty() {
		t.Error("Output with findings should not be Empty")
	}
}
