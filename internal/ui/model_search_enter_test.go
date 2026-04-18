package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"github.com/charmbracelet/bubbletea"
)

func TestModel_enterCommitsSearchDraftToSearchBuf(t *testing.T) {
	// Given: list focus, search compose opened, draft text, no prior committed query
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	if !m.searchCompose {
		t.Fatal("expected search compose after /")
	}
	m.searchDraft = "needle"
	// When: second Enter commits
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	// Then:
	if m.searchBuf != "needle" {
		t.Fatalf("searchBuf want needle, got %q", m.searchBuf)
	}
	if m.searchDraft != "needle" {
		t.Fatalf("searchDraft want synced needle, got %q", m.searchDraft)
	}
	if m.searchCompose {
		t.Fatal("expected compose off after commit")
	}
}

func TestModel_enterTrimsSearchDraft(t *testing.T) {
	// Given: compose with surrounding spaces in draft
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "  x  "
	// When: commit Enter
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	// Then:
	if m.searchBuf != "x" || m.searchDraft != "x" {
		t.Fatalf("want committed x, got buf=%q draft=%q", m.searchBuf, m.searchDraft)
	}
}

func TestModel_basicNav_esc_clearsSelectionBeforeComposePop(t *testing.T) {
	// Given: lines, range selection, then search compose with edited draft
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"a", "b", "c"})
	m.cursorIdx = 0
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyShiftDown}))
	if m.selAnchor < 0 {
		t.Fatal("expected range selection")
	}
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	if !m.searchCompose {
		t.Fatal("expected compose after /")
	}
	m.searchDraft = "edit"
	// When: first Esc — must clear selection first, keep compose
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	// Then:
	if m.selAnchor >= 0 {
		t.Fatal("expected selection cleared before compose pop")
	}
	if !m.searchCompose || m.searchDraft != "edit" {
		t.Fatalf("compose should remain with draft; compose=%v draft=%q", m.searchCompose, m.searchDraft)
	}
	// When: second Esc — compose pop
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	if m.searchCompose {
		t.Fatal("expected compose off")
	}
	if m.searchDraft != "" || m.searchBuf != "" {
		t.Fatalf("want empty committed search, buf=%q draft=%q", m.searchBuf, m.searchDraft)
	}
}

func TestModel_composeEscRevertsDraftToSearchBuf(t *testing.T) {
	// Given: committed search, compose opened, draft edited away from committed
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.searchBuf = "stable"
	m.searchDraft = "stable"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "typing"
	// When: Esc cancels compose
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEsc}))
	// Then:
	if m.searchCompose {
		t.Fatal("compose should be off")
	}
	if m.searchBuf != "stable" {
		t.Fatalf("searchBuf unchanged, got %q", m.searchBuf)
	}
	if m.searchDraft != "stable" {
		t.Fatalf("draft reverted to searchBuf, got %q", m.searchDraft)
	}
}

func TestModel_afterCommit_ctrlN_movesToNextHit_notSearchDraftAppend(t *testing.T) {
	// Given: lines with two "error" matches, committed search "error", cursor on first
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"error", "ok", "error2"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "error"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 0
	prevDraft := m.searchDraft
	// When: Ctrl+n as next-match
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyCtrlN}))
	// Then:
	if m.cursorIdx != 2 {
		t.Fatalf("want cursor fidx 2 (error2), got %d", m.cursorIdx)
	}
	if m.searchDraft != prevDraft {
		t.Fatalf("searchDraft should not change from Ctrl+n navigation, got %q want %q", m.searchDraft, prevDraft)
	}
}

func TestModel_afterCommit_plainN_doesNotAppendUnlessComposing(t *testing.T) {
	// Given: committed highlight — plain n does not touch draft without compose
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"error", "ok"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "error"
	m.searchBuf = ""
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m.cursorIdx = 0
	// When: plain n without opening compose
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'n'}}))
	// Then:
	if m.searchDraft != "error" {
		t.Fatalf("want draft unchanged error, got %q", m.searchDraft)
	}
	if m.cursorIdx != 0 {
		t.Fatalf("cursor should not move on plain n, got %d", m.cursorIdx)
	}
}

func TestModel_plainN_appendsWhenComposing(t *testing.T) {
	// Given: compose open after commit
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"error", "ok"})
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "error"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	// When: n while composing
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'n'}}))
	// Then:
	if m.searchDraft != "errorn" {
		t.Fatalf("want draft errorn, got %q", m.searchDraft)
	}
}

func TestModel_composeSlashExitsComposeAndOpensFilter(t *testing.T) {
	// Given: search compose with draft different from committed (simulated in-progress edit)
	r := buffer.NewRing(10)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.searchBuf = "a"
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	m.searchDraft = "bbb"
	// When: / (filter takes priority over compose)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	// Then:
	if m.searchCompose || m.searchDraft != "a" {
		t.Fatalf("compose off and draft reverted, got compose=%v draft=%q", m.searchCompose, m.searchDraft)
	}
	if !m.filterEdit {
		t.Fatal("expected filter edit mode")
	}
}

func TestModel_listNotComposing_keyN_doesNotMutateDraft(t *testing.T) {
	// Given: list focus, no compose, searchBuf empty
	r := buffer.NewRing(20)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.width, m.height = 80, 24
	m.applyIncomingLines([]string{"error", "ok", "error2"})
	m.searchDraft = ""
	m.searchBuf = ""
	m.cursorIdx = 0
	// When: n (must not append to draft)
	m = step(m, tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'n'}}))
	// Then:
	if m.searchDraft != "" {
		t.Fatalf("want draft empty, got %q", m.searchDraft)
	}
	if m.cursorIdx != 0 {
		t.Fatalf("cursor should stay 0, got %d", m.cursorIdx)
	}
}
