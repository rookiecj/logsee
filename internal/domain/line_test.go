package domain

import (
	"encoding/json"
	"testing"
)

func TestLine_JSONRoundTrip(t *testing.T) {
	l := Line{Seq: 12345, Text: "04-19 14:24:10 E ActivityManager: ANR"}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Line
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != l {
		t.Errorf("round-trip mismatch\n got=%#v\nwant=%#v", got, l)
	}
}

func TestSchemaVersion_NonZero(t *testing.T) {
	if SchemaVersion == 0 {
		t.Fatal("SchemaVersion must be > 0 so readers can tell a field was set")
	}
}
