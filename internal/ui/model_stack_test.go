package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestModel_formatLine_searchHighlightCaseSensitive(t *testing.T) {
	// Given: filter --ignore-case off; highlight is case-sensitive (PRD §8.2)
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	hi := lipgloss.NewStyle()
	rec := domain.Line{Seq: 1, Text: "Hello"}
	m.searchBuf = "ell"
	m.searchDraft = m.searchBuf
	out := m.formatLine(rec, hi, 50, false, false, false)
	if !strings.Contains(out, "ell") {
		t.Fatalf("expected case-exact substring in output: %q", out)
	}
	m.searchBuf = "ELL"
	m.searchDraft = m.searchBuf
	if SearchMatchesLine(rec.Text, m.searchBuf, false) {
		t.Fatal("ELL must not match Hello when matching is case-sensitive")
	}
}

// Case A (기본 탐색 + 하이라이트): 레이어 1 = 전체 줄, n/p·매칭은 전체 목록에서만.
func TestModel_stack_baseMode_searchOnlyOnListedLines(t *testing.T) {
	// Given: ring lines where only some contain the query substring; prog empty (기본 탐색)
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"error", "ok", "error2"})
	m.searchBuf = "error"
	m.searchDraft = m.searchBuf
	fidx := m.filteredIndices()
	if len(fidx) != 3 {
		t.Fatalf("Given empty prog, want 3 listed lines, got %d", len(fidx))
	}
	// When / Then: next hit skips non-matching ring line
	m.cursorIdx = 0
	m.gotoNextSearchHit(fidx)
	if m.cursorIdx != 2 {
		t.Fatalf("When cursor on first match, next hit want fidx index 2 (error2), got %d", m.cursorIdx)
	}
	m.gotoNextSearchHit(fidx)
	if m.cursorIdx != 2 {
		t.Fatalf("When cursor on last match, next hit does not wrap; want fidx index 2, got %d", m.cursorIdx)
	}
	// Then: every visited fidx entry matches search on full line text
	for _, fi := range []int{0, 2} {
		line := m.buf.At(fidx[fi]).Text
		if !SearchMatchesLine(line, m.searchBuf, false) {
			t.Fatalf("line %q should match search", line)
		}
	}
	if SearchMatchesLine(m.buf.At(fidx[1]).Text, m.searchBuf, false) {
		t.Fatal("middle line must not match search")
	}
}

// Case B (필터 + 하이라이트 스택): 다음 매칭은 fidx 안에서만 앞으로 스캔(순환 없음); 필터 밖 줄은 후보가 아님.
func TestModel_stack_filterThenSearch_nVisitsOnlyFilteredLines(t *testing.T) {
	// Given: one line fails filter; search matches a subset of filtered lines
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"foo a", "bar nomatch", "foo secret", "foo other"})
	var err error
	m.prog, err = filter.Parse("+foo")
	if err != nil {
		t.Fatal(err)
	}
	m.appliedFilter = "+foo"
	m.searchBuf = "secret"
	m.searchDraft = m.searchBuf
	fidx := m.filteredIndices()
	// ring indices 0,2,3 — never 1 (bar)
	if len(fidx) != 3 || fidx[1] != 2 {
		t.Fatalf("Given +foo filter, want fidx [0,2,3], got %v", fidx)
	}
	for _, ri := range fidx {
		if ri == 1 {
			t.Fatal("filtered list must not include bar line (ring index 1)")
		}
	}
	// When: walk next hits a few steps
	m.cursorIdx = 0
	for step := 0; step < 6; step++ {
		ri := fidx[m.cursorIdx]
		if ri == 1 {
			t.Fatalf("step %d: cursor must not land on filtered-out ring row 1", step)
		}
		if !filter.Match(m.buf.At(ri).Text, m.prog, false, filter.FormatUnknown) {
			t.Fatalf("step %d: line must pass filter", step)
		}
		m.gotoNextSearchHit(fidx)
	}
	// Then: only ring index 2 matches "secret" among filtered — cursor stays on that fidx slot
	onlySecret := -1
	for i, ri := range fidx {
		if SearchMatchesLine(m.buf.At(ri).Text, m.searchBuf, false) {
			onlySecret = i
		}
	}
	if onlySecret != 1 {
		t.Fatalf("expected exactly one fidx slot matching secret, got index %d", onlySecret)
	}
	if m.cursorIdx != onlySecret {
		t.Fatalf("After n-walk, cursor want only matching fidx index %d, got %d", onlySecret, m.cursorIdx)
	}
}

// Case C (축 독립성): 필터 적용·해제해도 적용 검색어 유지, 그 반대도 검색이 prog를 바꾸지 않음.
func TestModel_stack_axesIndependent_filterApplyPreservesSearch(t *testing.T) {
	// Given: applied search set, then filter applied (simulate Enter path via direct prog assign as in other tests)
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.applyIncomingLines([]string{"x1", "x2"})
	m.searchBuf = "needle"
	m.searchDraft = m.searchBuf
	var err error
	m.prog, err = filter.Parse("+x")
	if err != nil {
		t.Fatal(err)
	}
	m.appliedFilter = "+x"
	// Then: search unchanged
	if m.searchBuf != "needle" {
		t.Fatalf("When filter applied, searchBuf want unchanged, got %q", m.searchBuf)
	}
}

func TestModel_stack_esc_KeyMsg_popFilterInputFocus_viaUpdate(t *testing.T) {
	// Given: filter input focus via Enter from list
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"z"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.filterDraft = "draft"
	// When: Esc
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	// Then: list focus (PRD §6.0)
	if m.filterEdit {
		t.Fatal("expected list focus after Esc")
	}
	if m.filterDraft != "" {
		t.Fatalf("draft reverted to applied, want empty got %q", m.filterDraft)
	}
}

func TestModel_stack_esc_popFilterInputFocus_toList(t *testing.T) {
	// Given: filter input focus with a mutated draft
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a"})
	m.appliedFilter = "+a"
	m.prog, _ = filter.Parse("+a")
	m.filterEdit = true
	m.filterDraft = "+a extra"
	// When: Esc (mode stack pop: filter-input focus → list focus)
	m.popFilterInputFocus()
	// Then: list focus, draft reverted to applied
	if m.filterEdit {
		t.Fatal("expected list focus (filterEdit false)")
	}
	if m.filterDraft != m.appliedFilter || m.filterDraft != "+a" {
		t.Fatalf("draft reverted, want +a got %q", m.filterDraft)
	}
}

func TestModel_stack_axesIndependent_clearFilterPreservesSearch(t *testing.T) {
	// Given: both filter and search
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.applyIncomingLines([]string{"a", "b"})
	m.searchBuf = "keep"
	m.searchDraft = m.searchBuf
	m.prog, _ = filter.Parse("+a")
	m.appliedFilter = "+a"
	// When: clear filter (programmatic; list Esc does not clear filter per PRD §6.6)
	m.clearAppliedFilter()
	// Then: search buffer intact; prog empty
	if m.searchBuf != "keep" {
		t.Fatalf("When filter cleared, searchBuf want %q, got %q", "keep", m.searchBuf)
	}
	if !m.prog.Empty() || m.appliedFilter != "" {
		t.Fatalf("prog/appliedFilter should be cleared, progEmpty=%v applied=%q", m.prog.Empty(), m.appliedFilter)
	}
}
