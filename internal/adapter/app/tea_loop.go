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

type streamDoneMsg struct {
	err error
}

type runtimeMessageExpiredMsg struct {
	id int
}

type teaLoopModel struct {
	ctx        context.Context
	state      *loopState
	stream     *stdioStream
	noKeyInput bool
	err        error
}

func runBubbleTeaLoop(ctx context.Context, session usecase.InputSession, sourcePath string, logType usecase.LogType, width, height int, keyInput io.Reader, output io.Writer, stream *stdioStream, clipboardWriter port.ClipboardWriter) error {
	state, err := newLoopState(ctx, session, sourcePath, logType, width, height, unboundedRecordLimit)
	if err != nil {
		return err
	}
	state.clipboard = clipboardWriter
	if stream != nil {
		state.readState = tui.ReadStateRead
		defer stream.cancel()
	}

	model := teaLoopModel{
		ctx:        ctx,
		state:      state,
		stream:     stream,
		noKeyInput: keyInput == nil,
	}
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
	if m.stream == nil {
		return nil
	}
	return tea.Batch(
		waitStreamRefresh(m.stream.refresh),
		waitStreamDone(m.stream.done),
	)
}

func (m teaLoopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.state.resize(msg.Width, msg.Height)
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
