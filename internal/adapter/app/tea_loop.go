package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"logsee/internal/adapter/tui"
	"logsee/internal/port"
	"logsee/internal/usecase"
)

const runtimeMessageTTL = 1500 * time.Millisecond

type streamRefreshMsg struct{}
type fileRefreshMsg struct{}

func waitFileRefresh() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return fileRefreshMsg{} })
}

type streamDoneMsg struct {
	err error
}

type runtimeMessageExpiredMsg struct {
	id int
}

type teaLoopModel struct {
	ctx               context.Context
	state             *loopState
	stream            *stdioStream
	noKeyInput        bool
	err               error
	jobs              *sourceJobs
	requestID         uint64
	busy, dirty       bool
	appliedGeneration uint64
	appliedBytes      int64
}

func runBubbleTeaLoop(ctx context.Context, session usecase.InputSession, sourcePath string, logType usecase.LogType, width, height int, keyInput io.Reader, output io.Writer, stream *stdioStream, clipboardWriter port.ClipboardWriter, homeDir string) error {
	state, err := newLoopState(ctx, session, sourcePath, logType, width, height, asyncRecordLimit, homeDir)
	if err != nil {
		return err
	}
	state.clipboard = clipboardWriter
	if stream != nil {
		state.readState = tui.ReadStateRead
		defer stream.cancel()
	}

	jobs := newSourceJobs(ctx, sourcePath)
	defer jobs.Close()
	model := teaLoopModel{
		jobs:       jobs,
		ctx:        ctx,
		state:      state,
		stream:     stream,
		noKeyInput: keyInput == nil,
	}
	model.submitSource()
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(keyInput),
		tea.WithOutput(output),
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
	)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	if final, ok := finalModel.(teaLoopModel); ok && final.err != nil {
		return final.err
	}
	return nil
}

func (m teaLoopModel) Init() tea.Cmd {
	if m.jobs != nil {
		cmds := []tea.Cmd{waitSourceResult(m.jobs.results), waitFileRefresh()}
		if m.stream != nil {
			cmds = append(cmds, waitStreamRefresh(m.stream.refresh), waitStreamDone(m.stream.done))
		}
		return tea.Batch(cmds...)
	}
	if m.stream == nil {
		return waitFileRefresh()
	}
	return tea.Batch(
		waitStreamRefresh(m.stream.refresh),
		waitStreamDone(m.stream.done),
	)
}

