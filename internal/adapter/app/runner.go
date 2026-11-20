package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"logsee/internal/adapter/cli"
	clipboardadapter "logsee/internal/adapter/clipboard"
	"logsee/internal/adapter/config"
	"logsee/internal/adapter/filesystem"
	"logsee/internal/adapter/tui"
	"logsee/internal/port"
	"logsee/internal/usecase"
)

const (
	defaultFrameWidth    = 120
	defaultFrameHeight   = 24
	unboundedRecordLimit = 0
	asyncRecordLimit     = -1
)

type RunOptions struct {
	Width        int
	Height       int
	HomeDir      string
	WorkDir      string
	Now          time.Time
	Interactive  bool
	KeyInput     io.Reader
	UseBubbleTea bool
	Clipboard    port.ClipboardWriter
}

func Run(ctx context.Context, options cli.Options, stdin io.Reader, stdout io.Writer, runOptions RunOptions) (runErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		return fmt.Errorf("stdout writer is required")
	}

	width := runOptions.Width
	if width <= 0 {
		width = defaultFrameWidth
	}
	height := runOptions.Height
	if height <= 0 {
		height = defaultFrameHeight
	}

	workDir, err := resolveWorkDir(runOptions.WorkDir)
	if err != nil {
		return err
	}
	homeDir, err := resolveHomeDir(runOptions.HomeDir)
	if err != nil {
		return err
	}

	request := usecase.InputRequest{
		InputPath:  options.InputPath,
		OutPath:    options.OutPath,
		IgnoreCase: options.IgnoreCase,
		WorkDir:    workDir,
		Now:        runOptions.Now,
	}

	session, sourcePath, closeInput, err := openInputSession(request)
	if err != nil {
		return err
	}
	defer closeInput()

	configured, err := sessionLogTypeConfig(options, homeDir)
	if err != nil {
		return err
	}
	session.LogTypeConfig = configured
	var stream *stdioStream
	if session.Mode == usecase.InputModeStdio {
		if runOptions.Interactive {
			stream = startStdioStream(ctx, session, stdin)
			defer func() {
				if err := stream.stop(); err != nil && !errors.Is(err, context.Canceled) {
					runErr = errors.Join(runErr, err)
				}
			}()
		} else if err := session.ConsumeStdio(ctx, stdin, nil); err != nil {
			return err
		}
	}

	logType, err := session.DetectLogType(ctx, configured)
	if err != nil {
		return err
	}

	if runOptions.Interactive {
		keyInput := runOptions.KeyInput
		if keyInput == nil && session.Mode != usecase.InputModeStdio {
			keyInput = stdin
		}
		clipboardWriter := runOptions.Clipboard
		if clipboardWriter == nil {
			clipboardWriter = clipboardadapter.DefaultWriter()
		}
		if runOptions.UseBubbleTea {
			return runBubbleTeaLoop(ctx, session, sourcePath, logType, width, height, keyInput, stdout, stream, clipboardWriter, homeDir)
		}
		return runInteractiveLoop(ctx, session, sourcePath, logType, width, height, keyInput, stdout, stream, clipboardWriter, homeDir)
	}

	frame, err := buildInitialFrame(ctx, session, sourcePath, logType, width, height, homeDir)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, tui.FrameText(frame)); err != nil {
		return fmt.Errorf("write TUI frame: %w", err)
	}
	return nil
}

