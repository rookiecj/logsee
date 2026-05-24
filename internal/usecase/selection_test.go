package usecase

import (
	"context"
	"reflect"
	"testing"
)

func TestRangeSelectionShiftMovementCreatesAndUpdatesRawLineRange(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 10, Text: "first"},
		{RawLineNumber: 20, Text: "second"},
		{RawLineNumber: 30, Text: "third"},
		{RawLineNumber: 40, Text: "fourth"},
	}
	nav := mustNavigationState(t, NavigationOptions{
		OutputCount:       len(records),
		ViewportHeight:    3,
		CursorOutputIndex: 1,
	})
	selection := NewSelectionState()

	nav.MoveWithSelection(NavigationMoveDown, records, selection, true)

	got, ok := selection.Range()
	if !ok {
		t.Fatal("range selection missing after Shift+Down")
	}
	if want := (RawLineRange{Start: 20, End: 30}); got != want {
		t.Fatalf("range after first Shift+Down = %#v, want %#v", got, want)
	}

	nav.MoveWithSelection(NavigationMoveDown, records, selection, true)

	got, ok = selection.Range()
	if !ok {
		t.Fatal("range selection missing after second Shift+Down")
	}
	if want := (RawLineRange{Start: 20, End: 40}); got != want {
		t.Fatalf("range after second Shift+Down = %#v, want %#v", got, want)
	}
}

func TestShiftSelectionPersistsAfterNonShiftNavigation(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 10, Text: "first"},
		{RawLineNumber: 20, Text: "second"},
		{RawLineNumber: 30, Text: "third"},
		{RawLineNumber: 40, Text: "fourth"},
	}
	nav := mustNavigationState(t, NavigationOptions{
		OutputCount:       len(records),
		ViewportHeight:    3,
		CursorOutputIndex: 1,
	})
	selection := NewSelectionState()

	nav.MoveWithSelection(NavigationMoveDown, records, selection, true)
	if got, want := selection.PickedLines(), []int{20, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picked lines after Shift+Down = %#v, want %#v", got, want)
	}

	nav.MoveWithSelection(NavigationMoveDown, records, selection, false)
	if _, ok := selection.Range(); ok {
		t.Fatal("non-Shift movement should clear only active range display")
	}
	if got, want := selection.PickedLines(), []int{20, 30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picked lines after non-Shift movement = %#v, want persisted Shift selection %#v", got, want)
	}

	got := BuildCopyText(records, selection, 40)
	if got.Text != "second\nthird" {
		t.Fatalf("copy text after persisted Shift selection = %q, want selected Shift lines", got.Text)
	}
}

func TestSpacePickTogglesCanCoexistWithRangeSelection(t *testing.T) {
	selection := NewSelectionState()
	selection.StartOrUpdateRange(20, 40)

	added := selection.TogglePicked(10)
	if !added {
		t.Fatal("first Space pick removed line, want add")
	}
	selection.TogglePicked(30)

	if got, want := selection.PickedCount(), 2; got != want {
		t.Fatalf("picked count = %d, want %d", got, want)
	}
	if !selection.IsRangeSelected(30) {
		t.Fatal("range selection missing for raw line 30")
	}
	if !selection.IsPicked(30) {
		t.Fatal("picked selection missing for raw line 30")
	}

	removed := selection.TogglePicked(10)
	if removed {
		t.Fatal("second Space pick added line, want remove")
	}
	if got, want := selection.PickedLines(), []int{30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picked raw lines = %#v, want %#v", got, want)
	}
}

func TestSpaceWithActiveRangeTogglesCurrentLineAndKeepsOtherRangeLines(t *testing.T) {
	selection := NewSelectionState()
	selection.StartOrUpdateRange(40, 20)
	selection.TogglePicked(10)

	selection.PickCursorOrRange(40)

	if _, ok := selection.Range(); ok {
		t.Fatal("range still active after Space toggles selected line")
	}
	if got, want := selection.PickedLines(), []int{10, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picked raw lines = %#v, want range persisted except reselected cursor %#v", got, want)
	}
}

