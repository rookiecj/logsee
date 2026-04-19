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

func TestBuildJournal_FullFields(t *testing.T) {
	line := domain.Line{
		Seq:  1,
		Text: `2024-04-19T14:22:10.001234+0900 build01 systemd[1]: Starting nginx.service - nginx - high performance web server...`,
	}
	got := BuildRecord(line, domain.LineFormatJournal)
	if got.Tag != "systemd" {
		t.Errorf("Tag = %q, want systemd", got.Tag)
	}
	if got.PID != 1 {
		t.Errorf("PID = %d, want 1", got.PID)
	}
	if !strings.HasPrefix(got.Message, "Starting nginx.service") {
		t.Errorf("Message unexpected: %q", got.Message)
	}
	if got.Format != domain.LineFormatJournal {
		t.Errorf("Format = %v, want LineFormatJournal", got.Format)
	}
	if got.Time.IsZero() {
		t.Error("Time should parse non-zero")
	}
	// 14:22:10 +0900 = 05:22:10 UTC
	wantUTC := time.Date(2024, 4, 19, 5, 22, 10, 1234000, time.UTC)
	if !got.Time.Equal(wantUTC) {
		t.Errorf("Time UTC = %v, want %v", got.Time.UTC(), wantUTC)
	}
}

func TestBuildJournal_NoPID(t *testing.T) {
	line := domain.Line{
		Seq:  2,
		Text: `2024-04-19T14:24:18.045123+0900 build01 kernel: audit: type=1400 apparmor="STATUS"`,
	}
	got := BuildRecord(line, domain.LineFormatJournal)
	if got.Tag != "kernel" {
		t.Errorf("Tag = %q, want kernel", got.Tag)
	}
	if got.PID != 0 {
		t.Errorf("PID = %d, want 0 (absent in source)", got.PID)
	}
	if !strings.HasPrefix(got.Message, "audit:") {
		t.Errorf("Message: %q", got.Message)
	}
}

func TestBuildJournal_ZuluTimezone(t *testing.T) {
	line := domain.Line{
		Seq:  3,
		Text: `2024-04-19T09:14:02.278901Z node01 kernel: BUG: kernel NULL pointer dereference`,
	}
	got := BuildRecord(line, domain.LineFormatJournal)
	if got.Time.IsZero() {
		t.Error("Zulu timezone should parse")
	}
	if got.Tag != "kernel" {
		t.Errorf("Tag = %q", got.Tag)
	}
}

func TestBuildJournal_NoFraction(t *testing.T) {
	line := domain.Line{
		Seq:  4,
		Text: `2024-04-19T14:22:10+0900 build01 systemd[1]: OK`,
	}
	got := BuildRecord(line, domain.LineFormatJournal)
	if got.Time.IsZero() {
		t.Error("no-fraction timestamp should parse")
	}
	if got.PID != 1 {
		t.Errorf("PID = %d, want 1", got.PID)
	}
}

func TestBuildJournal_MalformedReturnsMinimal(t *testing.T) {
	for _, text := range []string{
		"",
		"not a journal line",
		"04-19 14:24:10.456  1245  1301 I Tag: adb threadtime line",
	} {
		got := BuildRecord(domain.Line{Seq: 99, Text: text}, domain.LineFormatJournal)
		if got.Seq != 99 || got.Format != domain.LineFormatJournal || got.SchemaVer != domain.SchemaVersion {
			t.Errorf("text %q: minimal fields wrong: %#v", text, got)
		}
		if got.Tag != "" || got.PID != 0 || got.Message != "" {
			t.Errorf("text %q: malformed line should not populate parsed fields: %#v", text, got)
		}
	}
}

func TestBuildJournal_NonJournalFormatIsNoop(t *testing.T) {
	// Feeding a real journal line with a non-journal format hint must not
	// populate parsed fields — that is the Android builder's domain.
	line := domain.Line{
		Seq:  5,
		Text: `2024-04-19T14:22:10.001234+0900 build01 systemd[1]: OK`,
	}
	got := BuildRecord(line, domain.LineFormatPlain)
	if got.Tag != "" || got.PID != 0 {
		t.Errorf("Plain format should leave fields empty, got %#v", got)
	}
}

// Golden: first 50 lines of each journalctl sample must serialize to a
// fixed JSONL blob under testdata/golden. Regenerate with
// UPDATE_GOLDEN=1 after schema changes.

func TestGolden_JournalSystemdUnitFailed(t *testing.T) {
	checkJournalGolden(t, "systemd_unit_failed.log", "records_journal_systemd_unit_failed_first50.jsonl")
}

func TestGolden_JournalKernelPanic(t *testing.T) {
	checkJournalGolden(t, "kernel_panic.log", "records_journal_kernel_panic_first50.jsonl")
}

func TestGolden_JournalOOMKiller(t *testing.T) {
	checkJournalGolden(t, "oom_killer.log", "records_journal_oom_killer_first50.jsonl")
}

func TestGolden_JournalCoredump(t *testing.T) {
	checkJournalGolden(t, "coredump.log", "records_journal_coredump_first50.jsonl")
}

func TestGolden_JournalAuthFailures(t *testing.T) {
	checkJournalGolden(t, "auth_failures.log", "records_journal_auth_failures_first50.jsonl")
}

func checkJournalGolden(t *testing.T, sample, golden string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "journalctl", sample))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 50 {
		t.Fatalf("sample %s too short: %d lines", sample, len(lines))
	}
	lines = lines[:50]
	var buf strings.Builder
	for i, text := range lines {
		r := BuildRecord(domain.Line{Seq: int64(i + 1), Text: text}, domain.LineFormatJournal)
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
