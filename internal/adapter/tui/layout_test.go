package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"logsee/internal/usecase"
)

func TestRenderFrameZonesInOrderAndListHeight(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  80,
		Height: 12,
		Filter: TextInputModel{
			Text: "level:ERROR",
		},
		Search: TextInputModel{
			Text: "timeout",
		},
		Logs: []LogLineModel{
			{RawLineNumber: 42, Text: "visible log line"},
		},
		Status: baseStatusModel(),
	})

	gotNames := zoneNames(frame.Zones)
	wantNames := []ZoneName{ZoneFilterInput, ZoneSearchInput, ZoneLogList, ZoneStatusbar}
	if strings.Join(zoneNameStrings(gotNames), ",") != strings.Join(zoneNameStrings(wantNames), ",") {
		t.Fatalf("zone order = %v, want %v", gotNames, wantNames)
	}

	list := findZone(t, frame, ZoneLogList)
	if got, want := list.Height, 9; got != want {
		t.Fatalf("list viewport height = %d, want %d", got, want)
	}

	status := findZone(t, frame, ZoneStatusbar)
	if status.Focusable {
		t.Fatal("statusbar must not accept focus")
	}
	if got, want := status.Height, 1; got != want {
		t.Fatalf("statusbar height = %d, want %d", got, want)
	}
}

func TestFrameTextWritesZonesInRenderOrder(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  80,
		Height: 6,
		Filter: TextInputModel{
			Text: "level:ERROR",
		},
		Search: TextInputModel{
			Text: "timeout",
		},
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "first"},
			{RawLineNumber: 2, Text: "second"},
		},
		Status: baseStatusModel(),
	})

	got := FrameText(frame)
	for _, want := range []string{
		"filter:level:ERROR\n",
		"search:timeout\n",
		"1   first\n",
		"2   second\n",
		"lines:120/9834 | in:stdio:read",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frame text %q does not contain %q", got, want)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("frame text %q must end with newline", got)
	}
}

func TestFrameTextPadsEmptyLogListSoStatusbarStaysBottom(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  80,
		Height: 6,
		Status: baseStatusModel(),
	})

	got := FrameText(frame)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if got, want := len(lines), 6; got != want {
		t.Fatalf("frame lines = %#v, count = %d, want %d", lines, got, want)
	}
	if got, want := lines[0], "filter:"; got != want {
		t.Fatalf("filter row = %q, want %q", got, want)
	}
	if got, want := lines[1], "search:"; got != want {
		t.Fatalf("search row = %q, want %q", got, want)
	}
	for row := 2; row <= 4; row++ {
		if lines[row] != "" {
			t.Fatalf("list viewport row %d = %q, want blank row", row, lines[row])
		}
	}
	if !strings.HasPrefix(lines[5], "lines:") {
		t.Fatalf("statusbar row = %q, want bottom statusbar", lines[5])
	}
}

func TestStyledFrameTextUsesLipglossStyles(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 5,
		Filter: TextInputModel{
			Text: "level:ERROR",
		},
		Search: TextInputModel{
			Text: "timeout",
		},
		Logs: []LogLineModel{
			{RawLineNumber: 7, Bookmark: 1, Text: "timeout database"},
		},
		Status: baseStatusModel(),
	})

	got := StyledFrameText(frame)
	plain := stripANSI(got)
	for _, want := range []string{
		"FILTER INPUT(':')  │  level:ERROR",
		"SEARCH INPUT('/')  │  timeout",
		"7 1 timeout database",
		"\x1b[",
	} {
		source := got
		if want != "\x1b[" {
			source = plain
		}
		if !strings.Contains(source, want) {
			t.Fatalf("styled frame text %q does not contain %q", got, want)
		}
	}
	if plain := FrameText(frame); strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain frame text %q should remain unstyled", plain)
	}
}

