package domain

import (
	"encoding/json"
	"testing"
)

func TestLineFormat_StringAndMarshalling(t *testing.T) {
	cases := []struct {
		f    LineFormat
		want string
	}{
		{LineFormatUnknown, "unknown"},
		{LineFormatPlain, "plain"},
		{LineFormatBracket, "bracket"},
		{LineFormatAndroid, "android"},
		{LineFormat(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.f.String(); got != tc.want {
			t.Errorf("LineFormat(%d).String() = %q, want %q", tc.f, got, tc.want)
		}
		b, err := json.Marshal(tc.f)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(b) != `"`+tc.want+`"` {
			t.Errorf("Marshal(%d) = %s, want %q", tc.f, b, tc.want)
		}
	}
}

func TestLineFormat_JSONRoundTrip(t *testing.T) {
	for _, f := range []LineFormat{LineFormatUnknown, LineFormatPlain, LineFormatBracket, LineFormatAndroid} {
		b, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var got LineFormat
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got != f {
			t.Errorf("round-trip %v -> %s -> %v", f, b, got)
		}
	}
}

func TestLineFormat_UnknownNameDecodesToUnknown(t *testing.T) {
	var f LineFormat = LineFormatAndroid
	if err := json.Unmarshal([]byte(`"logcat-v2"`), &f); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if f != LineFormatUnknown {
		t.Errorf("unknown name should decode to Unknown, got %v", f)
	}
}