func openInputSession(request usecase.InputRequest) (usecase.InputSession, string, func(), error) {
	if request.InputPath == "" || request.InputPath == "-" {
		outPath := usecase.ResolveStdioOutPath(request)
		appendFile, err := filesystem.NewAppendFile(outPath)
		if err != nil {
			return usecase.InputSession{}, "", func() {}, err
		}
		closeInput := func() {
			_ = appendFile.Close()
		}
		var session usecase.InputSession
		source := filesystem.NewFileSource(outPath).WithLimit(func() int64 { return session.Progress().PublishedBytes })
		request.OutPath = outPath
		session, err = usecase.NewInputSession(request, usecase.InputPorts{
			StdioSink:  appendFile,
			FileSource: source,
		})
		if err != nil {
			closeInput()
			return usecase.InputSession{}, "", func() {}, err
		}
		return session, outPath, closeInput, nil
	}

	if file, err := os.Open(request.InputPath); err != nil {
		return usecase.InputSession{}, "", func() {}, fmt.Errorf("open input file %q: %w", request.InputPath, err)
	} else {
		_ = file.Close()
	}
	source := filesystem.NewFileSource(request.InputPath)
	session, err := usecase.NewInputSession(request, usecase.InputPorts{
		FileSource: source,
	})
	if err != nil {
		return usecase.InputSession{}, "", func() {}, err
	}
	return session, request.InputPath, func() {}, nil
}

type stdioStream struct {
	refresh <-chan struct{}
	done    <-chan error
	cancel  context.CancelFunc
	stop    func() error
}

func startStdioStream(ctx context.Context, session usecase.InputSession, stdin io.Reader) *stdioStream {
	streamCtx, cancel := context.WithCancel(ctx)
	refresh := make(chan struct{}, 1)
	done := make(chan error, 1)
	observer := sourceRefreshObserver{refresh: refresh}
	exited := make(chan struct{})
	var consumeErr error
	go func() {
		defer close(refresh)
		defer close(exited)
		consumeErr = session.ConsumeStdio(streamCtx, stdin, observer)
		done <- consumeErr
		close(done)
	}()
	return &stdioStream{
		refresh: refresh,
		done:    done,
		cancel:  cancel,
		stop:    func() error { cancel(); <-exited; return consumeErr },
	}
}

type sourceRefreshObserver struct {
	refresh chan<- struct{}
}

func (o sourceRefreshObserver) SourceLineAvailable(string) {
	select {
	case o.refresh <- struct{}{}:
	default:
	}
}

func sessionLogTypeConfig(options cli.Options, homeDir string) (usecase.LogTypeConfig, error) {
	configPath := config.ResolveConfigPath(options.ConfigPath, homeDir)
	if options.ConfigPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return usecase.LogTypeConfig{}, err
		}
	}
	configured, err := config.LoadLogTypeConfig(configPath)
	if err != nil {
		return usecase.LogTypeConfig{}, err
	}
	return usecase.ResolveLogTypeConfig(usecase.LogType(options.LogType), configured)
}
func resolveSessionLogType(ctx context.Context, session usecase.InputSession, options cli.Options, homeDir string) (usecase.LogType, error) {
	configured, err := sessionLogTypeConfig(options, homeDir)
	if err != nil {
		return "", err
	}
	return session.DetectLogType(ctx, configured)
}

func buildInitialFrame(ctx context.Context, session usecase.InputSession, sourcePath string, logType usecase.LogType, width, height int, homeDir string) (tui.Frame, error) {
	state, err := newLoopState(ctx, session, sourcePath, logType, width, height, height-3, homeDir)
	if err != nil {
		return tui.Frame{}, err
	}
	return state.renderFrame(), nil
}

func usesIndexedSourceWindow(session usecase.InputSession, recordLimit int) bool {
	if recordLimit != unboundedRecordLimit {
		return false
	}
	return session.Mode == usecase.InputModeFile || session.Mode == usecase.InputModeStdio
}