func TestStyledFrameTextUsesFullWidthChromeBackgrounds(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  70,
		Height: 5,
		Filter: TextInputModel{
			Text: "level:ERROR",
		},
		Search: TextInputModel{
			Text: "timeout",
		},
		Status: baseStatusModel(),
	})

	got := StyledFrameText(frame)
	rawLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	plainLines := strings.Split(stripANSI(strings.TrimSuffix(got, "\n")), "\n")
	if got, want := len(rawLines), 5; got != want {
		t.Fatalf("styled frame lines = %#v, count = %d, want %d", plainLines, got, want)
	}
	for _, row := range []int{0, 1, 4} {
		if got, want := lipgloss.Width(plainLines[row]), frame.Width; got != want {
			t.Fatalf("styled row %d width = %d, want %d: %q", row, got, want, plainLines[row])
		}
	}
	for row, wantBG := range map[int]string{
		0: "48;5;236",
		1: "48;5;236",
		4: "48;5;233",
	} {
		if !strings.Contains(rawLines[row], wantBG) {
			t.Fatalf("styled row %d = %q, want background color %q", row, rawLines[row], wantBG)
		}
	}
}

func TestStyledFrameTextRendersVisibleEditingChrome(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 5,
		Filter: TextInputModel{
			Text:      "level:ERROR",
			Editing:   true,
			CursorPos: 11,
		},
		Search: TextInputModel{
			Text:      "timeout",
			Editing:   true,
			CursorPos: 7,
		},
		Status: baseStatusModel(),
	})

	got := StyledFrameText(frame)
	for _, want := range []string{
		"FILTER INPUT(':')  │  > level:ERROR_",
		"SEARCH INPUT('/')  │  > timeout_",
		"38;5;254",
		"48;5;25",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("styled frame text %q does not contain visible editing chrome %q", got, want)
		}
	}
}

func TestFrameTextRendersVisibleEmptyFilterEditingState(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 5,
		Filter: TextInputModel{
			Editing: true,
		},
		Status: baseStatusModel(),
	})

	got := FrameText(frame)
	if !strings.Contains(got, "filter:> _\n") {
		t.Fatalf("frame text %q does not show empty filter editing state", got)
	}
}

func TestFormatEditingTextRendersCursorInMiddle(t *testing.T) {
	got := formatEditingText("error", 3)
	if got != "err_or" {
		t.Fatalf("formatEditingText = %q, want err_or", got)
	}
}

func TestFrameTextRendersEditingCursorAtEndOfInput(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 5,
		Filter: TextInputModel{
			Text:      "level:ERROR",
			Editing:   true,
			CursorPos: 11,
		},
		Search: TextInputModel{
			Text:      "timeout",
			Editing:   true,
			CursorPos: 7,
		},
		Status: baseStatusModel(),
	})

	got := FrameText(frame)
	for _, want := range []string{
		"filter:> level:ERROR_\n",
		"search:> timeout_\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frame text %q does not render end cursor %q", got, want)
		}
	}
}

func TestStyledFrameTextKeepsInputChromeSingleLineUnderWidthPressure(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  10,
		Height: 5,
		Filter: TextInputModel{
			Editing: true,
		},
		Search: TextInputModel{},
		Status: baseStatusModel(),
	})

	got := stripANSI(StyledFrameText(frame))
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if got, want := len(lines), 5; got != want {
		t.Fatalf("styled frame lines = %#v, count = %d, want %d stable zone lines", lines, got, want)
	}
	if !strings.HasPrefix(lines[0], "FILTER") {
		t.Fatalf("first styled line = %q, want filter input chrome", lines[0])
	}
	if !strings.HasPrefix(lines[1], "SEARCH") {
		t.Fatalf("second styled line = %q, want search input chrome", lines[1])
	}
	if lines[2] != "" || lines[3] != "" {
		t.Fatalf("list viewport blank lines = %#v, want two empty rows", lines[2:4])
	}
	if !strings.HasPrefix(lines[4], "lines:") {
		t.Fatalf("bottom styled line = %q, want statusbar", lines[4])
	}
}

