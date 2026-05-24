package usecase

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"logsee/internal/port"
)

type InputMode string

const (
	InputModeStdio InputMode = "stdio"
	InputModeFile  InputMode = "file"
)

type InputRequest struct {
	InputPath  string
	OutPath    string
	IgnoreCase bool
	WorkDir    string
	Now        time.Time
}

type InputPorts struct {
	StdioSink   port.LineAppender
	StdioWorker port.LineAppendWorker
	FileSource  port.FileSource
}

type InputSession struct {
	Mode       InputMode
	SOTPath    string
	OutPath    string
	IgnoreCase bool

	worker      port.LineAppendWorker
	closeWorker func()
	source      port.FileSource
}

func NewInputSession(request InputRequest, ports InputPorts) (InputSession, error) {
	session := InputSession{
		IgnoreCase: request.IgnoreCase,
	}

	if isStdioInput(request.InputPath) {
		if ports.StdioWorker == nil && ports.StdioSink == nil {
			return InputSession{}, fmt.Errorf("stdio input requires an append worker or sink")
		}
		session.Mode = InputModeStdio
		session.OutPath = resolveOutPath(request)
		session.SOTPath = session.OutPath
		session.worker = ports.StdioWorker
		if session.worker == nil {
			session.worker, session.closeWorker = startLineAppendWorker(ports.StdioSink)
		}
		session.source = ports.FileSource
		if session.source == nil {
			return InputSession{}, fmt.Errorf("stdio input requires a file-backed source")
		}
		return session, nil
	}

	if ports.FileSource == nil {
		return InputSession{}, fmt.Errorf("file input requires a file-backed source")
	}
	session.Mode = InputModeFile
	session.SOTPath = request.InputPath
	session.source = ports.FileSource
	return session, nil
}

func (s InputSession) SourcePath() string {
	if s.source == nil {
		return ""
	}
	return s.source.Path()
}

func (s InputSession) DetectLogType(ctx context.Context, config LogTypeConfig) (LogType, error) {
	if s.source == nil {
		return "", fmt.Errorf("input session requires a file-backed source for log type detection")
	}
	return DetectLogType(ctx, s.source, config)
}

func (s InputSession) ConsumeStdio(ctx context.Context, input io.Reader, observer port.SourceObserver) error {
	if s.Mode != InputModeStdio {
		return fmt.Errorf("cannot consume stdio for %s input", s.Mode)
	}
	if s.worker == nil {
		return fmt.Errorf("stdio input has no append worker")
	}
	if s.closeWorker != nil {
		defer s.closeWorker()
	}

	scanner := bufio.NewScanner(input)
	lineNumber := 0
	for scanner.Scan() {
		line := scanner.Text()
		if err := s.worker.AppendLine(ctx, line); err != nil {
			return fmt.Errorf("append stdio line to SOT %q: %w", s.SOTPath, err)
		}
		if observer != nil {
			lineNumber++
			visibleLine, err := s.source.ReadLine(ctx, lineNumber)
			if err != nil {
				return fmt.Errorf("read visible line %d from SOT %q: %w", lineNumber, s.SOTPath, err)
			}
			observer.SourceLineAvailable(visibleLine)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stdio input: %w", err)
	}
	return nil
}

type appendRequest struct {
	ctx  context.Context
	line string
	done chan error
}

type lineAppendWorker struct {
	sink     port.LineAppender
	requests chan appendRequest
}

func startLineAppendWorker(sink port.LineAppender) (port.LineAppendWorker, func()) {
	worker := &lineAppendWorker{
		sink:     sink,
		requests: make(chan appendRequest),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for request := range worker.requests {
			request.done <- sink.AppendLine(request.ctx, request.line)
			close(request.done)
		}
	}()
	closeWorker := func() {
		close(worker.requests)
		<-done
	}
	return worker, closeWorker
}

func (w *lineAppendWorker) Path() string {
	return w.sink.Path()
}

func (w *lineAppendWorker) AppendLine(ctx context.Context, line string) error {
	request := appendRequest{
		ctx:  ctx,
		line: line,
		done: make(chan error, 1),
	}
	select {
	case w.requests <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isStdioInput(path string) bool {
	return path == "" || path == "-"
}

func ResolveStdioOutPath(request InputRequest) string {
	return resolveOutPath(request)
}

func resolveOutPath(request InputRequest) string {
	if request.OutPath != "" {
		return request.OutPath
	}

	now := request.Now
	if now.IsZero() {
		now = time.Now()
	}
	workDir := request.WorkDir
	if strings.TrimSpace(workDir) == "" {
		workDir = "."
	}
	return filepath.Join(workDir, "logsee-"+now.Format("20060102-150405")+".log")
}
