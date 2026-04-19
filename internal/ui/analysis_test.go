package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

func TestClassifyIncoming_AndroidANRRecordsFinding(t *testing.T) {
	m := NewModel(buffer.NewRing(100), nil, false, false, "", "stdin", "test",
		nil, &LogTypeOpts{Kind: LogTypeADB, ProbeLines: 32}, nil)
	m.effectiveLogFmt = filter.FormatAndroid

	m.classifyIncoming(42,
		`04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app (com.example.app/.MainActivity)`)

	k, ok := m.FindingAt(42)
	if !ok {
		t.Fatal("expected a finding at seq 42")
	}
	if k != domain.FindingANR {
		t.Errorf("FindingAt(42) = %v, want FindingANR", k)
	}
	if m.FindingCount() != 1 {
		t.Errorf("FindingCount = %d, want 1", m.FindingCount())
	}
}

func TestClassifyIncoming_NonAnomalyLineLeavesFindingsEmpty(t *testing.T) {
	m := NewModel(buffer.NewRing(100), nil, false, false, "", "stdin", "test",
		nil, &LogTypeOpts{Kind: LogTypeADB, ProbeLines: 32}, nil)
	m.effectiveLogFmt = filter.FormatAndroid

	m.classifyIncoming(1, `04-19 14:23:40.012  1245  1301 I SystemServer: Entered the Android system server!`)

	if _, ok := m.FindingAt(1); ok {
		t.Error("normal info line should not produce a finding")
	}
	if m.FindingCount() != 0 {
		t.Errorf("FindingCount = %d, want 0", m.FindingCount())
	}
}

func TestClassifyIncoming_NonAndroidFormatIsNoop(t *testing.T) {
	m := NewModel(buffer.NewRing(100), nil, false, false, "", "stdin", "test",
		nil, &LogTypeOpts{Kind: LogTypePlain, ProbeLines: 32}, nil)
	m.effectiveLogFmt = filter.FormatPlain

	// Same ANR-shaped text, but format is Plain — classifier should stay idle.
	m.classifyIncoming(1, `04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app`)
	if m.FindingCount() != 0 {
		t.Errorf("FindingCount = %d, want 0 when format != Android", m.FindingCount())
	}
}
