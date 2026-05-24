package tui

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"logsee/internal/usecase"
)

const (
	statusSeparator         = " | "
	statusPathValueMaxWidth = 24
)

type ZoneName string

const (
	ZoneFilterInput ZoneName = "filter"
	ZoneSearchInput ZoneName = "search"
	ZoneLogList     ZoneName = "list"
	ZoneStatusbar   ZoneName = "statusbar"
	ZoneHelpModal   ZoneName = "help"
)

type InputMode string

const (
	InputModeStdio InputMode = "stdio"
	InputModeFile  InputMode = "file"
)

type ReadState string

const (
	ReadStateRead ReadState = "read"
	ReadStateEOF  ReadState = "eof"
)

type TextInputModel struct {
	Text    string
	Error   string
	Editing bool
}

type LogLineModel struct {
	RawLineNumber int
	Bookmark      int
	Text          string
	Highlights    []usecase.HighlightRange
	Selected      SelectionKind
	Cursor        bool
	Continuation  bool
	PrefixWidth   int
}

type LineRange struct {
	Start int
	End   int
}

type SelectionKind string

const (
	SelectionKindNone   SelectionKind = ""
	SelectionKindRange  SelectionKind = "range"
	SelectionKindPicked SelectionKind = "picked"
)

type StatusModel struct {
	CursorRawLine int
	TotalRawLines int

	InputMode InputMode
	ReadState ReadState

	LogType        usecase.LogType
	LogTypePending bool
	Follow         bool
	Wrap           bool

	OutputPath string
	SOTPath    string

	FilterText string
	SearchText string

	Bookmarks   []int
	Selection   *LineRange
	PickedCount int
	Message     string
}

type RenderModel struct {
	Width  int
	Height int

	Filter   TextInputModel
	Search   TextInputModel
	Logs     []LogLineModel
	Status   StatusModel
	HelpOpen bool
	Wrap     bool

	HorizontalOffset int
}

type Frame struct {
	Width  int
	Height int
	Zones  []Zone
}

type Zone struct {
	Name       ZoneName
	Height     int
	Focusable  bool
	Lines      []string
	Highlights [][]usecase.HighlightRange
	Input      TextInputModel
	Logs       []LogLineModel
}

func RenderFrame(model RenderModel) Frame {
	listHeight := model.Height - 3
	if listHeight < 0 {
		listHeight = 0
	}

	visibleLogs := visualLogModels(model.Logs, listHeight, model.Width, model.Wrap, model.HorizontalOffset)
	logLines := formatLogLines(visibleLogs, listHeight)
	logHighlights := formatLogHighlights(visibleLogs, listHeight)

	frame := Frame{
		Width:  model.Width,
		Height: model.Height,
		Zones: []Zone{
			{
				Name:      ZoneFilterInput,
				Height:    1,
				Focusable: true,
				Lines:     []string{formatInputLine("filter", model.Filter)},
				Input:     model.Filter,
			},
			{
				Name:      ZoneSearchInput,
				Height:    1,
				Focusable: true,
				Lines:     []string{formatInputLine("search", model.Search)},
				Input:     model.Search,
			},
			{
				Name:       ZoneLogList,
				Height:     listHeight,
				Focusable:  true,
				Lines:      logLines,
				Highlights: logHighlights,
				Logs:       visibleLogs,
			},
			{
				Name:      ZoneStatusbar,
				Height:    1,
				Focusable: false,
				Lines:     []string{BuildStatusbar(model.Status, model.Width)},
			},
		},
	}
	if model.HelpOpen {
		frame.Zones = append(frame.Zones, Zone{
			Name:      ZoneHelpModal,
			Height:    helpModalHeight(model.Height),
			Focusable: true,
			Lines:     formatHelpModalLines(),
		})
	}
	return frame
}

