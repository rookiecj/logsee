package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
	tea "github.com/charmbracelet/bubbletea"
)

func newAnomalyModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel(buffer.NewRing(100), nil, false, false, "", "stdin", "test",
		nil, &LogTypeOpts{Kind: LogTypeADB, ProbeLines: 32}, nil)
	m.effectiveLogFmt = filter.FormatAndroid
	m.width = 80
	m.height = 20
	return m
}

func pushLines(m *Model, lines ...string) {
	for _, text := range lines {
		seq := m.buf.Push(text).Seq
		m.classifyIncoming(seq, text)
	}
}

func TestAnomalyToggle_AFiltersToFindingsOnly(t *testing.T) {
	m := newAnomalyModel(t)
	pushLines(m,
		`04-19 14:23:40.012  1245  1301 I SystemServer: Entered the Android system server!`,
		`04-19 14:23:45.123  1245  1301 I ActivityManager: normal traffic`,
		`04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app`,
		`04-19 14:24:11.100  4567  4567 F DEBUG   : *** *** *** *** *** *** *** *** *** ***`,
		`04-19 14:24:12.200  1245  1301 I ActivityManager: after-anomaly noise`,
	)

	// Default view: all 5 rows included.
	if got := len(m.filteredIndices()); got != 5 {
		t.Errorf("pre-toggle fidx len = %d, want 5", got)
	}

	// Simulate pressing "A".
	_, _ = m.tryBrowseKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")}, m.filteredIndices(), 10)
	if !m.anomalyOnly {
		t.Fatal("A key did not toggle anomalyOnly on")
	}
	got := m.filteredIndices()
	if len(got) != 2 {
		t.Errorf("anomalyOnly fidx len = %d, want 2 (ANR + native header)", len(got))
	}

	// Press A again to turn off.
	_, _ = m.tryBrowseKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")}, got, 10)
	if m.anomalyOnly {
		t.Fatal("second A press should have turned anomalyOnly off")
	}
	if got := len(m.filteredIndices()); got != 5 {
		t.Errorf("post-toggle-off fidx len = %d, want 5", got)
	}
}

func TestAnomalyToggle_DropsWhenFindingsEmpty(t *testing.T) {
	m := newAnomalyModel(t)
	pushLines(m,
		`04-19 14:23:40.012  1245  1301 I SystemServer: boring`,
		`04-19 14:23:45.123  1245  1301 I ActivityManager: also boring`,
	)
	m.anomalyOnly = true
	if got := m.filteredIndices(); len(got) != 0 {
		t.Errorf("anomalyOnly with no findings should yield 0 rows, got %d", len(got))
	}
}

func TestAnomalyTag_ComposesWithPlainFilter(t *testing.T) {
	m := newAnomalyModel(t)
	pushLines(m,
		`04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app`,
		`04-19 14:24:11.100  4567  4567 F DEBUG   : *** *** *** *** ***`,
	)

	prog, err := filter.Parse("anomaly:anr")
	if err != nil {
		t.Fatal(err)
	}
	m.prog = prog
	fidx := m.filteredIndices()
	if len(fidx) != 1 {
		t.Fatalf("anomaly:anr should match exactly 1 line, got %d", len(fidx))
	}
	if m.buf.At(fidx[0]).Seq != 1 {
		t.Errorf("matched wrong line: Seq = %d, want 1", m.buf.At(fidx[0]).Seq)
	}
}

func TestAnomalyAnyViaDSL(t *testing.T) {
	m := newAnomalyModel(t)
	pushLines(m,
		`04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app`,
		`04-19 14:23:45.123  1245  1301 I SystemServer: boring`,
		`04-19 14:24:11.100  4567  4567 F DEBUG   : *** *** *** *** ***`,
	)

	prog, err := filter.Parse("anomaly:any")
	if err != nil {
		t.Fatal(err)
	}
	m.prog = prog
	if got := len(m.filteredIndices()); got != 2 {
		t.Errorf("anomaly:any fidx len = %d, want 2", got)
	}
	_ = domain.FindingANR // silence unused import if refactored
}
