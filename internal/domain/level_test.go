package domain

import (
	"encoding/json"
	"testing"
)

func TestLevel_String(t *testing.T) {
	tests := []struct {
		lv   Level
		want string
	}{
		{LevelUnknown, "?"},
		{LevelVerbose, "V"},
		{LevelDebug, "D"},
		{LevelInfo, "I"},
		{LevelWarn, "W"},
		{LevelError, "E"},
		{LevelFatal, "F"},
		{Level(99), "?"},
	}
	for _, tc := range tests {
		if got := tc.lv.String(); got != tc.want {
			t.Errorf("Level(%d).String() = %q, want %q", tc.lv, got, tc.want)
		}
	}
}

func TestLevelFromRaw(t *testing.T) {
	tests := []struct {
		in   string
		want Level
	}{
		{"", LevelUnknown},
		{"  ", LevelUnknown},
		{"V", LevelVerbose},
		{"v", LevelVerbose},
		{"verbose", LevelVerbose},
		{"VERBOSE", LevelVerbose},
		{"TRACE", LevelVerbose},
		{"T", LevelVerbose},
		{"D", LevelDebug},
		{"debug", LevelDebug},
		{"I", LevelInfo},
		{"Info", LevelInfo},
		{"W", LevelWarn},
		{"warning", LevelWarn},
		{"WARN", LevelWarn},
		{"E", LevelError},
		{"ERR", LevelError},
		{"error", LevelError},
		{"F", LevelFatal},
		{"FATAL", LevelFatal},
		{"nope", LevelUnknown},
		{"XYZ", LevelUnknown},
		{"  w  ", LevelWarn},
	}
	for _, tc := range tests {
		if got := LevelFromRaw(tc.in); got != tc.want {
			t.Errorf("LevelFromRaw(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestLevel_JSONRoundTrip(t *testing.T) {
	for _, lv := range []Level{LevelUnknown, LevelVerbose, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal} {
		b, err := json.Marshal(lv)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", lv, err)
		}
		var got Level
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", b, err)
		}
		if got != lv {
			t.Errorf("round-trip Level %v -> %s -> %v", lv, b, got)
		}
	}
}

func TestLevel_JSONUnknownDecodesToUnknown(t *testing.T) {
	var lv Level
	if err := json.Unmarshal([]byte(`"X"`), &lv); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if lv != LevelUnknown {
		t.Errorf("unknown text should decode to LevelUnknown, got %v", lv)
	}
}