func TestSpaceWithoutActiveRangeTogglesOnlyCursorLine(t *testing.T) {
	selection := NewSelectionState()

	selection.PickCursorOrRange(30)

	if got, want := selection.PickedLines(), []int{30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picked lines after first Space = %#v, want %#v", got, want)
	}

	selection.PickCursorOrRange(30)

	if got := selection.PickedLines(); len(got) != 0 {
		t.Fatalf("picked lines after second Space = %#v, want none", got)
	}
}

func TestNonShiftNavigationClearsRangeAndPreservesPickedLines(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 10, Text: "first"},
		{RawLineNumber: 20, Text: "second"},
		{RawLineNumber: 30, Text: "third"},
		{RawLineNumber: 40, Text: "fourth"},
	}
	for _, move := range []NavigationMove{
		NavigationMoveDown,
		NavigationMoveUp,
		NavigationMovePageDown,
		NavigationMovePageUp,
		NavigationMoveHome,
		NavigationMoveEnd,
	} {
		t.Run(move.String(), func(t *testing.T) {
			nav := mustNavigationState(t, NavigationOptions{
				OutputCount:       len(records),
				ViewportHeight:    2,
				CursorOutputIndex: 1,
			})
			selection := NewSelectionState()
			selection.StartOrUpdateRange(10, 30)
			selection.TogglePicked(40)

			nav.MoveWithSelection(move, records, selection, false)

			if _, ok := selection.Range(); ok {
				t.Fatalf("%s left range active after non-Shift movement", move)
			}
			if got, want := selection.PickedLines(), []int{40}; !reflect.DeepEqual(got, want) {
				t.Fatalf("%s picked lines = %#v, want %#v", move, got, want)
			}
		})
	}
}

func TestSearchMatchNavigationClearsRangeAndPreservesPickedLines(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 10, Text: "first"},
		{RawLineNumber: 20, Text: "timeout second"},
		{RawLineNumber: 30, Text: "third"},
	}
	nav := mustNavigationState(t, NavigationOptions{
		OutputCount:       len(records),
		ViewportHeight:    3,
		CursorOutputIndex: 0,
	})
	selection := NewSelectionState()
	selection.StartOrUpdateRange(10, 30)
	selection.TogglePicked(30)

	moved := nav.MoveToSearchMatchWithSelection(records, NewSearchMatcher("timeout"), SearchDirectionNext, selection)

	if !moved {
		t.Fatal("search match navigation did not move")
	}
	if _, ok := selection.Range(); ok {
		t.Fatal("search match navigation left range active")
	}
	if got, want := selection.PickedLines(), []int{30}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picked lines = %#v, want %#v", got, want)
	}
}

func TestBuildCopyTextUsesSelectedUnionUniqueSortedByRawLineNumber(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 30, Text: "thirty"},
		{RawLineNumber: 10, Text: "ten"},
		{RawLineNumber: 20, Text: "twenty"},
		{RawLineNumber: 40, Text: "forty"},
	}
	selection := NewSelectionState()
	selection.StartOrUpdateRange(20, 40)
	selection.TogglePicked(10)
	selection.TogglePicked(30)

	got := BuildCopyText(records, selection, 30)

	if got.LineCount != 4 {
		t.Fatalf("copy line count = %d, want 4", got.LineCount)
	}
	if got.Text != "ten\ntwenty\nthirty\nforty" {
		t.Fatalf("copy text = %q, want raw-line ascending unique output", got.Text)
	}
}

func TestBuildCopyTextFallsBackToCurrentCursorLineWhenSelectionIsEmpty(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 30, Text: "thirty"},
		{RawLineNumber: 10, Text: "ten"},
	}

	got := BuildCopyText(records, NewSelectionState(), 30)

	if got.LineCount != 1 || got.Text != "thirty" {
		t.Fatalf("copy fallback = %#v, want one current cursor line", got)
	}
}