func newLoopState(ctx context.Context, session usecase.InputSession, sourcePath string, logType usecase.LogType, width, height, recordLimit int, homeDir string) (*loopState, error) {
	listHeight := height - 3
	if listHeight < 0 {
		listHeight = 0
	}
	if recordLimit > 0 && recordLimit < listHeight {
		recordLimit = listHeight
	}

	filter, err := usecase.CompileFilter("", usecase.FilterOptions{LogType: logType})
	if err != nil {
		return nil, err
	}

	var lineIndex *fileLineIndex
	var rawLogs []usecase.RawLogLine
	var outputRecords []usecase.OutputLogRecord
	var totalRawLines int
	var outputCount int
	if recordLimit == asyncRecordLimit {
		lineIndex = &fileLineIndex{}
	} else if usesIndexedSourceWindow(session, recordLimit) {
		size, err := readableSize(session, sourcePath)
		if err != nil {
			return nil, err
		}
		index, err := buildFileLineIndexLimit(ctx, sourcePath, size)
		if err != nil {
			return nil, err
		}
		lineIndex = &index
		totalRawLines = index.totalLines()
		outputCount = totalRawLines
		rawLogs, err = readRawLogWindow(ctx, sourcePath, index, 0, listHeight)
		if err != nil {
			return nil, err
		}
		outputRecords = usecase.ApplyFilterToRawLogs(rawLogs, filter)
	} else {
		var err error
		rawLogs, totalRawLines, err = readInitialRawLogs(ctx, sourcePath, recordLimit)
		if err != nil {
			return nil, err
		}
		outputRecords = usecase.ApplyFilterToRawLogs(rawLogs, filter)
		outputCount = len(outputRecords)
	}

	nav, err := usecase.NewNavigationState(usecase.NavigationOptions{
		OutputCount:    outputCount,
		ViewportHeight: listHeight,
	})
	if err != nil {
		return nil, err
	}
	state := &loopState{
		background:    recordLimit == asyncRecordLimit,
		counting:      recordLimit == asyncRecordLimit,
		width:         width,
		height:        height,
		listHeight:    listHeight,
		homeDir:       homeDir,
		sourcePath:    sourcePath,
		recordLimit:   recordLimit,
		session:       session,
		logType:       logType,
		totalRawLines: totalRawLines,
		rawLogs:       rawLogs,
		records:       outputRecords,
		filter:        filter,
		lineIndex:     lineIndex,
		readState:     tui.ReadStateEOF,
		navigation:    nav,
		bookmarks:     usecase.NewBookmarkState(),
		selection:     usecase.NewSelectionState(),
	}
	state.sourceInfo, _ = os.Stat(sourcePath)
	if err := state.loadPersistedInputHistory(); err != nil {
		return nil, err
	}
	return state, nil
}

type loopFocus int

const (
	loopFocusLogList loopFocus = iota
	loopFocusFilterInput
	loopFocusSearchInput
)

type loopMode int

const (
	loopModeSelection loopMode = iota
	loopModeSearch
)

type loopState struct {
	pendingRangeActive                        bool
	pendingSelectionOps                       []selectionOperation
	intentRevision                            uint64
	pendingAnchorOutput                       int
	background, counting                      bool
	pendingCopy, pendingSelect, pendingSearch bool
	pendingAnchor                             int
	pendingDirection                          usecase.SearchDirection

	width      int
	height     int
	listHeight int

	session       usecase.InputSession
	logType       usecase.LogType
	sourcePath    string
	recordLimit   int
	totalRawLines int

	rawLogs []usecase.RawLogLine
	records []usecase.OutputLogRecord
	filter  usecase.CompiledFilter

	sourceInfo             os.FileInfo
	lineIndex              *fileLineIndex
	windowStartOutputIndex int
	filteredOutputCount    int
	filteredSourceSize     int64
	filterRevision         uint64

	filterText        string
	filterEditingText string
	filterCursor      int
	filterError       string
	searchText        string
	searchEditingText string
	searchCursor      int
	focus             loopFocus
	modeStack         []loopMode

	navigation          *usecase.NavigationState
	bookmarks           *usecase.BookmarkState
	selection           *usecase.SelectionState
	clipboard           port.ClipboardWriter
	homeDir             string
	inputHistoryPath    string
	inputHistory        usecase.InputHistorySnapshot
	historyPickerOpen   bool
	historyPickerFilter bool
	historyPickerIndex  int
	historyPickerScroll int
	helpOpen            bool
	helpScrollOffset    int
	wrap                bool
	wrapOffset          int
	horizontalOffset    int

	readState               tui.ReadState
	runtimeMessage          string
	runtimeMessageTransient bool
	runtimeMessageID        int
}

