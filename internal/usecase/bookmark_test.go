package usecase

import "testing"

func TestBookmarkToggleAllocatesLowestAvailableSlot(t *testing.T) {
	bookmarks := NewBookmarkState()

	if slot, added := bookmarks.ToggleRawLine(42); slot != 1 || !added {
		t.Fatalf("first toggle slot = %d, added = %v; want slot 1 added", slot, added)
	}
	if slot, added := bookmarks.ToggleRawLine(7); slot != 2 || !added {
		t.Fatalf("second toggle slot = %d, added = %v; want slot 2 added", slot, added)
	}

	bookmarks.ToggleRawLine(42)

	if slot, added := bookmarks.ToggleRawLine(99); slot != 1 || !added {
		t.Fatalf("slot after freeing 1 = %d, added = %v; want slot 1 added", slot, added)
	}
	if got, ok := bookmarks.SlotForRawLine(7); !ok || got != 2 {
		t.Fatalf("remaining bookmark slot = %d, ok = %v; want slot 2 still assigned", got, ok)
	}
}

func TestBookmarkToggleRefusesTenthBookmarkWithoutChangingExistingSlots(t *testing.T) {
	bookmarks := NewBookmarkState()
	for rawLine := 1; rawLine <= MaxBookmarkSlots; rawLine++ {
		slot, added := bookmarks.ToggleRawLine(rawLine)
		if !added || slot != rawLine {
			t.Fatalf("toggle raw line %d slot = %d, added = %v; want matching slot added", rawLine, slot, added)
		}
	}

	if slot, added := bookmarks.ToggleRawLine(10); slot != 0 || added {
		t.Fatalf("tenth toggle slot = %d, added = %v; want refused", slot, added)
	}
	if got := bookmarks.Slots(); len(got) != MaxBookmarkSlots {
		t.Fatalf("bookmark count after refused toggle = %d, want %d", len(got), MaxBookmarkSlots)
	}
	for slot := 1; slot <= MaxBookmarkSlots; slot++ {
		rawLine, ok := bookmarks.RawLineForSlot(slot)
		if !ok || rawLine != slot {
			t.Fatalf("slot %d raw line = %d, ok = %v; want raw line %d preserved", slot, rawLine, ok, slot)
		}
	}
}

func TestBookmarkToggleRemovesSameLineWithoutRenumberingOtherBookmarks(t *testing.T) {
	bookmarks := NewBookmarkState()
	bookmarks.HandleLogListKey(BookmarkKeyToggle, 10)
	bookmarks.HandleLogListKey(BookmarkKeyToggle, 20)
	bookmarks.HandleLogListKey(BookmarkKeyToggle, 30)

	if slot, added := bookmarks.HandleLogListKey(BookmarkKeyToggle, 20); slot != 2 || added {
		t.Fatalf("remove toggle slot = %d, added = %v; want slot 2 removed", slot, added)
	}
	if _, ok := bookmarks.SlotForRawLine(20); ok {
		t.Fatal("removed raw line still has bookmark")
	}
	if got, ok := bookmarks.SlotForRawLine(10); !ok || got != 1 {
		t.Fatalf("raw line 10 slot = %d, ok = %v; want slot 1", got, ok)
	}
	if got, ok := bookmarks.SlotForRawLine(30); !ok || got != 3 {
		t.Fatalf("raw line 30 slot = %d, ok = %v; want slot 3", got, ok)
	}
}

func TestNavigationBookmarkJumpOnlyMovesToVisibleBookmarkedLine(t *testing.T) {
	bookmarks := NewBookmarkState()
	bookmarks.ToggleRawLine(100)
	bookmarks.ToggleRawLine(200)

	records := []OutputLogRecord{
		{RawLineNumber: 90, Text: "visible before"},
		{RawLineNumber: 100, Text: "visible bookmark"},
		{RawLineNumber: 110, Text: "visible after"},
	}
	state := mustNavigationState(t, NavigationOptions{
		OutputCount:       len(records),
		ViewportHeight:    2,
		CursorOutputIndex: 0,
		ScrollOffset:      0,
	})

	if moved := state.MoveToBookmark(records, bookmarks, 1); !moved {
		t.Fatal("MoveToBookmark did not move to visible bookmark")
	}
	assertNavigationState(t, state, 1, 0, false)

	if moved := state.MoveToBookmark(records, bookmarks, 2); moved {
		t.Fatal("MoveToBookmark moved to hidden bookmark")
	}
	assertNavigationState(t, state, 1, 0, false)

	if moved := state.MoveToBookmark(records, bookmarks, 9); moved {
		t.Fatal("MoveToBookmark moved to missing bookmark")
	}
	assertNavigationState(t, state, 1, 0, false)
}