func TestStyledFrameTextUsesPerSearchHighlightColors(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 6,
		Logs: []LogLineModel{
			{
				RawLineNumber: 1,
				Text:          "error timeout",
				Highlights: []usecase.HighlightRange{
					{Start: 0, End: 5, Color: "red"},
					{Start: 6, End: 13, Color: "green"},
				},
			},
		},
		Status: baseStatusModel(),
	})

	got := StyledFrameText(frame)
	if !strings.Contains(got, "48;5;196") {
		t.Fatalf("styled frame text %q must contain red highlight background", got)
	}
	if !strings.Contains(got, "48;5;34") {
		t.Fatalf("styled frame text %q must contain green highlight background", got)
	}
}

func TestStyledFrameTextColorsLogVisualStates(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 7, Bookmark: 1, Text: "bookmarked"},
			{
				RawLineNumber: 8,
				Text:          "timeout database",
				Highlights: []usecase.HighlightRange{
					{Start: 0, End: 7},
				},
				Cursor: true,
			},
		},
		Status: baseStatusModel(),
	})

	got := StyledFrameText(frame)
	plain := stripANSI(got)
	for _, want := range []string{
		"7 1 bookmarked",
		"8   timeout database",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("styled frame text %q does not contain visible log state %q", got, want)
		}
	}
	for _, want := range []string{
		"48;5;25",
		"38;5;230",
		"48;5;214",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("styled frame text %q does not contain colored log state %q", got, want)
		}
	}
	if !hasSGRParam(got, "30") {
		t.Fatalf("styled frame text %q must render search highlight foreground with ANSI black", got)
	}
	if !hasSGRParam(got, "7") {
		t.Fatalf("styled frame text %q must render cursor row with reverse video", got)
	}
}

func TestStyledFrameTextColorsStatusbarOnOffValues(t *testing.T) {
	status := baseStatusModel()
	status.Follow = true
	status.FilterText = "level:ERROR"
	status.SearchText = ""

	frame := RenderFrame(RenderModel{
		Width:  140,
		Height: 4,
		Status: status,
	})

	got := StyledFrameText(frame)
	for _, want := range []string{
		"follow:",
		"38;5;120",
		"38;5;240",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("styled frame text %q does not contain status color/state %q", got, want)
		}
	}
	for _, removed := range []string{"sot:", "filter:", "search:"} {
		if strings.Contains(stripANSI(got), removed) {
			t.Fatalf("styled frame text %q must not contain removed status field %q", got, removed)
		}
	}
}

func TestStyledFrameTextColorsCopySuccessStatusMessage(t *testing.T) {
	status := baseStatusModel()
	status.Message = "2 lines copied"

	frame := RenderFrame(RenderModel{
		Width:  180,
		Height: 4,
		Status: status,
	})

	got := StyledFrameText(frame)
	plain := stripANSI(got)
	for _, want := range []string{
		"msg:",
		"2 lines copied",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("styled frame text %q does not contain copy message %q", got, want)
		}
	}
	for _, want := range []string{
		"38;5;16",
		"48;5;120",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("styled frame text %q does not contain copy success color %q", got, want)
		}
	}
}

func TestStyledFrameTextHighlightsArbitraryStatusMessage(t *testing.T) {
	status := baseStatusModel()
	status.Message = "no match"

	frame := RenderFrame(RenderModel{
		Width:  180,
		Height: 4,
		Status: status,
	})

	got := StyledFrameText(frame)
	plain := stripANSI(got)
	for _, want := range []string{
		"msg:",
		"no match",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("styled frame text %q does not contain status message %q", got, want)
		}
	}
	for _, want := range []string{
		"38;5;16",
		"48;5;120",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("styled frame text %q does not contain highlighted message color %q", got, want)
		}
	}
}

