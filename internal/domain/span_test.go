package domain

import (
	"encoding/json"
	"testing"
)

func TestSpanKind_String(t *testing.T) {
	cases := []struct {
		k    SpanKind
		want string
	}{
		{SpanUnknown, "unknown"},
		{SpanNativeCrash, "native_crash"},
		{SpanJavaFatal, "java_fatal"},
		{SpanANR, "anr"},
		{SpanWatchdog, "watchdog"},
		{SpanGCStorm, "gc_storm"},
		{SpanKind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("SpanKind(%d).String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}

func TestSpanKind_JSONRoundTrip(t *testing.T) {
	for _, k := range []SpanKind{SpanUnknown, SpanNativeCrash, SpanJavaFatal, SpanANR, SpanWatchdog, SpanGCStorm} {
		b, err := json.Marshal(k)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var got SpanKind
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got != k {
			t.Errorf("round-trip %v -> %s -> %v", k, b, got)
		}
	}
	var got SpanKind = SpanANR
	if err := json.Unmarshal([]byte(`"bogus"`), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != SpanUnknown {
		t.Errorf("unknown kind should decode to Unknown, got %v", got)
	}
}

func TestSpan_JSONRoundTrip(t *testing.T) {
	s := Span{
		ID:        101,
		Kind:      SpanANR,
		StartSeq:  42,
		EndSeq:    128,
		PID:       12345,
		Summary:   "ANR in com.example.app: Input dispatching timed out",
		SchemaVer: SchemaVersion,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Span
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != s {
		t.Errorf("round-trip Span mismatch\n got=%#v\nwant=%#v", got, s)
	}
}

func TestSpan_ZeroValueJSON(t *testing.T) {
	var s Span
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"id":0,"kind":"unknown","start_seq":0,"end_seq":0}`
	if string(b) != want {
		t.Errorf("zero-value Span JSON = %s, want %s", b, want)
	}
}
