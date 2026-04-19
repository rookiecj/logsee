package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecord_JSONRoundTrip(t *testing.T) {
	ts := time.Date(2024, 4, 19, 14, 24, 10, 456_000_000, time.UTC)
	r := Record{
		Seq:       42,
		Time:      ts,
		Level:     LevelError,
		PID:       1245,
		TID:       1301,
		Tag:       "ActivityManager",
		Component: "com.example.app",
		Message:   "ANR in com.example.app",
		Format:    LineFormatAndroid,
		SchemaVer: SchemaVersion,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Record
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.Time.Equal(r.Time) {
		t.Errorf("Time mismatch: got %v want %v", got.Time, r.Time)
	}
	got.Time = r.Time
	if got != r {
		t.Errorf("round-trip mismatch\n got=%#v\nwant=%#v", got, r)
	}
}

func TestRecord_ZeroValueJSON(t *testing.T) {
	var r Record
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Zero value keeps the required seq/level keys and the zero time
	// (Go 1.22 json has no omitzero for time.Time, so the epoch-like
	// string is acceptable — parsers round-trip it fine).
	want := `{"seq":0,"time":"0001-01-01T00:00:00Z","level":"?"}`
	if string(b) != want {
		t.Errorf("zero-value Record JSON = %s, want %s", b, want)
	}
	var got Record
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != r {
		t.Errorf("zero-value round-trip mismatch: got %#v want %#v", got, r)
	}
}

func TestRecord_AcceptsFutureUnknownFields(t *testing.T) {
	payload := []byte(`{"seq":7,"level":"W","tag":"Foo","msg":"hi","future_field":"ignored"}`)
	var r Record
	if err := json.Unmarshal(payload, &r); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if r.Seq != 7 || r.Level != LevelWarn || r.Tag != "Foo" || r.Message != "hi" {
		t.Errorf("decoded Record unexpected: %#v", r)
	}
}