func TestStatusbarFieldsRenderInRequiredOrderWithoutRemovedFields(t *testing.T) {
	status := baseStatusModel()
	status.CursorRawLine = 120
	status.TotalRawLines = 9834
	status.InputMode = InputModeStdio
	status.ReadState = ReadStateRead
	status.LogType = usecase.LogTypeADB
	status.LogTypePending = false
	status.Follow = true
	status.OutputPath = "/tmp/session.log"
	status.SOTPath = "/tmp/session.log"
	status.FilterText = "level:ERROR"
	status.SearchText = "timeout"
	status.Bookmarks = []int{4, 1, 3}
	status.Selection = &LineRange{Start: 120, End: 130}
	status.PickedCount = 3
	status.Message = "3 lines copied"
	status.Wrap = true

	got := BuildStatusbar(status, 200)
	want := "lines:120/9834 | in:stdio:read | type:adb | follow:on | wrap:on | out:/tmp/session.log | bm:1,3,4 | sel:3 | msg:3 lines copied"
	if got != want {
		t.Fatalf("statusbar = %q, want %q", got, want)
	}
}

func TestStatusbarOmitsSOTFilterAndSearchEvenWhenSet(t *testing.T) {
	status := baseStatusModel()
	status.SOTPath = "/tmp/source-of-truth.log"
	status.FilterText = "level:ERROR"
	status.SearchText = "timeout"

	got := BuildStatusbar(status, 200)
	for _, removed := range []string{"sot:", "filter:", "search:"} {
		if strings.Contains(got, removed) {
			t.Fatalf("statusbar = %q, must omit %q", got, removed)
		}
	}
	for _, want := range []string{"lines:", "in:", "type:", "follow:", "wrap:", "out:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("statusbar = %q, want preserved field %q", got, want)
		}
	}
}

func TestStatusbarCapsOutputPathBeforeWidthPressure(t *testing.T) {
	status := baseStatusModel()
	status.OutputPath = "/very/long/output/path/with/a/session-output-file-name-that-stays-too-wide.log"
	status.Bookmarks = []int{1}
	status.PickedCount = 4
	status.Message = "4 lines copied"

	got := BuildStatusbar(status, 500)
	out := statusbarValue(t, got, "out")
	if len(out) > 24 {
		t.Fatalf("out field value length = %d, want <= 24: %q in %q", len(out), out, got)
	}
	if !strings.Contains(out, "...") {
		t.Fatalf("out field value = %q, want middle abbreviation in %q", out, got)
	}
	if strings.Contains(got, status.OutputPath) {
		t.Fatalf("statusbar = %q, must not render full output path", got)
	}
	for _, want := range []string{"bm:1", "sel:4", "msg:4 lines copied"} {
		if !strings.Contains(got, want) {
			t.Fatalf("statusbar = %q, want field after output path %q", got, want)
		}
	}
	if strings.Contains(got, "picked:") {
		t.Fatalf("statusbar = %q, must combine picked status into sel", got)
	}
}

func TestStatusbarAbbreviatesPathsAndOmitsAuxiliaryFieldsUnderWidthPressure(t *testing.T) {
	status := baseStatusModel()
	status.OutputPath = "/very/long/output/path/session-output-file-for-stdio.log"
	status.SOTPath = "/very/long/source/of/truth/path/session-source-of-truth.log"
	status.FilterText = "level:ERROR"
	status.SearchText = "timeout"
	status.Bookmarks = []int{1, 3, 4}
	status.Selection = &LineRange{Start: 120, End: 130}
	status.PickedCount = 7
	status.Message = "3 lines copied"

	got := BuildStatusbar(status, 96)
	if len(got) > 96 {
		t.Fatalf("statusbar width = %d, want <= 96: %q", len(got), got)
	}
	for _, required := range []string{"lines:120/9834", "in:stdio:read", "type:adb", "follow:on", "wrap:off"} {
		if !strings.Contains(got, required) {
			t.Fatalf("statusbar %q does not preserve required field %q", got, required)
		}
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("statusbar %q does not abbreviate long paths", got)
	}
	for _, omitted := range []string{"bm:", "sel:", "picked:", "search:", "filter:", "sot:"} {
		if strings.Contains(got, omitted) {
			t.Fatalf("statusbar %q did not omit auxiliary field %q under width pressure", got, omitted)
		}
	}
}

