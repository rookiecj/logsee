package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.inpt.fr/42dottools/log/internal/domain"
)

func TestBuild_ThreadtimeFullFields(t *testing.T) {
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	line := domain.Line{
		Seq:  42,
		Text: `04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app (com.example.app/.MainActivity)`,
	}
	got := b.Build(line, domain.LineFormatAndroid)
	want := domain.Record{
		Seq:       42,
		Time:      time.Date(2024, 4, 19, 14, 24, 10, 456_000_000, time.UTC),
		Level:     domain.LevelError,
		PID:       1245,
		TID:       1301,
		Tag:       "ActivityManager",
		Message:   "ANR in com.example.app (com.example.app/.MainActivity)",
		Format:    domain.LineFormatAndroid,
		SchemaVer: domain.SchemaVersion,
	}
	if !got.Time.Equal(want.Time) {
		t.Errorf("Time: got %v want %v", got.Time, want.Time)
	}
	got.Time = want.Time
	if got != want {
		t.Errorf("Build mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestBuild_AllLevels(t *testing.T) {
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	cases := []struct {
		letter string
		want   domain.Level
	}{
		{"V", domain.LevelVerbose},
		{"D", domain.LevelDebug},
		{"I", domain.LevelInfo},
		{"W", domain.LevelWarn},
		{"E", domain.LevelError},
		{"F", domain.LevelFatal},
		{"T", domain.LevelVerbose}, // adb occasionally emits T for trace
		{"S", domain.LevelUnknown}, // silent — not a real level
	}
	for _, tc := range cases {
		text := "04-19 14:24:10.456  1245  1301 " + tc.letter + " MyTag: hello"
		got := b.Build(domain.Line{Seq: 1, Text: text}, domain.LineFormatAndroid)
		if got.Level != tc.want {
			t.Errorf("letter %q -> Level %v, want %v (text=%q)", tc.letter, got.Level, tc.want, text)
		}
	}
}

func TestBuild_NonAndroidFormatReturnsMinimalRecord(t *testing.T) {
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	line := domain.Line{Seq: 7, Text: `04-19 14:24:10.456  1245  1301 E ActivityManager: ANR`}
	for _, f := range []domain.LineFormat{
		domain.LineFormatUnknown,
		domain.LineFormatPlain,
		domain.LineFormatBracket,
	} {
		got := b.Build(line, f)
		if got.Seq != 7 || got.Format != f || got.SchemaVer != domain.SchemaVersion {
			t.Errorf("format %v: minimal fields wrong: %#v", f, got)
		}
		if got.Level != domain.LevelUnknown || got.Tag != "" || got.Message != "" || got.PID != 0 {
			t.Errorf("format %v: should skip parsing but got populated fields: %#v", f, got)
		}
	}
}

func TestBuild_MalformedAndroidLineReturnsMinimalRecord(t *testing.T) {
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	lines := []string{
		"",
		"garbage",
		"2024-04-19 14:24:10 yearly prefix not threadtime",
		"04-19 NO-TIME  1245  1301 E Tag: hi",
	}
	for _, text := range lines {
		got := b.Build(domain.Line{Seq: 99, Text: text}, domain.LineFormatAndroid)
		if got.Seq != 99 || got.Format != domain.LineFormatAndroid || got.SchemaVer != domain.SchemaVersion {
			t.Errorf("text %q: core fields wrong: %#v", text, got)
		}
		if got.Level != domain.LevelUnknown || got.Tag != "" || got.PID != 0 || got.TID != 0 {
			t.Errorf("text %q: malformed line should not populate parsed fields: %#v", text, got)
		}
	}
}

func TestBuild_TagWithTrailingSpaces(t *testing.T) {
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	line := domain.Line{Seq: 1, Text: `04-19 15:10:22.100  4567  4567 F DEBUG   : *** *** *** *** *** *** *** *** *** *** *** *** *** *** *** ***`}
	got := b.Build(line, domain.LineFormatAndroid)
	if got.Tag != "DEBUG" {
		t.Errorf("Tag should trim trailing spaces, got %q", got.Tag)
	}
	if got.Level != domain.LevelFatal {
		t.Errorf("Level: got %v want Fatal", got.Level)
	}
	if !strings.HasPrefix(got.Message, "*** ") {
		t.Errorf("Message should start with ***, got %q", got.Message)
	}
}

func TestBuild_EmptyMessage(t *testing.T) {
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	line := domain.Line{Seq: 1, Text: `04-19 15:10:22.100  4567  4567 I SomeTag:`}
	got := b.Build(line, domain.LineFormatAndroid)
	if got.Tag != "SomeTag" {
		t.Errorf("Tag: got %q want SomeTag", got.Tag)
	}
	if got.Message != "" {
		t.Errorf("Message should be empty, got %q", got.Message)
	}
}

func TestBuild_BadPIDHandledGracefully(t *testing.T) {
	// Regex restricts PID to digits, so we can only stress the parser via
	// a crafted Line.Text that matches structurally but has extreme numbers.
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	line := domain.Line{Seq: 1, Text: `04-19 14:24:10.456  9999999999  1301 E Tag: hi`}
	got := b.Build(line, domain.LineFormatAndroid)
	// ParseInt 32-bit overflow → 0 via our helper; just assert no panic and
	// other fields still populate.
	if got.PID != 0 {
		t.Errorf("oversized PID should clamp to 0, got %d", got.PID)
	}
	if got.Tag != "Tag" {
		t.Errorf("Tag should still parse despite bad PID, got %q", got.Tag)
	}
}

func TestDefaultRecordBuilder_UsesCurrentYearAndLocal(t *testing.T) {
	b := DefaultRecordBuilder()
	if b.RefYear == 0 {
		t.Error("DefaultRecordBuilder should set RefYear")
	}
	if b.Location == nil {
		t.Error("DefaultRecordBuilder should set Location")
	}
}

func TestBuildRecord_ConvenienceMatchesDefaultBuilder(t *testing.T) {
	line := domain.Line{Seq: 1, Text: `04-19 14:24:10.456  1245  1301 I Tag: hi`}
	a := BuildRecord(line, domain.LineFormatAndroid)
	b := DefaultRecordBuilder().Build(line, domain.LineFormatAndroid)
	// Times may disagree by nanoseconds if clock ticks between calls, but
	// both should parse the same string, so equality is fine.
	if !a.Time.Equal(b.Time) || a.Tag != b.Tag || a.PID != b.PID {
		t.Errorf("convenience != builder: %#v vs %#v", a, b)
	}
}

// Golden tests: first 50 lines of each adb sample must serialize to a fixed
// JSONL blob under testdata/golden. Regenerate with UPDATE_GOLDEN=1 after
// schema changes.

func TestGolden_ANRInputDispatch(t *testing.T) {
	checkGolden(t, "anr_input_dispatch.log", "records_anr_input_dispatch_first50.jsonl")
}

func TestGolden_NativeTombstone(t *testing.T) {
	checkGolden(t, "native_tombstone.log", "records_native_tombstone_first50.jsonl")
}

func TestGolden_JavaFatalSystemServer(t *testing.T) {
	checkGolden(t, "java_fatal_system_server.log", "records_java_fatal_system_server_first50.jsonl")
}

func checkGolden(t *testing.T, sample, golden string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "android", sample))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 50 {
		t.Fatalf("sample %s too short: %d lines", sample, len(lines))
	}
	lines = lines[:50]
	b := RecordBuilder{RefYear: 2024, Location: time.UTC}
	var buf strings.Builder
	for i, text := range lines {
		r := b.Build(domain.Line{Seq: int64(i + 1), Text: text}, domain.LineFormatAndroid)
		enc, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal line %d: %v", i+1, err)
		}
		buf.Write(enc)
		buf.WriteByte('\n')
	}
	goldenPath := filepath.Join("..", "..", "testdata", "golden", golden)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(buf.String()), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (regenerate with UPDATE_GOLDEN=1): %v", err)
	}
	if buf.String() != string(want) {
		t.Errorf("%s golden mismatch — first 500 bytes:\n got=%q\nwant=%q",
			sample, safeHead(buf.String(), 500), safeHead(string(want), 500))
	}
}

func safeHead(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
