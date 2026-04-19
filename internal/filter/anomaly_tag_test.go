package filter

import "testing"

func TestAnomalyTag_AnyMatchesAnyFinding(t *testing.T) {
	p, err := Parse("anomaly:any")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cases := []struct {
		finding string
		want    bool
	}{
		{"", false},
		{"anr", true},
		{"fatal_java", true},
		{"oom", true},
	}
	for _, tc := range cases {
		ctx := MatchContext{Seq: 1, Finding: tc.finding}
		got := MatchWithContext("some line", ctx, p, false, FormatUnknown)
		if got != tc.want {
			t.Errorf("finding=%q: got %v, want %v", tc.finding, got, tc.want)
		}
	}
}

func TestAnomalyTag_SpecificKind(t *testing.T) {
	p, err := Parse("anomaly:anr")
	if err != nil {
		t.Fatal(err)
	}
	if !MatchWithContext("x", MatchContext{Finding: "anr"}, p, false, FormatUnknown) {
		t.Error("anr finding should match anomaly:anr")
	}
	if MatchWithContext("x", MatchContext{Finding: "fatal_java"}, p, false, FormatUnknown) {
		t.Error("fatal_java must not match anomaly:anr")
	}
	if MatchWithContext("x", MatchContext{Finding: ""}, p, false, FormatUnknown) {
		t.Error("empty finding must not match")
	}
}

func TestAnomalyTag_MultipleIncludesOR(t *testing.T) {
	p, err := Parse("anomaly:anr anomaly:fatal_java")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"anr", "fatal_java"} {
		if !MatchWithContext("x", MatchContext{Finding: f}, p, false, FormatUnknown) {
			t.Errorf("%q should match either anr or fatal_java", f)
		}
	}
	if MatchWithContext("x", MatchContext{Finding: "oom"}, p, false, FormatUnknown) {
		t.Error("oom should not match include list anr|fatal_java")
	}
}

func TestAnomalyTag_Exclude(t *testing.T) {
	p, err := Parse("anomaly:any -anomaly:oom")
	if err != nil {
		t.Fatal(err)
	}
	if MatchWithContext("x", MatchContext{Finding: "oom"}, p, false, FormatUnknown) {
		t.Error("excluded oom should not match")
	}
	if !MatchWithContext("x", MatchContext{Finding: "anr"}, p, false, FormatUnknown) {
		t.Error("anr should still match after -anomaly:oom")
	}
}

func TestAnomalyTag_EmptyContextLeavesProgramMatchable(t *testing.T) {
	// Without anomaly: clauses, empty MatchContext should be indistinguishable
	// from plain Match.
	p, err := Parse("hello +world")
	if err != nil {
		t.Fatal(err)
	}
	line := "hello world"
	if Match(line, p, false, FormatUnknown) != MatchWithContext(line, MatchContext{}, p, false, FormatUnknown) {
		t.Error("Match and MatchWithContext with empty ctx must agree on non-context programs")
	}
}

func TestAnomalyTag_CaseInsensitiveValue(t *testing.T) {
	p, err := Parse("anomaly:ANR")
	if err != nil {
		t.Fatal(err)
	}
	if !MatchWithContext("x", MatchContext{Finding: "anr"}, p, false, FormatUnknown) {
		t.Error("value comparison should be case-insensitive")
	}
}

func TestAnomalyTag_InOR(t *testing.T) {
	p, err := Parse("anomaly:anr | crash")
	if err != nil {
		t.Fatal(err)
	}
	// left branch: anomaly:anr matches
	if !MatchWithContext("no substring", MatchContext{Finding: "anr"}, p, false, FormatUnknown) {
		t.Error("OR left branch should match")
	}
	// right branch: substring "crash"
	if !MatchWithContext("process crash detected", MatchContext{}, p, false, FormatUnknown) {
		t.Error("OR right branch should match on substring")
	}
	// neither
	if MatchWithContext("hello", MatchContext{}, p, false, FormatUnknown) {
		t.Error("neither branch should match")
	}
}