func (s *loopState) renderFrame() tui.Frame {
	logs := s.visibleLogLines()
	cursorRawLine := s.cursorRawLine()

	return tui.RenderFrame(tui.RenderModel{
		Width:                     s.width,
		Height:                    s.height,
		Filter:                    s.filterInputModel(),
		Search:                    s.searchInputModel(),
		Logs:                      logs,
		HelpOpen:                  s.helpOpen,
		HelpScrollOffset:          s.helpScrollOffset,
		HistoryPickerOpen:         s.historyPickerOpen,
		HistoryPickerTitle:        s.historyPickerTitle(),
		HistoryPickerItems:        s.historyPickerItems(),
		HistoryPickerIndex:        s.historyPickerIndex,
		HistoryPickerScrollOffset: s.historyPickerScroll,
		Wrap:                      s.wrap,
		HorizontalOffset:          s.horizontalOffset,
		Status: tui.StatusModel{
			CursorRawLine: cursorRawLine,
			TotalRawLines: s.totalRawLines,
			Counting:      s.counting,
			InputMode:     tuiInputMode(s.session.Mode),
			ReadState:     s.readState,
			LogType:       s.logType,
			Follow:        s.navigation.Follow(),
			Wrap:          s.wrap,
			OutputPath:    s.session.OutPath,
			SOTPath:       s.session.SOTPath,
			FilterText:    s.filterText,
			SearchText:    s.searchText,
			Bookmarks:     s.bookmarks.Slots(),
			Selection:     s.statusSelection(),
			PickedCount:   s.selection.PickedCount(),
			Message:       s.runtimeMessage,
		},
	})
}

func (s *loopState) filterInputModel() tui.TextInputModel {
	if s.focus == loopFocusFilterInput {
		return tui.TextInputModel{
			Text:      s.filterEditingText,
			Error:     s.filterError,
			Editing:   true,
			CursorPos: s.filterCursor,
		}
	}
	return tui.TextInputModel{Text: s.filterText}
}

func (s *loopState) searchInputModel() tui.TextInputModel {
	if s.focus == loopFocusSearchInput {
		return tui.TextInputModel{
			Text:      s.searchEditingText,
			Editing:   true,
			CursorPos: s.searchCursor,
		}
	}
	return tui.TextInputModel{Text: s.searchText}
}

func (s *loopState) visibleLogLines() []tui.LogLineModel {
	if s.listHeight <= 0 || len(s.records) == 0 {
		return nil
	}

	start := s.navigation.ScrollOffset()
	if start < 0 {
		start = 0
	}
	localStart := start
	if s.recordsWindowActive() {
		localStart = start - s.windowStartOutputIndex
	}
	if localStart < 0 {
		localStart = 0
	}
	if localStart > len(s.records) {
		localStart = len(s.records)
	}
	end := localStart + s.listHeight
	if end > len(s.records) {
		end = len(s.records)
	}

	logs := make([]tui.LogLineModel, 0, end-localStart)
	matcher := usecase.NewSearchMatcher(s.searchText)
	cursorOutputIndex := s.navigation.CursorOutputIndex()
	for index, record := range s.records[localStart:end] {
		outputIndex := localStart + index
		if s.recordsWindowActive() {
			outputIndex += s.windowStartOutputIndex
		}
		bookmark, _ := s.bookmarks.SlotForRawLine(record.RawLineNumber)
		logs = append(logs, tui.LogLineModel{
			RawLineNumber: record.RawLineNumber,
			Bookmark:      bookmark,
			Text:          record.Text,
			Highlights:    matcher.HighlightRanges(record.Text),
			Selected:      s.selectionKind(record.RawLineNumber),
			Cursor:        outputIndex == cursorOutputIndex,
			WrapOffset:    s.wrapOffset,
		})
	}
	return logs
}

