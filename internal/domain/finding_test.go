package domain

import (
	"encoding/json"
	"testing"
)

func TestFindingKind_Names(t *testing.T) {
	if got := FindingANR.String(); got != "anr" {
		t.Errorf("FindingANR.String() = %q, want anr", got)
	}
	if got := FindingFatalJava.String(); got != "fatal_java" {
		t.Errorf("FindingFatalJava.String() = %q, want fatal_java", got)
	}
	if got := FindingKind(9999).String(); got != "unknown" {
		t.Errorf("out-of-range kind should stringify to unknown, got %q", got)
	}
}

func TestFindingKind_JSONRoundTripAllValues(t *testing.T) {
	// All declared kinds must round-trip through text form.
	declared := []FindingKind{
		FindingUnknown,
		FindingFatalJava,
		FindingANR,
		FindingNativeCrashHeader,
		FindingWatchdog,
		FindingLMKKill,
		FindingBinderFail,
		FindingSELinuxDenied,
		FindingWTF,
		FindingOOM,
		FindingGCStorm,
		FindingWakelockLeak,
		FindingBootLoop,
		FindingRareTemplate,
		FindingBurst,
		FindingBaselineDeviation,
	}
	for _, k := range declared {
		b, err := json.Marshal(k)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", k, err)
		}
		var got FindingKind
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", b, err)
		}
		if got != k {
			t.Errorf("round-trip %v -> %s -> %v", k, b, got)
		}
	}
}

func TestFindingKind_UnknownNameDecodesToUnknown(t *testing.T) {
	var k FindingKind = FindingANR
	if err := json.Unmarshal([]byte(`"totally_new_kind"`), &k); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if k != FindingUnknown {
		t.Errorf("unknown kind should decode to Unknown, got %v", k)
	}
}

func TestFinding_JSONRoundTrip(t *testing.T) {
	f := Finding{
		Kind:       FindingANR,
		Seq:        42,
		SpanID:     101,
		Severity:   LevelError,
		Confidence: 0.95,
		Fields: map[string]string{
			"pid":       "12345",
			"component": "com.example.app",
			"reason":    "Input dispatching timed out",
		},
		SchemaVer: SchemaVersion,
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Finding
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Kind != f.Kind || got.Seq != f.Seq || got.SpanID != f.SpanID ||
		got.Severity != f.Severity || got.Confidence != f.Confidence ||
		got.SchemaVer != f.SchemaVer {
		t.Errorf("scalar fields mismatch\n got=%#v\nwant=%#v", got, f)
	}
	if len(got.Fields) != len(f.Fields) {
		t.Fatalf("Fields len mismatch: got %d want %d", len(got.Fields), len(f.Fields))
	}
	for k, v := range f.Fields {
		if got.Fields[k] != v {
			t.Errorf("Fields[%q]: got %q want %q", k, got.Fields[k], v)
		}
	}
}

func TestFinding_ZeroValueJSON(t *testing.T) {
	var f Finding
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"kind":"unknown","seq":0}`
	if string(b) != want {
		t.Errorf("zero-value Finding JSON = %s, want %s", b, want)
	}
}