func TestFilterErrorsRenderOnlyInFilterInput(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  120,
		Height: 10,
		Filter: TextInputModel{
			Text:  `level:"ERROR`,
			Error: "unterminated quoted filter token",
		},
		Search: TextInputModel{
			Text: "timeout",
		},
		Status: baseStatusModel(),
	})

	filter := findZone(t, frame, ZoneFilterInput)
	if !strings.Contains(strings.Join(filter.Lines, "\n"), "unterminated quoted filter token") {
		t.Fatalf("filter zone lines = %v, want filter parse error", filter.Lines)
	}

	status := findZone(t, frame, ZoneStatusbar)
	if strings.Contains(strings.Join(status.Lines, "\n"), "unterminated quoted filter token") {
		t.Fatalf("statusbar duplicated filter parse error: %v", status.Lines)
	}
}

func TestLogRowsPreserveBookmarkColumnAlignment(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  80,
		Height: 5,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Bookmark: 7, Text: "bookmarked"},
			{RawLineNumber: 2, Text: "unbookmarked"},
		},
		Status: baseStatusModel(),
	})

	list := findZone(t, frame, ZoneLogList)
	if got, want := list.Lines[0], "1 7 bookmarked"; got != want {
		t.Fatalf("bookmarked row = %q, want %q", got, want)
	}
	if got, want := list.Lines[1], "2   unbookmarked"; got != want {
		t.Fatalf("unbookmarked row = %q, want %q", got, want)
	}
	if got, want := strings.Index(list.Lines[0], "bookmarked"), strings.Index(list.Lines[1], "unbookmarked"); got != want {
		t.Fatalf("log text start columns = %d and %d, want equal", got, want)
	}
}

func TestRenderFrameTruncatesLongLogRowsWhenWrapDisabled(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  10,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "abcdefghij"},
			{RawLineNumber: 2, Text: "second"},
		},
		Status: baseStatusModel(),
		Wrap:   false,
	})

	list := findZone(t, frame, ZoneLogList)
	want := []string{
		"1   abcdef",
		"2   second",
	}
	if !reflect.DeepEqual(list.Lines, want) {
		t.Fatalf("list lines = %#v, want %#v", list.Lines, want)
	}
}

func TestRenderFrameHorizontallyScrollsAllLogRowsWhenWrapDisabled(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:            10,
		Height:           6,
		HorizontalOffset: 2,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "abcdefghij"},
			{RawLineNumber: 2, Text: "second"},
			{RawLineNumber: 3, Text: "xy"},
		},
		Status: baseStatusModel(),
		Wrap:   false,
	})

	list := findZone(t, frame, ZoneLogList)
	want := []string{
		"1   cdefgh",
		"2   cond",
		"3   ",
	}
	if !reflect.DeepEqual(list.Lines, want) {
		t.Fatalf("list lines = %#v, want %#v", list.Lines, want)
	}
}

func TestRenderFrameIgnoresHorizontalScrollWhenWrapEnabled(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:            10,
		Height:           6,
		HorizontalOffset: 2,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "abcdefghij"},
		},
		Status: baseStatusModel(),
		Wrap:   true,
	})

	list := findZone(t, frame, ZoneLogList)
	want := []string{
		"1   abcdef",
		"    ghij",
	}
	if !reflect.DeepEqual(list.Lines, want) {
		t.Fatalf("list lines = %#v, want %#v", list.Lines, want)
	}
}

func TestHorizontalScrollRebasesSearchHighlights(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:            10,
		Height:           6,
		HorizontalOffset: 2,
		Logs: []LogLineModel{
			{
				RawLineNumber: 1,
				Text:          "abcdefghij",
				Highlights: []usecase.HighlightRange{
					{Start: 3, End: 5},
				},
			},
		},
		Status: baseStatusModel(),
		Wrap:   false,
	})

	list := findZone(t, frame, ZoneLogList)
	if got, want := list.Lines[0], "1   cdefgh"; got != want {
		t.Fatalf("list line = %q, want %q", got, want)
	}
	wantHighlights := [][]usecase.HighlightRange{{{Start: 1, End: 3}}}
	if !reflect.DeepEqual(list.Highlights, wantHighlights) {
		t.Fatalf("highlights = %#v, want %#v", list.Highlights, wantHighlights)
	}
}