func TestCopySelectedLinesWritesThroughClipboardPortAndReportsStatus(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 1, Text: "one"},
		{RawLineNumber: 2, Text: "two"},
	}
	selection := NewSelectionState()
	selection.TogglePicked(2)
	clipboard := &fakeClipboardWriter{}

	result := CopySelectedLines(context.Background(), clipboard, records, selection, 1)

	if result.Message != "1 line copied" {
		t.Fatalf("copy message = %q, want singular success", result.Message)
	}
	if clipboard.text != "two" {
		t.Fatalf("clipboard text = %q, want selected line", clipboard.text)
	}
}

func TestLogListCopyKeyWritesClipboardAndSetsStatusMessage(t *testing.T) {
	records := []OutputLogRecord{
		{RawLineNumber: 1, Text: "one"},
		{RawLineNumber: 2, Text: "two"},
	}
	app := NewInteractionState()
	app.SetFocus(InteractionFocusLogList)
	app.SetCursorRawLine(1)
	app.Selection().TogglePicked(2)
	clipboard := &fakeClipboardWriter{}

	result := app.CopyFromLogList(context.Background(), clipboard, records)

	if result.Err != nil {
		t.Fatalf("copy returned error: %v", result.Err)
	}
	if got, want := clipboard.text, "two"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
	if got, want := app.StatusMessage(), "1 line copied"; got != want {
		t.Fatalf("status message = %q, want %q", got, want)
	}
}

func TestCopyKeyInTextInputFocusDoesNotWriteClipboardOrSetStatus(t *testing.T) {
	records := []OutputLogRecord{{RawLineNumber: 1, Text: "one"}}
	for _, focus := range []InteractionFocus{InteractionFocusFilterInput, InteractionFocusSearchInput} {
		t.Run(focus.String(), func(t *testing.T) {
			app := NewInteractionState()
			app.SetFocus(focus)
			app.SetCursorRawLine(1)
			clipboard := &fakeClipboardWriter{}

			result := app.CopyFromLogList(context.Background(), clipboard, records)

			if result.LineCount != 0 {
				t.Fatalf("copy result = %#v, want no copy from text input focus", result)
			}
			if clipboard.text != "" {
				t.Fatalf("clipboard text = %q, want no write", clipboard.text)
			}
			if app.StatusMessage() != "" {
				t.Fatalf("status message = %q, want empty", app.StatusMessage())
			}
		})
	}
}

func TestFocusedTextInputsTreatSpaceAndCAsCharacters(t *testing.T) {
	app := NewInteractionState()
	app.SetFocus(InteractionFocusFilterInput)
	app.HandleRune('c')
	app.HandleRune(' ')

	if got, want := app.FilterEditingText(), "c "; got != want {
		t.Fatalf("filter edit text = %q, want %q", got, want)
	}
	if app.Selection().PickedCount() != 0 {
		t.Fatal("Space in filter input mutated selection")
	}
	if app.StatusMessage() != "" {
		t.Fatalf("c in filter input produced status message %q", app.StatusMessage())
	}

	app.SetFocus(InteractionFocusSearchInput)
	app.HandleRune('c')
	app.HandleRune(' ')
	if got, want := app.SearchEditingText(), "c "; got != want {
		t.Fatalf("search edit text = %q, want %q", got, want)
	}
}

func TestEscClearsLogListSelection(t *testing.T) {
	app := NewInteractionState()
	app.SetFocus(InteractionFocusLogList)
	app.Selection().StartOrUpdateRange(10, 20)
	app.Selection().TogglePicked(30)

	app.HandleKey(InteractionKeyEsc)

	if app.Selection().HasSelection() {
		t.Fatal("Esc did not clear log list selection")
	}
}