func (m teaLoopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.jobs != nil {
		return m.updateAsync(msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.state.resize(msg.Width, msg.Height)
		_, err := m.state.refreshFromSOT(m.ctx)
		if err != nil {
			m.err = err
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyMsg:
		input := teaKeyToLoopInput(msg)
		beforeMessageID := m.state.runtimeMessageID
		inputRedraw, quit := m.state.handleInput(m.ctx, input)
		redraw, err := m.state.refreshFromSOT(m.ctx)
		if err != nil {
			m.err = fmt.Errorf("refresh SOT: %w", err)
			m.state.setPersistentRuntimeMessage(m.err.Error())
			return m, tea.Quit
		}
		if quit {
			return m, tea.Quit
		}
		expiryCmd := m.runtimeMessageExpiryCmd(beforeMessageID)
		if inputRedraw || redraw {
			return m, expiryCmd
		}
		return m, expiryCmd
	case fileRefreshMsg:
		_, err := m.state.refreshFromSOT(m.ctx)
		if err != nil {
			m.err = err
			return m, tea.Quit
		}
		return m, waitFileRefresh()
	case streamRefreshMsg:
		redraw, err := m.state.refreshFromSOT(m.ctx)
		if err != nil {
			m.err = fmt.Errorf("refresh SOT: %w", err)
			m.state.setPersistentRuntimeMessage(m.err.Error())
			return m, tea.Quit
		}
		if redraw {
			return m, waitStreamRefresh(m.stream.refresh)
		}
		return m, waitStreamRefresh(m.stream.refresh)
	case streamDoneMsg:
		m.state.readState = tui.ReadStateEOF
		if msg.err != nil {
			m.err = fmt.Errorf("stream stdio: %w", msg.err)
			m.state.setPersistentRuntimeMessage(m.err.Error())
			return m, tea.Quit
		}
		_, err := m.state.refreshFromSOT(m.ctx)
		if err != nil {
			m.err = fmt.Errorf("refresh SOT: %w", err)
			m.state.setPersistentRuntimeMessage(m.err.Error())
			return m, tea.Quit
		}
		if m.noKeyInput {
			return m, tea.Quit
		}
		return m, nil
	case runtimeMessageExpiredMsg:
		m.state.clearTransientRuntimeMessage(msg.id)
		return m, nil
	default:
		return m, nil
	}
}

func (m teaLoopModel) runtimeMessageExpiryCmd(beforeMessageID int) tea.Cmd {
	if !m.state.runtimeMessageTransient || m.state.runtimeMessageID == beforeMessageID {
		return nil
	}
	return waitRuntimeMessageExpiry(m.state.runtimeMessageID)
}

func (m teaLoopModel) View() string {
	return strings.TrimSuffix(tui.StyledFrameText(m.state.renderFrame()), "\n")
}

func waitStreamRefresh(refresh <-chan struct{}) tea.Cmd {
	if refresh == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-refresh
		if !ok {
			return nil
		}
		return streamRefreshMsg{}
	}
}

func waitStreamDone(done <-chan error) tea.Cmd {
	if done == nil {
		return nil
	}
	return func() tea.Msg {
		err, ok := <-done
		if !ok {
			return streamDoneMsg{}
		}
		return streamDoneMsg{err: err}
	}
}

func waitRuntimeMessageExpiry(id int) tea.Cmd {
	return tea.Tick(runtimeMessageTTL, func(time.Time) tea.Msg {
		return runtimeMessageExpiredMsg{id: id}
	})
}

func teaKeyToLoopInput(msg tea.KeyMsg) loopInput {
	switch msg.Type {
	case tea.KeyCtrlC:
		return eventLoopInput(loopEventCtrlC)
	case tea.KeyEnter:
		return eventLoopInput(loopEventEnter)
	case tea.KeyEsc:
		return eventLoopInput(loopEventEsc)
	case tea.KeyBackspace:
		return eventLoopInput(loopEventBackspace)
	case tea.KeyUp:
		return eventLoopInput(loopEventUp)
	case tea.KeyDown:
		return eventLoopInput(loopEventDown)
	case tea.KeyShiftUp:
		return eventLoopInput(loopEventShiftUp)
	case tea.KeyShiftHome:
		return eventLoopInput(loopEventShiftHome)
	case tea.KeyShiftEnd:
		return eventLoopInput(loopEventShiftEnd)
	case tea.KeyShiftDown:
		return eventLoopInput(loopEventShiftDown)
	case tea.KeyPgUp:
		return eventLoopInput(loopEventPageUp)
	case tea.KeyPgDown:
		return eventLoopInput(loopEventPageDown)
	case tea.KeyCtrlB:
		return eventLoopInput(loopEventPageUp)
	case tea.KeyCtrlF:
		return eventLoopInput(loopEventPageDown)
	case tea.KeyHome:
		return eventLoopInput(loopEventHome)
	case tea.KeyEnd:
		return eventLoopInput(loopEventEnd)
	case tea.KeyLeft:
		return eventLoopInput(loopEventHorizontalLeft)
	case tea.KeyRight:
		return eventLoopInput(loopEventHorizontalRight)
	case tea.KeyF1:
		return eventLoopInput(loopEventHelpToggle)
	case tea.KeyCtrlN:
		return eventLoopInput(loopEventSearchNext)
	case tea.KeyCtrlP:
		return eventLoopInput(loopEventSearchPrevious)
	case tea.KeySpace:
		return loopInput{event: loopEventSpacePick, text: " "}
	case tea.KeyRunes:
		text := string(msg.Runes)
		if len(msg.Runes) == 1 && msg.Runes[0] < 128 {
			return byteLoopInput(byte(msg.Runes[0]))
		}
		return loopInput{event: loopEventText, text: text}
	default:
		text := msg.String()
		if len(text) == 1 {
			return byteLoopInput(text[0])
		}
		if strings.TrimSpace(text) == "" {
			return eventLoopInput(loopEventUnknown)
		}
		return eventLoopInput(loopEventUnknown)
	}
}

func waitSourceResult(results <-chan sourceResult) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-results
		if !ok {
			return nil
		}
		return r
	}
}
func (m *teaLoopModel) submitSource() {
	m.requestID++
	m.busy = true
	m.dirty = false
	s := m.state
	// Ordinary refreshes do not need a copy of a potentially large selection.
	var selection *usecase.SelectionState
	if s.pendingCopy || s.pendingSelect {
		selection = s.selection.Clone()
	}
	m.jobs.Submit(sourceRequest{SelectionOps: append([]selectionOperation(nil), s.pendingSelectionOps...), IntentRevision: s.intentRevision, AcknowledgedGeneration: m.appliedGeneration, AnchorOutput: s.pendingAnchorOutput, ID: m.requestID, FilterRevision: s.filterRevision, Session: s.session, LogType: s.logType, FilterText: s.filterText,
		Navigation: usecase.NavigationOptions{OutputCount: s.outputCount(), ViewportHeight: s.listHeight, CursorOutputIndex: s.navigation.CursorOutputIndex(), ScrollOffset: s.navigation.ScrollOffset(), Follow: s.navigation.Follow()},
		Search:     s.searchText, SearchDirection: s.pendingDirection, SearchRequested: s.pendingSearch, Copy: s.pendingCopy, Select: s.pendingSelect, Anchor: s.pendingAnchor, Selection: selection, Clipboard: s.clipboard})
}
func (m teaLoopModel) updateAsync(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.state
	switch msg := msg.(type) {
	case sourceResult:
		if msg.ID != m.requestID || msg.FilterRevision != s.filterRevision || msg.Snapshot.Generation < m.appliedGeneration || msg.Snapshot.Generation == m.appliedGeneration && msg.Snapshot.ByteLimit < m.appliedBytes {
			return m, waitSourceResult(m.jobs.results)
		}
		m.busy = false
		if msg.IntentRevision != s.intentRevision {
			m.submitSource()
			return m, waitSourceResult(m.jobs.results)
		}
		if msg.Err != nil && !sourceJobCanceled(msg.Err) {
			m.err = msg.Err
			s.setPersistentRuntimeMessage(msg.Err.Error())
			return m, tea.Quit
		}
		if msg.Retry {
			m.submitSource()
			return m, waitSourceResult(m.jobs.results)
		}
		if msg.Replaced || m.appliedGeneration != 0 && msg.Snapshot.Generation != m.appliedGeneration {
			s.clearSelectionMode()
			s.bookmarks = usecase.NewBookmarkState()
		}
		m.appliedGeneration = msg.Snapshot.Generation
		m.appliedBytes = msg.Snapshot.ByteLimit
		s.counting = false
		s.totalRawLines = msg.TotalRaw
		s.filteredOutputCount = msg.TotalMatches
		s.lineIndex = &fileLineIndex{total: msg.TotalRaw, size: msg.Snapshot.ByteLimit, snapshot: msg.Snapshot}
		s.records = msg.Records
		s.rawLogs = nil
		s.windowStartOutputIndex = msg.Start
		s.logType = msg.LogType
		s.filter, _ = usecase.CompileFilter(s.filterText, s.filterOptions())
		if msg.Navigation != nil {
			s.navigation = msg.Navigation
		}
		if msg.Selection != nil {
			s.selection = msg.Selection
			s.syncSelectionMode()
		}
		s.pendingCopy = false
		s.pendingSelectionOps = nil
		s.pendingSelect = false
		s.pendingSearch = false
		var expiry tea.Cmd
		if msg.Message != "" {
			before := s.runtimeMessageID
			s.setTransientRuntimeMessage(msg.Message)
			expiry = m.runtimeMessageExpiryCmd(before)
		}
		if m.dirty {
			m.submitSource()
		} else if m.noKeyInput && m.stream != nil && s.readState == tui.ReadStateEOF {
			return m, tea.Quit
		}
		return m, tea.Batch(waitSourceResult(m.jobs.results), expiry)
	case tea.KeyMsg:
		beforeCursor, beforeScroll, beforeFollow, beforeRevision := s.navigation.CursorOutputIndex(), s.navigation.ScrollOffset(), s.navigation.Follow(), s.filterRevision
		beforeMessage := s.runtimeMessageID
		beforeIntent := s.intentRevision
		_, quit := s.handleInput(m.ctx, teaKeyToLoopInput(msg))
		if quit {
			return m, tea.Quit
		}
		if beforeIntent != s.intentRevision || s.pendingCopy || s.pendingSelect || s.pendingSearch || beforeCursor != s.navigation.CursorOutputIndex() || beforeScroll != s.navigation.ScrollOffset() || beforeFollow != s.navigation.Follow() || beforeRevision != s.filterRevision {
			m.submitSource()
		}
		return m, m.runtimeMessageExpiryCmd(beforeMessage)
	case tea.WindowSizeMsg:
		s.resize(msg.Width, msg.Height)
		m.submitSource()
		return m, nil
	case fileRefreshMsg:
		if m.busy {
			m.dirty = true
		} else {
			m.submitSource()
		}
		return m, waitFileRefresh()
	case streamRefreshMsg:
		if m.busy {
			m.dirty = true
		} else {
			m.submitSource()
		}
		return m, waitStreamRefresh(m.stream.refresh)
	case streamDoneMsg:
		s.readState = tui.ReadStateEOF
		if msg.err != nil {
			m.err = msg.err
			s.setPersistentRuntimeMessage(msg.err.Error())
			return m, tea.Quit
		}
		if m.busy {
			m.dirty = true
		} else {
			m.submitSource()
		}
		return m, nil
	case runtimeMessageExpiredMsg:
		s.clearTransientRuntimeMessage(msg.id)
		return m, nil
	}
	return m, nil
}