func (s *loopState) cursorRawLine() int {
	if s.fileWindowActive() {
		if index, ok := s.recordIndexForOutputIndex(s.navigation.CursorOutputIndex()); ok {
			return s.records[index].RawLineNumber
		}
		if s.totalRawLines == 0 {
			return 0
		}
		return s.navigation.CursorOutputIndex() + 1
	}
	if len(s.records) == 0 {
		return 0
	}
	if index, ok := s.recordIndexForOutputIndex(s.navigation.CursorOutputIndex()); ok {
		return s.records[index].RawLineNumber
	}
	return 0
}

func (s *loopState) applyFilterEditingText() bool {
	filter, err := usecase.CompileFilter(s.filterEditingText, s.filterOptions())
	if err != nil {
		s.filterError = err.Error()
		return true
	}

	var filteredWindow filteredOutputWindow
	filterText := s.filterEditingText
	filterActive := strings.TrimSpace(filterText) != ""
	if s.lineIndex != nil && filterActive && !s.background {
		filteredWindow, err = scanIndexedFilter(context.Background(), *s.lineIndex, filter, 0, usecase.DefaultOutputLogCacheCapacity, s.filterRevision+1)
		if err != nil {
			s.filterError = err.Error()
			return true
		}
	}

	s.filterText = s.filterEditingText
	s.filterError = ""
	s.filter = filter
	s.filterRevision++
	s.counting = s.background
	s.clearSelectionMode()
	if s.fileWindowActive() {
		s.rawLogs = nil
		s.records = nil
		s.windowStartOutputIndex = 0
		s.filteredOutputCount = 0
		s.filteredSourceSize = 0
		s.resetNavigation()
	} else if s.filteredWindowActive() {
		s.totalRawLines = filteredWindow.totalRawLines
		s.rawLogs = nil
		s.records = filteredWindow.records
		s.windowStartOutputIndex = filteredWindow.startOutputIndex
		s.filteredOutputCount = filteredWindow.totalMatches
		s.filteredSourceSize = filteredWindow.fileSize
		s.resetNavigation()
	} else {
		rawLogs := s.rawLogs
		s.records = usecase.ApplyFilterToRawLogs(rawLogs, filter)
		s.filteredOutputCount = 0
		s.filteredSourceSize = 0
		s.resetNavigation()
	}
	s.focus = loopFocusLogList
	s.recordFilterHistory()
	return true
}

func (s *loopState) statusSelection() *tui.LineRange {
	if s.selection == nil {
		return nil
	}
	rawRange, ok := s.selection.Range()
	if !ok {
		return nil
	}
	return &tui.LineRange{Start: rawRange.Start, End: rawRange.End}
}

func (s *loopState) selectionKind(rawLineNumber int) tui.SelectionKind {
	if s.selection == nil {
		return tui.SelectionKindNone
	}
	if s.selection.IsRangeSelected(rawLineNumber) {
		return tui.SelectionKindRange
	}
	if s.selection.IsPicked(rawLineNumber) {
		return tui.SelectionKindPicked
	}
	return tui.SelectionKindNone
}

func (s *loopState) applySearchEditingText() bool {
	s.intentRevision++
	s.pendingSearch = false
	s.searchText = s.searchEditingText
	if strings.TrimSpace(s.searchText) == "" {
		s.searchText = ""
		s.searchEditingText = ""
		s.removeMode(loopModeSearch)
	} else {
		s.pushMode(loopModeSearch)
	}
	s.focus = loopFocusLogList
	s.recordSearchHistory()
	return true
}

func (s *loopState) setTransientRuntimeMessage(message string) {
	s.runtimeMessage = message
	s.runtimeMessageTransient = message != ""
	s.runtimeMessageID++
}

func (s *loopState) setPersistentRuntimeMessage(message string) {
	s.runtimeMessage = message
	s.runtimeMessageTransient = false
	s.runtimeMessageID++
}