func TestHelpModalOpensFromEachFocusWithoutMutatingState(t *testing.T) {
	for _, focus := range []InteractionFocus{
		InteractionFocusLogList,
		InteractionFocusFilterInput,
		InteractionFocusSearchInput,
	} {
		t.Run(focus.String(), func(t *testing.T) {
			app := NewInteractionState()
			app.SetFocus(focus)
			app.SetFilterEditingText("level:ERROR")
			app.SetSearchEditingText("timeout")
			app.SetCursorRawLine(42)
			app.Selection().StartOrUpdateRange(40, 42)
			app.Selection().TogglePicked(7)
			before := app.Snapshot()

			app.HandleKey(InteractionKeyHelpF1)

			if !app.HelpOpen() {
				t.Fatal("F1 did not open help")
			}
			after := app.Snapshot()
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("state changed after help open: before %#v after %#v", before, after)
			}
		})
	}
}

func TestHelpModalClosesWithEscOrF1WithoutMutatingState(t *testing.T) {
	for _, key := range []InteractionKey{
		InteractionKeyEsc,
		InteractionKeyHelpF1,
	} {
		t.Run(key.String(), func(t *testing.T) {
			app := NewInteractionState()
			app.SetFocus(InteractionFocusLogList)
			app.SetCursorRawLine(42)
			app.Selection().StartOrUpdateRange(40, 42)
			app.Selection().TogglePicked(7)
			app.HandleKey(InteractionKeyHelpF1)
			before := app.Snapshot()

			app.HandleKey(key)

			if app.HelpOpen() {
				t.Fatalf("%s did not close open help modal", key)
			}
			if after := app.Snapshot(); !reflect.DeepEqual(after, before) {
				t.Fatalf("state changed after help close: before %#v after %#v", before, after)
			}
		})
	}
}

func TestQuestionMarkIsNotAHelpKey(t *testing.T) {
	for _, focus := range []InteractionFocus{
		InteractionFocusLogList,
		InteractionFocusFilterInput,
		InteractionFocusSearchInput,
	} {
		t.Run(focus.String(), func(t *testing.T) {
			app := NewInteractionState()
			app.SetFocus(focus)
			app.SetFilterEditingText("level:ERROR")
			app.SetSearchEditingText("timeout")
			app.SetCursorRawLine(42)
			app.Selection().StartOrUpdateRange(40, 42)
			app.Selection().TogglePicked(7)
			before := app.Snapshot()

			app.HandleKey(InteractionKeyHelpQuestion)

			if app.HelpOpen() {
				t.Fatal("? opened help")
			}
			if after := app.Snapshot(); !reflect.DeepEqual(after, before) {
				t.Fatalf("state changed after question key: before %#v after %#v", before, after)
			}
		})
	}
}

func TestQuestionMarkDoesNotCloseOpenHelp(t *testing.T) {
	app := NewInteractionState()
	app.SetFocus(InteractionFocusSearchInput)
	app.SetFilterEditingText("level:ERROR")
	app.SetSearchEditingText("timeout")
	app.SetCursorRawLine(42)
	app.Selection().StartOrUpdateRange(40, 42)
	app.Selection().TogglePicked(7)
	app.HandleKey(InteractionKeyHelpF1)
	before := app.Snapshot()

	app.HandleKey(InteractionKeyHelpQuestion)

	if !app.HelpOpen() {
		t.Fatal("? closed open help")
	}
	if after := app.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("state changed after question key while help open: before %#v after %#v", before, after)
	}
}

func TestQuestionMarkCanBeTypedInFilterAndSearchInput(t *testing.T) {
	tests := []struct {
		name  string
		focus InteractionFocus
		want  func(*InteractionState) string
	}{
		{name: "filter", focus: InteractionFocusFilterInput, want: (*InteractionState).FilterEditingText},
		{name: "search", focus: InteractionFocusSearchInput, want: (*InteractionState).SearchEditingText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewInteractionState()
			app.SetFocus(tt.focus)

			app.HandleRune('?')

			if got := tt.want(app); got != "?" {
				t.Fatalf("typed text = %q, want ?", got)
			}
			if app.HelpOpen() {
				t.Fatal("typing ? opened help")
			}
		})
	}
}

type fakeClipboardWriter struct {
	text string
}

func (w *fakeClipboardWriter) WriteText(_ context.Context, text string) error {
	w.text = text
	return nil
}