func TestRenderFrameWrapsLogRowsWhenEnabled(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  10,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "abcdefghij"},
			{RawLineNumber: 2, Text: "second"},
		},
		Status: baseStatusModel(),
		Wrap:   true,
	})

	list := findZone(t, frame, ZoneLogList)
	want := []string{
		"1   abcdef",
		"    ghij",
		"2   second",
	}
	if !reflect.DeepEqual(list.Lines, want) {
		t.Fatalf("list lines = %#v, want %#v", list.Lines, want)
	}
}

func TestRenderFrameWrapKeepsCursorRowVisible(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  10,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "abcdefghijklmnop"},
			{RawLineNumber: 2, Text: "cursor", Cursor: true},
		},
		Status: baseStatusModel(),
		Wrap:   true,
	})

	list := findZone(t, frame, ZoneLogList)
	if !containsLine(list.Lines, "2   cursor") {
		t.Fatalf("wrapped list lines = %#v, want cursor row visible", list.Lines)
	}
	if len(list.Logs) == 0 || !list.Logs[len(list.Logs)-1].Cursor {
		t.Fatalf("wrapped list logs = %#v, want visible cursor metadata", list.Logs)
	}
}

func TestStyledFrameTextHighlightsCursorRowWhenWrapKeepsItVisible(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  10,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 1, Text: "abcdefghijklmnop"},
			{RawLineNumber: 2, Text: "cursor", Cursor: true},
		},
		Status: baseStatusModel(),
		Wrap:   true,
	})

	got := StyledFrameText(frame)
	if !strings.Contains(stripANSI(got), "2   cursor") {
		t.Fatalf("styled frame %q does not contain cursor row", got)
	}
	if !hasSGRParam(got, "7") {
		t.Fatalf("styled frame %q must render wrapped cursor row with reverse video", got)
	}
}

func TestLogLineNumberUsesFaintLowerContrastStyle(t *testing.T) {
	line := styleLogLine(LogLineModel{RawLineNumber: 42, Text: "hello world"})
	if !hasSGRParam(line, "2") {
		t.Fatalf("log line %q must render line number with faint (lower contrast)", line)
	}
}

func TestLogLineNumberStaysDimOnCursorRowWithoutGutterReverse(t *testing.T) {
	line := styleLogLine(LogLineModel{RawLineNumber: 5, Text: "body", Cursor: true})
	if !hasSGRParam(line, "2") {
		t.Fatalf("cursor log line %q must keep faint gutter", line)
	}
	if !hasSGRParam(line, "7") {
		t.Fatalf("cursor log line %q must reverse body for cursor row", line)
	}
}

func TestLogRowsRenderRangeAndPickedSelectionIndicators(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  80,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 10, Text: "range", Selected: SelectionKindRange},
			{RawLineNumber: 20, Text: "picked", Selected: SelectionKindPicked},
			{RawLineNumber: 30, Text: "plain"},
		},
		Status: baseStatusModel(),
	})

	list := findZone(t, frame, ZoneLogList)
	if got, want := list.Lines[0], "10   range"; got != want {
		t.Fatalf("range row = %q, want %q", got, want)
	}
	if got, want := list.Lines[1], "20   picked"; got != want {
		t.Fatalf("picked row = %q, want %q", got, want)
	}
	if got, want := list.Lines[2], "30   plain"; got != want {
		t.Fatalf("plain row = %q, want %q", got, want)
	}
}