func (s *loopState) clearTransientRuntimeMessage(id int) bool {
	if !s.runtimeMessageTransient || s.runtimeMessageID != id {
		return false
	}
	s.runtimeMessage = ""
	s.runtimeMessageTransient = false
	return true
}

func (s *loopState) resetNavigation() {
	nav, err := usecase.NewNavigationState(usecase.NavigationOptions{
		OutputCount:    s.outputCount(),
		ViewportHeight: s.listHeight,
	})
	if err == nil {
		s.navigation = nav
	}
}

func (s *loopState) filterOptions() usecase.FilterOptions {
	return usecase.FilterOptions{
		LogType:    s.logType,
		IgnoreCase: s.session.IgnoreCase,
	}
}

func (s *loopState) resize(width, height int) {
	s.wrapOffset = 0
	if width > 0 {
		s.width = width
	}
	if height > 0 {
		s.height = height
	}
	listHeight := s.height - 3
	if listHeight < 0 {
		listHeight = 0
	}
	if listHeight == s.listHeight {
		return
	}
	s.listHeight = listHeight
	nav, err := usecase.NewNavigationState(usecase.NavigationOptions{
		OutputCount:       s.outputCount(),
		ViewportHeight:    s.listHeight,
		CursorOutputIndex: s.navigation.CursorOutputIndex(),
		ScrollOffset:      s.navigation.ScrollOffset(),
		Follow:            s.navigation.Follow(),
	})
	if err == nil {
		s.navigation = nav
	}
}

func (s *loopState) refreshFromSOT(ctx context.Context) (bool, error) {
	if s.background {
		return false, nil
	}
	if s.logType == usecase.LogTypeAuto {
		detected, err := s.session.DetectLogType(ctx, s.session.LogTypeConfig)
		if err != nil {
			return false, err
		}
		if detected != usecase.LogTypeAuto {
			s.logType = detected
			s.filter, err = usecase.CompileFilter(s.filterText, s.filterOptions())
			if err != nil {
				return false, err
			}
			s.filteredSourceSize = -1
			s.filterRevision++
		}
	}
	info, err := os.Stat(s.sourcePath)
	if err != nil {
		return false, fmt.Errorf("refresh SOT: stat rendered SOT: %w", err)
	}
	replaced := s.sourceInfo != nil && (!os.SameFile(s.sourceInfo, info) || info.Size() < s.sourceInfo.Size() || info.Size() == s.sourceInfo.Size() && !info.ModTime().Equal(s.sourceInfo.ModTime()))
	s.sourceInfo = info
	if replaced && s.lineIndex != nil {
		index, err := buildFileLineIndex(ctx, s.sourcePath)
		if err != nil {
			return false, err
		}
		s.lineIndex = &index
		s.filteredSourceSize = -1
		s.clearSelectionMode()
		s.bookmarks = usecase.NewBookmarkState()
		s.totalRawLines = index.totalLines()
		s.resetNavigation()
	}

	if s.fileWindowActive() {
		changed, err := s.refreshFileWindow(ctx)
		if err != nil {
			return false, fmt.Errorf("refresh SOT: %w", err)
		}
		return changed, nil
	}
	if s.filteredWindowActive() {
		changed, err := s.refreshFilteredWindow(ctx)
		if err != nil {
			return false, fmt.Errorf("refresh SOT: %w", err)
		}
		return changed, nil
	}

	rawLogs, totalRawLines, err := readInitialRawLogs(ctx, s.sourcePath, s.recordLimit)
	if err != nil {
		return false, fmt.Errorf("refresh SOT: %w", err)
	}

	records := usecase.ApplyFilterToRawLogs(rawLogs, s.filter)
	changed := totalRawLines != s.totalRawLines ||
		!sameRawLogs(rawLogs, s.rawLogs) ||
		!sameOutputRecords(records, s.records)

	s.totalRawLines = totalRawLines
	s.rawLogs = rawLogs
	s.records = records

	oldCursor := s.navigation.CursorOutputIndex()
	oldScroll := s.navigation.ScrollOffset()
	oldFollow := s.navigation.Follow()
	s.navigation.SetOutputCount(len(records))
	if oldCursor != s.navigation.CursorOutputIndex() ||
		oldScroll != s.navigation.ScrollOffset() ||
		oldFollow != s.navigation.Follow() {
		changed = true
	}
	return changed, nil
}