func FrameText(frame Frame) string {
	var builder strings.Builder
	for _, zone := range frame.Zones {
		for index := 0; index < zone.Height; index++ {
			line := ""
			if index < len(zone.Lines) {
				line = zone.Lines[index]
			}
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

var frameRenderer = newFrameRenderer()

var (
	logListStyle = frameRenderer.NewStyle().
			Foreground(lipgloss.Color("252"))
	statusbarBaseStyle = frameRenderer.NewStyle().
				Foreground(lipgloss.Color("245")).
				Background(lipgloss.Color("233"))
	statusbarOnStyle = frameRenderer.NewStyle().
				Foreground(lipgloss.Color("120")).
				Background(lipgloss.Color("233")).
				Bold(true)
	statusbarOffStyle = frameRenderer.NewStyle().
				Foreground(lipgloss.Color("240")).
				Background(lipgloss.Color("233")).
				Faint(true)
	statusbarMessageStyle = frameRenderer.NewStyle().
				Foreground(lipgloss.Color("16")).
				Background(lipgloss.Color("120")).
				Bold(true)
	helpModalStyle = frameRenderer.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")).
			Bold(true)
)

func newFrameRenderer() *lipgloss.Renderer {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.ANSI256)
	renderer.SetHasDarkBackground(true)
	return renderer
}

func StyledFrameText(frame Frame) string {
	var builder strings.Builder
	for _, zone := range frame.Zones {
		if zone.Name == ZoneLogList {
			for index := 0; index < zone.Height; index++ {
				if index < len(zone.Logs) {
					builder.WriteString(styleLogLine(zone.Logs[index]))
				} else {
					builder.WriteString(styleZoneLine(zone, "", frame.Width))
				}
				builder.WriteByte('\n')
			}
			continue
		}
		for index := 0; index < zone.Height; index++ {
			line := ""
			if index < len(zone.Lines) {
				line = zone.Lines[index]
			}
			builder.WriteString(styleZoneLine(zone, line, frame.Width))
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func styleZoneLine(zone Zone, line string, width int) string {
	switch zone.Name {
	case ZoneFilterInput:
		return renderStyledLine(inputBarStyle(zone.Input, ZoneFilterInput), formatInputChromeLine(ZoneFilterInput, zone.Input), width)
	case ZoneSearchInput:
		return renderStyledLine(inputBarStyle(zone.Input, ZoneSearchInput), formatInputChromeLine(ZoneSearchInput, zone.Input), width)
	case ZoneLogList:
		return logListStyle.Render(line)
	case ZoneStatusbar:
		return styleStatusbarLine(line, width)
	case ZoneHelpModal:
		return helpModalStyle.Render(line)
	default:
		return line
	}
}

func renderStyledLine(style lipgloss.Style, line string, width int) string {
	if width < 1 {
		return style.Render(line)
	}
	line = truncateRunes(line, width)
	return style.Width(width).Render(line)
}

func inputBarStyle(input TextInputModel, name ZoneName) lipgloss.Style {
	base := frameRenderer.NewStyle().Bold(true)
	switch name {
	case ZoneFilterInput:
		base = base.Foreground(lipgloss.Color("254"))
		if input.Editing {
			return base.Background(lipgloss.Color("25"))
		}
		return base.Background(lipgloss.Color("236"))
	case ZoneSearchInput:
		if input.Editing {
			return base.Foreground(lipgloss.Color("254")).Background(lipgloss.Color("25"))
		}
		if strings.TrimSpace(input.Text) != "" {
			return base.Foreground(lipgloss.Color("254")).Background(lipgloss.Color("236"))
		}
		return base.Foreground(lipgloss.Color("247")).Background(lipgloss.Color("235"))
	default:
		return base
	}
}

func formatInputChromeLine(name ZoneName, input TextInputModel) string {
	label := "FILTER INPUT"
	if name == ZoneSearchInput {
		label = "SEARCH INPUT"
	}

	var builder strings.Builder
	builder.WriteString(label)
	builder.WriteString("  │  ")
	if input.Editing {
		builder.WriteString("> ")
		builder.WriteString(formatEditingText(input.Text))
	} else if strings.TrimSpace(input.Text) == "" {
		builder.WriteString("∅")
	} else {
		builder.WriteString(input.Text)
	}
	if input.Error != "" {
		builder.WriteString("  │  err: ")
		builder.WriteString(input.Error)
	}
	return builder.String()
}

func styleLogLine(log LogLineModel) string {
	base := logListStyle
	switch log.Selected {
	case SelectionKindRange:
		base = frameRenderer.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	case SelectionKindPicked:
		base = frameRenderer.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("252"))
	}
	if log.Cursor {
		base = base.Reverse(true)
	}

	if log.Continuation {
		return base.Render(strings.Repeat(" ", log.PrefixWidth)) + styleHighlightedText(log.Text, log.Highlights, base)
	}

	bookmark := " "
	if log.Bookmark > 0 {
		bookmark = bookmarkBadgeStyle(log.Cursor).Render(strconv.Itoa(log.Bookmark))
	}
	var builder strings.Builder
	builder.WriteString(base.Render(strconv.Itoa(log.RawLineNumber)))
	builder.WriteString(base.Render(" "))
	builder.WriteString(bookmark)
	builder.WriteString(base.Render(" "))
	if selection := selectionPrefix(log.Selected); selection != "" {
		builder.WriteString(base.Render(selection + " "))
	}
	builder.WriteString(styleHighlightedText(log.Text, log.Highlights, base))
	return builder.String()
}

func bookmarkBadgeStyle(cursor bool) lipgloss.Style {
	if cursor {
		return frameRenderer.NewStyle().
			Background(lipgloss.Color("214")).
			Foreground(lipgloss.Color("16")).
			Bold(true)
	}
	return frameRenderer.NewStyle().
		Background(lipgloss.Color("25")).
		Foreground(lipgloss.Color("230")).
		Bold(true)
}

func styleHighlightedText(text string, ranges []usecase.HighlightRange, base lipgloss.Style) string {
	ranges = mergedHighlightRanges(text, ranges)
	if len(ranges) == 0 {
		return base.Render(text)
	}

	match := base.
		Background(lipgloss.Color("214")).
		Foreground(lipgloss.Color("0"))

	var builder strings.Builder
	pos := 0
	for _, r := range ranges {
		if r.Start > pos {
			builder.WriteString(base.Render(text[pos:r.Start]))
		}
		builder.WriteString(match.Render(text[r.Start:r.End]))
		pos = r.End
	}
	if pos < len(text) {
		builder.WriteString(base.Render(text[pos:]))
	}
	return builder.String()
}

func mergedHighlightRanges(text string, ranges []usecase.HighlightRange) []usecase.HighlightRange {
	if len(ranges) == 0 || text == "" {
		return nil
	}

	clean := make([]usecase.HighlightRange, 0, len(ranges))
	for _, r := range ranges {
		if r.Start < 0 {
			r.Start = 0
		}
		if r.End > len(text) {
			r.End = len(text)
		}
		if r.Start >= r.End {
			continue
		}
		clean = append(clean, r)
	}
	if len(clean) == 0 {
		return nil
	}

	sort.Slice(clean, func(i, j int) bool {
		if clean[i].Start == clean[j].Start {
			return clean[i].End < clean[j].End
		}
		return clean[i].Start < clean[j].Start
	})

	merged := clean[:1]
	for _, r := range clean[1:] {
		last := &merged[len(merged)-1]
		if r.Start <= last.End {
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

func styleStatusbarLine(line string, width int) string {
	if width < 0 {
		width = 0
	}
	if line == "" {
		return statusbarBaseStyle.Render(strings.Repeat(" ", width))
	}

	parts := strings.Split(line, statusSeparator)
	styled := make([]string, 0, len(parts))
	for _, part := range parts {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			styled = append(styled, statusbarBaseStyle.Render(part))
			continue
		}
		styled = append(styled, statusbarBaseStyle.Render(key+":")+styleStatusValue(key, value))
	}
	row := strings.Join(styled, statusbarBaseStyle.Render(statusSeparator))
	if width > 0 {
		rowWidth := lipgloss.Width(row)
		if rowWidth < width {
			row += statusbarBaseStyle.Render(strings.Repeat(" ", width-rowWidth))
		}
	}
	return row
}

func styleStatusValue(key string, value string) string {
	if key == "msg" {
		return statusbarMessageStyle.Render(value)
	}
	switch value {
	case "on":
		return statusbarOnStyle.Render(value)
	case "off":
		return statusbarOffStyle.Render(value)
	default:
		return statusbarBaseStyle.Render(value)
	}
}

func BuildStatusbar(model StatusModel, width int) string {
	fields := capPathFields(statusFields(model), statusPathValueMaxWidth)
	if line := joinStatusFields(fields); fits(line, width) {
		return line
	}

	fields = abbreviatePathFields(fields, width)
	if line := joinStatusFields(fields); fits(line, width) {
		return line
	}

	for _, key := range []string{"bm", "sel", "msg"} {
		fields = omitStatusField(fields, key)
		fields = abbreviatePathFields(fields, width)
		if line := joinStatusFields(fields); fits(line, width) {
			return line
		}
	}

	line := joinStatusFields(fields)
	if width >= 0 && len(line) > width {
		return line[:width]
	}
	return line
}

type statusField struct {
	key    string
	value  string
	isPath bool
}

func statusFields(model StatusModel) []statusField {
	bookmarks := append([]int(nil), model.Bookmarks...)
	sort.Ints(bookmarks)

	fields := []statusField{
		{key: "lines", value: fmt.Sprintf("%d/%d", model.CursorRawLine, model.TotalRawLines)},
		{key: "in", value: fmt.Sprintf("%s:%s", defaultInputMode(model.InputMode), defaultReadState(model.ReadState))},
		{key: "type", value: formatLogType(model.LogType, model.LogTypePending)},
		{key: "follow", value: onOff(model.Follow)},
		{key: "wrap", value: onOff(model.Wrap)},
		{key: "out", value: defaultOutputPath(model.OutputPath), isPath: true},
		{key: "bm", value: formatBookmarks(bookmarks)},
	}

	if count := statusSelectionCount(model); count > 0 {
		fields = append(fields, statusField{key: "sel", value: strconv.Itoa(count)})
	}
	if model.Message != "" {
		fields = append(fields, statusField{key: "msg", value: model.Message})
	}
	return fields
}

func statusSelectionCount(model StatusModel) int {
	if model.PickedCount > 0 {
		return model.PickedCount
	}
	if model.Selection == nil {
		return 0
	}

	start, end := model.Selection.Start, model.Selection.End
	if start > end {
		start, end = end, start
	}
	if start <= 0 || end <= 0 {
		return 0
	}
	return end - start + 1
}

func capPathFields(fields []statusField, maxWidth int) []statusField {
	next := cloneStatusFields(fields)
	for i := range next {
		if next[i].isPath {
			next[i].value = abbreviateMiddle(next[i].value, maxWidth)
		}
	}
	return next
}

func abbreviatePathFields(fields []statusField, width int) []statusField {
	if width < 0 {
		return fields
	}

	next := cloneStatusFields(fields)
	for maxPathWidth := longestPathValue(next); maxPathWidth >= 8; maxPathWidth-- {
		for i := range next {
			if next[i].isPath {
				next[i].value = abbreviateMiddle(next[i].value, maxPathWidth)
			}
		}
		if fits(joinStatusFields(next), width) {
			return next
		}
	}
	for i := range next {
		if next[i].isPath {
			next[i].value = abbreviateMiddle(next[i].value, 8)
		}
	}
	return next
}

func omitStatusField(fields []statusField, key string) []statusField {
	next := fields[:0]
	for _, field := range fields {
		if field.key != key {
			next = append(next, field)
		}
	}
	return next
}

func joinStatusFields(fields []statusField) string {
	parts := make([]string, len(fields))
	for i, field := range fields {
		parts[i] = field.key + ":" + field.value
	}
	return strings.Join(parts, statusSeparator)
}

func cloneStatusFields(fields []statusField) []statusField {
	return append([]statusField(nil), fields...)
}

func longestPathValue(fields []statusField) int {
	longest := 0
	for _, field := range fields {
		if field.isPath && len(field.value) > longest {
			longest = len(field.value)
		}
	}
	return longest
}

func abbreviateMiddle(value string, maxWidth int) string {
	if maxWidth < 1 || len(value) <= maxWidth {
		return value
	}
	if maxWidth <= 3 {
		return value[:maxWidth]
	}

	keep := maxWidth - 3
	left := keep / 2
	right := keep - left
	if right > len(value) {
		right = len(value)
	}
	return value[:left] + "..." + value[len(value)-right:]
}

func fits(value string, width int) bool {
	return width < 0 || len(value) <= width
}

func formatInputLine(label string, input TextInputModel) string {
	line := label + ":"
	if input.Editing {
		line += "> "
		line += formatEditingText(input.Text)
	} else {
		line += input.Text
	}
	if input.Error != "" {
		line += " ! " + input.Error
	}
	return line
}

func formatEditingText(text string) string {
	if text == "" {
		return "_"
	}
	return text + "_"
}

func truncateRunes(text string, width int) string {
	if width < 0 {
		return ""
	}
	count := 0
	for index := range text {
		if count == width {
			return text[:index]
		}
		count++
	}
	return text
}

func visualLogModels(logs []LogLineModel, height int, width int, wrap bool, horizontalOffset int) []LogLineModel {
	if height <= 0 {
		return nil
	}
	if horizontalOffset < 0 {
		horizontalOffset = 0
	}
	if width <= 0 {
		if len(logs) > height {
			logs = logs[:height]
		}
		return append([]LogLineModel(nil), logs...)
	}

	rows := make([]LogLineModel, 0, height)
	for _, log := range logs {
		if !wrap && len(rows) == height {
			break
		}
		prefixWidth := len(plainLogPrefix(log))
		bodyWidth := width - prefixWidth
		if bodyWidth < 0 {
			bodyWidth = 0
		}
		if !wrap {
			next := log
			start, end := horizontalTextWindow(log.Text, bodyWidth, horizontalOffset)
			next.Text = log.Text[start:end]
			next.Highlights = intersectHighlightRanges(log.Highlights, start, end)
			rows = append(rows, next)
			continue
		}

		if bodyWidth < 1 {
			bodyWidth = 1
		}
		segments := splitTextByWidth(log.Text, bodyWidth)
		if len(segments) == 0 {
			segments = []textSegment{{start: 0, end: 0}}
		}
		for index, segment := range segments {
			next := log
			next.Text = log.Text[segment.start:segment.end]
			next.Highlights = intersectHighlightRanges(log.Highlights, segment.start, segment.end)
			next.Continuation = index > 0
			next.PrefixWidth = prefixWidth
			if next.Continuation {
				next.Bookmark = 0
				next.RawLineNumber = 0
			}
			rows = append(rows, next)
		}
	}
	if wrap {
		return trimVisualRowsForCursor(rows, height)
	}
	return rows
}

func trimVisualRowsForCursor(rows []LogLineModel, height int) []LogLineModel {
	if height <= 0 {
		return nil
	}
	if len(rows) <= height {
		return append([]LogLineModel(nil), rows...)
	}
	cursorIndex := -1
	for index, row := range rows {
		if row.Cursor {
			cursorIndex = index
			break
		}
	}
	start := 0
	if cursorIndex >= height {
		start = cursorIndex - height + 1
	}
	if start+height > len(rows) {
		start = len(rows) - height
	}
	return append([]LogLineModel(nil), rows[start:start+height]...)
}

func MaxHorizontalOffset(logs []LogLineModel, width int) int {
	if width <= 0 {
		return 0
	}
	maxOffset := 0
	for _, log := range logs {
		prefixWidth := len(plainLogPrefix(log))
		bodyWidth := width - prefixWidth
		if bodyWidth < 0 {
			bodyWidth = 0
		}
		if offset := len(log.Text) - bodyWidth; offset > maxOffset {
			maxOffset = offset
		}
	}
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func horizontalTextWindow(text string, width int, offset int) (int, int) {
	if width < 0 {
		width = 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	end := offset + width
	if end > len(text) {
		end = len(text)
	}
	return offset, end
}

func formatLogLines(logs []LogLineModel, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := make([]string, 0, height)
	for _, log := range logs {
		if len(lines) == height {
			break
		}
		lines = append(lines, formatLogLine(log))
	}
	return lines
}

func formatLogLine(log LogLineModel) string {
	return plainLogPrefix(log) + log.Text
}

func plainLogPrefix(log LogLineModel) string {
	if log.Continuation {
		return strings.Repeat(" ", log.PrefixWidth)
	}
	bookmark := " "
	if log.Bookmark > 0 {
		bookmark = strconv.Itoa(log.Bookmark)
	}
	prefix := selectionPrefix(log.Selected)
	if prefix != "" {
		prefix += " "
	}
	return fmt.Sprintf("%d %s %s", log.RawLineNumber, bookmark, prefix)
}

type textSegment struct {
	start int
	end   int
}

func splitTextByWidth(text string, width int) []textSegment {
	if text == "" {
		return nil
	}
	if width <= 0 {
		return []textSegment{{start: 0, end: len(text)}}
	}
	segments := make([]textSegment, 0, (len(text)+width-1)/width)
	for start := 0; start < len(text); start += width {
		end := start + width
		if end > len(text) {
			end = len(text)
		}
		segments = append(segments, textSegment{start: start, end: end})
	}
	return segments
}

func truncateText(text string, width int) string {
	if width < 0 {
		width = 0
	}
	if width >= len(text) {
		return text
	}
	return text[:width]
}

func intersectHighlightRanges(ranges []usecase.HighlightRange, start int, end int) []usecase.HighlightRange {
	if len(ranges) == 0 || start >= end {
		return nil
	}
	intersections := make([]usecase.HighlightRange, 0, len(ranges))
	for _, r := range ranges {
		if r.End <= start || r.Start >= end {
			continue
		}
		if r.Start < start {
			r.Start = start
		}
		if r.End > end {
			r.End = end
		}
		intersections = append(intersections, usecase.HighlightRange{
			Start: r.Start - start,
			End:   r.End - start,
		})
	}
	return intersections
}

func formatLogHighlights(logs []LogLineModel, height int) [][]usecase.HighlightRange {
	if height <= 0 {
		return nil
	}
	highlights := make([][]usecase.HighlightRange, 0, height)
	for _, log := range logs {
		if len(highlights) == height {
			break
		}
		highlights = append(highlights, append([]usecase.HighlightRange(nil), log.Highlights...))
	}
	return highlights
}

func selectionPrefix(kind SelectionKind) string {
	switch kind {
	case SelectionKindRange:
		return ""
	case SelectionKindPicked:
		return ""
	default:
		return ""
	}
}

func helpModalHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return 0
	}
	lineCount := len(formatHelpModalLines())
	if frameHeight < lineCount {
		return frameHeight
	}
	return lineCount
}

func formatHelpModalLines() []string {
	return []string{
		"Help",
		"Movement: j/k, Up/Down, PageUp/PageDown, Home/End, n/p",
		"Follow mode: G jumps to end and follows; upward movement stops follow",
		"Filter input: : opens filter input",
		"Search input: / opens search input; n/p move matches",
		"Bookmarks: m toggles current line; 1-9 jump to visible bookmark",
		"Wrap: w toggles long log row wrapping",
		"Selection/copy: Shift+move selects range; Space picks; c copies",
		"Esc/F1 close",
	}
}

func defaultInputMode(mode InputMode) InputMode {
	if mode == "" {
		return InputModeFile
	}
	return mode
}

func defaultReadState(state ReadState) ReadState {
	if state == "" {
		return ReadStateEOF
	}
	return state
}

func formatLogType(logType usecase.LogType, pending bool) string {
	if pending || logType == "" || logType == usecase.LogTypeAuto {
		return string(usecase.LogTypeAuto) + "~"
	}
	return string(logType)
}

func defaultOutputPath(path string) string {
	if path == "" {
		return "-"
	}
	return path
}

func defaultPath(path string) string {
	if path == "" {
		return "-"
	}
	return path
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func formatBookmarks(bookmarks []int) string {
	if len(bookmarks) == 0 {
		return "-"
	}
	values := make([]string, len(bookmarks))
	for i, bookmark := range bookmarks {
		values[i] = strconv.Itoa(bookmark)
	}
	return strings.Join(values, ",")
}