func TestSelectionRowsUseStyleAndStatusWithoutTextPrefix(t *testing.T) {
	status := baseStatusModel()
	status.PickedCount = 1
	status.Selection = &LineRange{Start: 10, End: 20}
	frame := RenderFrame(RenderModel{
		Width:  180,
		Height: 6,
		Logs: []LogLineModel{
			{RawLineNumber: 10, Text: "range row", Selected: SelectionKindRange},
			{RawLineNumber: 20, Text: "picked row", Selected: SelectionKindPicked},
		},
		Status: status,
	})

	plainFrame := FrameText(frame)
	for _, unwanted := range []string{"[sel]", "[picked]"} {
		if strings.Contains(plainFrame, unwanted) {
			t.Fatalf("plain frame %q must not include selection text prefix %q", plainFrame, unwanted)
		}
	}
	styled := StyledFrameText(frame)
	plainStyled := stripANSI(styled)
	for _, unwanted := range []string{"[sel]", "[picked]"} {
		if strings.Contains(plainStyled, unwanted) {
			t.Fatalf("styled frame %q must not include selection text prefix %q", styled, unwanted)
		}
	}
	for _, want := range []string{
		"10   range row",
		"20   picked row",
		"sel:1",
		"48;5;236",
		"48;5;238",
	} {
		if !strings.Contains(styled, want) && !strings.Contains(stripANSI(styled), want) {
			t.Fatalf("styled frame %q does not contain %q", styled, want)
		}
	}
	if strings.Contains(stripANSI(styled), "picked:") {
		t.Fatalf("styled frame %q must not contain picked status field", styled)
	}
}

func TestLogListCarriesSearchHighlightMetadata(t *testing.T) {
	frame := RenderFrame(RenderModel{
		Width:  80,
		Height: 5,
		Logs: []LogLineModel{
			{
				RawLineNumber: 1,
				Text:          "timeout",
				Highlights: []usecase.HighlightRange{
					{Start: 0, End: 7},
				},
			},
		},
		Status: baseStatusModel(),
	})

	list := findZone(t, frame, ZoneLogList)
	want := [][]usecase.HighlightRange{
		{{Start: 0, End: 7}},
	}
	if !reflect.DeepEqual(list.Highlights, want) {
		t.Fatalf("log list highlights = %#v, want %#v", list.Highlights, want)
	}
}

func baseStatusModel() StatusModel {
	return StatusModel{
		CursorRawLine: 120,
		TotalRawLines: 9834,
		InputMode:     InputModeStdio,
		ReadState:     ReadStateRead,
		LogType:       usecase.LogTypeADB,
		Follow:        true,
		OutputPath:    "/tmp/session.log",
		SOTPath:       "/tmp/session.log",
	}
}

func hasSGRParam(text, want string) bool {
	for {
		start := strings.Index(text, "\x1b[")
		if start < 0 {
			return false
		}
		text = text[start+2:]
		end := strings.IndexByte(text, 'm')
		if end < 0 {
			return false
		}
		for _, part := range strings.Split(text[:end], ";") {
			if part == want {
				return true
			}
		}
		text = text[end+1:]
	}
}

func statusbarValue(t *testing.T, statusbar string, key string) string {
	t.Helper()

	prefix := key + ":"
	for _, field := range strings.Split(statusbar, " | ") {
		if strings.HasPrefix(field, prefix) {
			return strings.TrimPrefix(field, prefix)
		}
	}
	t.Fatalf("statusbar = %q, missing field %q", statusbar, key)
	return ""
}

func stripANSI(text string) string {
	var builder strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\x1b' || i+1 >= len(text) || text[i+1] != '[' {
			builder.WriteByte(text[i])
			continue
		}
		i += 2
		for i < len(text) && text[i] != 'm' {
			i++
		}
	}
	return builder.String()
}

func zoneNames(zones []Zone) []ZoneName {
	names := make([]ZoneName, len(zones))
	for i, zone := range zones {
		names[i] = zone.Name
	}
	return names
}

func zoneNameStrings(names []ZoneName) []string {
	values := make([]string, len(names))
	for i, name := range names {
		values[i] = string(name)
	}
	return values
}

func findZone(t *testing.T, frame Frame, name ZoneName) Zone {
	t.Helper()
	for _, zone := range frame.Zones {
		if zone.Name == name {
			return zone
		}
	}
	t.Fatalf("zone %q not found in %#v", name, frame.Zones)
	return Zone{}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