func (s *loopState) fileWindowActive() bool {
	return s.lineIndex != nil && strings.TrimSpace(s.filterText) == ""
}

func (s *loopState) filteredWindowActive() bool {
	return s.lineIndex != nil && strings.TrimSpace(s.filterText) != ""
}

func (s *loopState) recordsWindowActive() bool {
	return s.fileWindowActive() || s.filteredWindowActive()
}

func (s *loopState) outputCount() int {
	if s.fileWindowActive() && s.lineIndex != nil {
		return s.lineIndex.totalLines()
	}
	if s.filteredWindowActive() {
		return s.filteredOutputCount
	}
	return len(s.records)
}

func (s *loopState) recordIndexForOutputIndex(outputIndex int) (int, bool) {
	index := outputIndex
	if s.recordsWindowActive() {
		index = outputIndex - s.windowStartOutputIndex
	}
	if index < 0 || index >= len(s.records) {
		return 0, false
	}
	return index, true
}

func (s *loopState) filteredViewportCached() bool {
	if !s.filteredWindowActive() {
		return false
	}
	if s.filteredOutputCount == 0 {
		return len(s.records) == 0
	}

	start := s.navigation.ScrollOffset()
	end := start + s.listHeight
	if s.listHeight <= 0 {
		end = start
	}
	if end > s.filteredOutputCount {
		end = s.filteredOutputCount
	}

	cachedStart := s.windowStartOutputIndex
	cachedEnd := s.windowStartOutputIndex + len(s.records)
	return start >= cachedStart && end <= cachedEnd
}

func (s *loopState) refreshFileWindow(ctx context.Context) (bool, error) {
	changed := false
	size, err := readableSize(s.session, s.sourcePath)
	if err != nil {
		return false, fmt.Errorf("refresh file window: %w", err)
	}
	if s.lineIndex == nil || size != s.lineIndex.size {
		var index fileLineIndex
		var err error
		switch {
		case s.lineIndex == nil:
			index, err = buildFileLineIndexLimit(ctx, s.sourcePath, size)
		case size < s.lineIndex.size:
			index, err = buildFileLineIndexLimit(ctx, s.sourcePath, size)
		default:
			index, err = extendFileLineIndexLimit(ctx, *s.lineIndex, size)
		}
		if err != nil {
			return false, fmt.Errorf("refresh file index: %w", err)
		}
		s.lineIndex = &index
		if s.totalRawLines != index.totalLines() {
			changed = true
		}
		s.totalRawLines = index.totalLines()
		oldCursor := s.navigation.CursorOutputIndex()
		oldScroll := s.navigation.ScrollOffset()
		oldFollow := s.navigation.Follow()
		s.navigation.SetOutputCount(index.totalLines())
		if oldCursor != s.navigation.CursorOutputIndex() ||
			oldScroll != s.navigation.ScrollOffset() ||
			oldFollow != s.navigation.Follow() {
			changed = true
		}
	}

	start := s.navigation.ScrollOffset()
	rawLogs, err := readRawLogWindow(ctx, s.sourcePath, *s.lineIndex, start, s.listHeight)
	if err != nil {
		return false, fmt.Errorf("refresh file window: %w", err)
	}
	records := usecase.ApplyFilterToRawLogs(rawLogs, s.filter)
	if start != s.windowStartOutputIndex ||
		!sameRawLogs(rawLogs, s.rawLogs) ||
		!sameOutputRecords(records, s.records) {
		changed = true
	}
	s.windowStartOutputIndex = start
	s.rawLogs = rawLogs
	s.records = records
	return changed, nil
}

