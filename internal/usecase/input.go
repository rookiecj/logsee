package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/muesli/cancelreader"

	"logsee/internal/port"
)

type InputMode string

const (
	InputModeStdio InputMode = "stdio"
	InputModeFile  InputMode = "file"
)

const (
	stdioAppendBatchMaxLines = 64
	stdioAppendBatchMaxBytes = 64 * 1024
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
	Mode          InputMode
	SOTPath       string
	OutPath       string
	IgnoreCase    bool
	LogTypeConfig LogTypeConfig

	worker   port.LineAppendWorker
	source   port.FileSource
	rawSink  port.ByteAppender
	progress *inputProgress
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
			session.worker = ports.StdioSink
		}
		session.rawSink, _ = ports.StdioSink.(port.ByteAppender)
		session.progress = &inputProgress{}
		if session.rawSink != nil {
			size, err := session.rawSink.Size(context.Background())
			if err != nil {
				return InputSession{}, err
			}
			session.progress.value.PublishedBytes = size
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
	resolved, err := ResolveLogTypeConfig("", config)
	if err != nil {
		return "", err
	}
	if s.Mode == InputModeStdio && resolved.Default == LogTypeAuto && !s.Progress().EOF {
		var lines []string
		var err error
		if sampler, ok := s.source.(interface {
			SampleCompleteLines(context.Context, int) ([]string, error)
		}); ok {
			lines, err = sampler.SampleCompleteLines(ctx, resolved.ProbeLines)
		} else {
			lines, err = s.source.SampleLines(ctx, resolved.ProbeLines)
		}
		if err != nil {
			return "", err
		}
		if len(lines) < resolved.ProbeLines {
			return LogTypeAuto, nil
		}
	}
	return DetectLogType(ctx, s.source, resolved)
}

// Progress is a shared, absolute publication frontier. Notifications may coalesce;
// readers always consult this state, which advances only after the sink has synced.
type inputProgress struct {
	mu    sync.RWMutex
	value port.SourceProgress
}

func (s InputSession) Progress() port.SourceProgress {
	if s.progress == nil {
		return port.SourceProgress{EOF: true}
	}
	s.progress.mu.RLock()
	defer s.progress.mu.RUnlock()
	return s.progress.value
}
func (s InputSession) publish(bytes int64, lines int, eof bool, observer port.SourceObserver) {
	if s.progress != nil {
		s.progress.mu.Lock()
		s.progress.value.PublishedBytes += bytes
		s.progress.value.CompleteLines += lines
		s.progress.value.EOF = eof
		value := s.progress.value
		s.progress.mu.Unlock()
		if o, ok := observer.(port.ProgressObserver); ok {
			o.SourceProgressAvailable(value)
			return
		}
	}
	if observer != nil {
		observer.SourceLineAvailable("")
	}
}

func (s InputSession) ConsumeStdio(ctx context.Context, input io.Reader, observer port.SourceObserver) error {
	if s.Mode != InputModeStdio {
		return fmt.Errorf("cannot consume stdio for %s input", s.Mode)
	}
	if s.worker == nil {
		return fmt.Errorf("stdio input has no append worker")
	}
	// File descriptors need cancellable polling, rather than closing caller-owned stdin.
	stopRead := func() {}
	interruptible := false
	if f, ok := input.(*os.File); ok {
		reader, err := cancelreader.NewReader(f)
		if err != nil {
			return err
		}
		input = reader
		stopRead = func() { reader.Cancel() }
		interruptible = true
		defer reader.Close()
	} else if closer, ok := input.(io.Closer); ok {
		stopRead = func() { _ = closer.Close() }
		interruptible = true
	} else if closer, ok := input.(interface{ Close() }); ok {
		stopRead = closer.Close
		interruptible = true
	}
	type readResult struct {
		data []byte
		err  error
	}
	results := make(chan readResult)
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		defer close(results)
		for {
			buffer := make([]byte, 32*1024)
			n, err := input.Read(buffer)
			if n > 0 || err != nil {
				select {
				case results <- readResult{buffer[:n], err}:
				case <-stopped:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var raw bytes.Buffer
	var partial []byte
	batch := make([]string, 0, stdioAppendBatchMaxLines)
	batchBytes, lines := 0, 0
	// Once accepted, bytes are drained even when the input context is canceled.
	writeCtx := context.WithoutCancel(ctx)
	flush := func() error {
		if raw.Len() == 0 && len(batch) == 0 {
			return nil
		}
		var err error
		count := batchBytes
		if s.rawSink != nil {
			count = raw.Len()
			err = s.rawSink.AppendBytes(writeCtx, raw.Bytes())
		} else {
			err = s.worker.AppendLines(writeCtx, batch)
		}
		if err != nil {
			return fmt.Errorf("append stdio batch to SOT %q: %w", s.SOTPath, err)
		}
		s.publish(int64(count), lines, false, observer)
		raw.Reset()
		batch = batch[:0]
		batchBytes = 0
		lines = 0
		return nil
	}
	var readErr error
	accept := func(data []byte) error {
		if s.rawSink != nil {
			for len(data) > 0 {
				end := min(len(data), stdioAppendBatchMaxBytes-raw.Len())
				for i, b := range data[:end] {
					if b == '\n' {
						lines++
						if lines == stdioAppendBatchMaxLines {
							end = i + 1
							break
						}
					}
				}
				raw.Write(data[:end])
				data = data[end:]
				if raw.Len() == stdioAppendBatchMaxBytes || lines == stdioAppendBatchMaxLines {
					if err := flush(); err != nil {
						if readErr != io.EOF {
							return errors.Join(readErr, err)
						}
						return err
					}
				}
			}
			return nil
		}
		for len(data) > 0 {
			n := bytes.IndexByte(data, '\n')
			complete := n >= 0
			if !complete {
				n = len(data) - 1
			}
			part := data[:n+1]
			data = data[n+1:]
			partial = append(partial, part...)
			if complete {
				batch = append(batch, strings.TrimSuffix(strings.TrimSuffix(string(partial), "\n"), "\r"))
				batchBytes += len(partial)
				partial = nil
			}

			if complete {
				lines++
			}
			if lines >= stdioAppendBatchMaxLines || raw.Len() >= stdioAppendBatchMaxBytes || batchBytes >= stdioAppendBatchMaxBytes {
				if err := flush(); err != nil {
					if readErr != io.EOF {
						return errors.Join(readErr, err)
					}
					return err
				}
			}
		}
		return nil
	}
	canceled := ctx.Done()
	for {
		select {
		case result, ok := <-results:
			if !ok {
				if len(partial) > 0 {
					batch = append(batch, string(partial))
					batchBytes += len(partial)
				}
				if err := flush(); err != nil {
					if readErr != io.EOF {
						return errors.Join(readErr, err)
					}
					return err
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if readErr == nil || readErr == io.EOF {
					s.publish(0, 0, true, nil)
					return nil
				}
				return fmt.Errorf("read stdio input: %w", readErr)
			}
			if result.err != nil {
				readErr = result.err
			}
			if err := accept(result.data); err != nil {
				stopRead()
				return err
			}
			if result.err != nil {
				readErr = result.err
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				stopRead()
				return err
			}
		case <-canceled:
			canceled = nil
			stopRead()
			if !interruptible {
				if err := flush(); err != nil {
					return err
				}
				return ctx.Err()
			}
		}
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

func (s InputSession) PublishedLimit() (int64, bool) {
	if s.progress == nil {
		return 0, false
	}
	return s.Progress().PublishedBytes, true
}
