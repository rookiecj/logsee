package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/filter"
)

func TestLogType_explicitPlainInStatus(t *testing.T) {
	// Given: plain log type
	r := buffer.NewRing(8)
	lt := &LogTypeOpts{Kind: LogTypePlain, ProbeLines: 32}
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, lt, nil)
	// When
	s := m.buildStatusLine()
	// Then
	if !strings.Contains(s, "type:plain") {
		t.Fatalf("Then: status should show type:plain, got %q", s)
	}
}

func TestLogType_autoResolvesADB(t *testing.T) {
	// Given: auto with probe 2 and two Android-shaped lines
	r := buffer.NewRing(100)
	lt := &LogTypeOpts{Kind: LogTypeAuto, ProbeLines: 2}
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, lt, nil)
	line := `2022-12-29 04:00:18.823 1-2 Tag com.example D msg`
	// When
	m.applyIncomingLines([]string{line, line})
	// Then
	if !m.logFormatResolved {
		t.Fatal("Then: auto format should resolve after probe lines")
	}
	if m.effectiveLogFmt != filter.FormatAndroid {
		t.Fatalf("Then: want FormatAndroid, got %v", m.effectiveLogFmt)
	}
	if got := m.buildStatusLine(); !strings.Contains(got, "type:adb") {
		t.Fatalf("Then: status should show type:adb, got %q", got)
	}
}

func TestLogType_autoPendingBeforeProbe(t *testing.T) {
	// Given: auto but fewer non-empty lines than probe
	r := buffer.NewRing(100)
	lt := &LogTypeOpts{Kind: LogTypeAuto, ProbeLines: 5}
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, lt, nil)
	m.applyIncomingLines([]string{"only one"})
	// Then
	if m.logFormatResolved {
		t.Fatal("Then: should not resolve yet")
	}
	if !strings.Contains(m.buildStatusLine(), "type:auto~") {
		t.Fatalf("Then: pending marker, got %q", m.buildStatusLine())
	}
}

func TestLogType_explicitADB(t *testing.T) {
	r := buffer.NewRing(8)
	lt := &LogTypeOpts{Kind: LogTypeADB, ProbeLines: 32}
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, lt, nil)
	if m.effectiveLogFmt != filter.FormatAndroid {
		t.Fatal()
	}
	if !strings.Contains(m.buildStatusLine(), "type:adb") {
		t.Fatal(m.buildStatusLine())
	}
}