func (s *loopState) refreshFilteredWindow(ctx context.Context) (bool, error) {
	if s.filteredViewportCached() {
		size, err := readableSize(s.session, s.sourcePath)
		if err != nil {
			return false, fmt.Errorf("refresh filtered window: %w", err)
		}
		if size == s.filteredSourceSize {
			return false, nil
		}
	}

	size, err := readableSize(s.session, s.sourcePath)
	if err != nil {
		return false, err
	}
	if size != s.lineIndex.size {
		index, err := extendFileLineIndexLimit(ctx, *s.lineIndex, size)
		if err != nil {
			return false, err
		}
		s.lineIndex = &index
	}
	start := s.navigation.ScrollOffset()
	window, err := scanIndexedFilter(ctx, *s.lineIndex, s.filter, start, usecase.DefaultOutputLogCacheCapacity, s.filterRevision)
	if err != nil {
		return false, fmt.Errorf("refresh filtered window: %w", err)
	}

	changed := window.totalRawLines != s.totalRawLines ||
		window.totalMatches != s.filteredOutputCount ||
		start != s.windowStartOutputIndex ||
		len(s.rawLogs) != 0 ||
		!sameOutputRecords(window.records, s.records)

	s.totalRawLines = window.totalRawLines
	s.filteredOutputCount = window.totalMatches
	s.filteredSourceSize = window.fileSize

	oldCursor := s.navigation.CursorOutputIndex()
	oldScroll := s.navigation.ScrollOffset()
	oldFollow := s.navigation.Follow()
	s.navigation.SetOutputCount(window.totalMatches)
	if oldCursor != s.navigation.CursorOutputIndex() ||
		oldScroll != s.navigation.ScrollOffset() ||
		oldFollow != s.navigation.Follow() {
		changed = true
	}

	if nextStart := s.navigation.ScrollOffset(); nextStart != start {
		window, err = scanIndexedFilter(ctx, *s.lineIndex, s.filter, nextStart, usecase.DefaultOutputLogCacheCapacity, s.filterRevision)
		if err != nil {
			return false, fmt.Errorf("refresh filtered window: %w", err)
		}
		if window.totalRawLines != s.totalRawLines ||
			window.totalMatches != s.filteredOutputCount ||
			!sameOutputRecords(window.records, s.records) {
			changed = true
		}
		s.totalRawLines = window.totalRawLines
		s.filteredOutputCount = window.totalMatches
		s.filteredSourceSize = window.fileSize
	}

	s.windowStartOutputIndex = window.startOutputIndex
	s.rawLogs = nil
	s.records = window.records
	return changed, nil
}

func sameRawLogs(left, right []usecase.RawLogLine) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sameOutputRecords(left, right []usecase.OutputLogRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func readInitialRawLogs(ctx context.Context, path string, limit int) ([]usecase.RawLogLine, int, error) {
	index, err := buildFileLineIndex(ctx, path)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = index.totalLines()
	}
	rows, err := readRawLogWindow(ctx, path, index, 0, limit)
	return rows, index.totalLines(), err
}
func readableSize(session usecase.InputSession, path string) (int64, error) {
	if limit, ok := session.PublishedLimit(); ok {
		return limit, nil
	}
	return sourceFileSize(path)
}

func tuiInputMode(mode usecase.InputMode) tui.InputMode {
	switch mode {
	case usecase.InputModeStdio:
		return tui.InputModeStdio
	case usecase.InputModeFile:
		return tui.InputModeFile
	default:
		return ""
	}
}

func resolveHomeDir(homeDir string) (string, error) {
	if strings.TrimSpace(homeDir) != "" {
		return homeDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return home, nil
}

func resolveWorkDir(workDir string) (string, error) {
	if strings.TrimSpace(workDir) != "" {
		return workDir, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return wd, nil
}
