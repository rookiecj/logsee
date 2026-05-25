package usecase

import (
	"fmt"
	"testing"
)

func TestRecordHistoryEntry_DedupesAndCapsAtTen(t *testing.T) {
	// Given: nine prior entries and a repeat of the oldest value
	history := []string{
		"nine", "eight", "seven", "six", "five",
		"four", "three", "two", "one",
	}

	// When: recording "one" again
	got := RecordHistoryEntry(history, "one")

	// Then: "one" is newest and length stays at nine
	if len(got) != 9 {
		t.Fatalf("len = %d, want 9", len(got))
	}
	if got[0] != "one" {
		t.Fatalf("first = %q, want one", got[0])
	}
	if got[1] != "nine" {
		t.Fatalf("second = %q, want nine", got[1])
	}

	// When: adding ten distinct new values on top
	for i := 0; i < 10; i++ {
		got = RecordHistoryEntry(got, fmt.Sprintf("v%d", i))
	}

	// Then: only ten entries remain with newest first
	if len(got) != MaxInputHistoryEntries {
		t.Fatalf("len = %d, want %d", len(got), MaxInputHistoryEntries)
	}
	if got[0] != "v9" {
		t.Fatalf("newest = %q, want v9", got[0])
	}
}

func TestRecordHistoryEntry_IgnoresEmptyValue(t *testing.T) {
	// Given: existing history
	history := []string{"alpha"}

	// When: recording empty string
	got := RecordHistoryEntry(history, "")

	// Then: history is unchanged
	if len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("history = %#v, want [alpha]", got)
	}
}

func TestRecordChannelHistory_UpdatesLast(t *testing.T) {
	// Given: empty channel history
	channel := InputChannelHistory{History: []string{"old"}}

	// When: applying "new"
	got := RecordChannelHistory(channel, "new")

	// Then: last and history front match
	if got.Last != "new" {
		t.Fatalf("last = %q, want new", got.Last)
	}
	if len(got.History) != 2 || got.History[0] != "new" {
		t.Fatalf("history = %#v, want [new old]", got.History)
	}
}
