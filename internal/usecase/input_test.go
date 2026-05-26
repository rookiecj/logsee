package usecase

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStdioInputPersistsLinesBeforeSourceVisibility(t *testing.T) {
	events := []string{}
	appender := &recordingAppender{
		path:   "/tmp/session.log",
		events: &events,
	}
	observer := &recordingObserver{events: &events}
	source := &recordingSource{
		path:   "/tmp/session.log",
		lines:  []string{"first", "second"},
		events: &events,
	}

	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   "/tmp/session.log",
	}, InputPorts{
		StdioSink:  appender,
		FileSource: source,
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	err = session.ConsumeStdio(context.Background(), strings.NewReader("first\nsecond\n"), observer)
	if err != nil {
		t.Fatalf("consume stdio: %v", err)
	}

	wantEvents := []string{
		"append-batch:2",
		"visible:",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	if got, want := appender.lines, []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted lines = %#v, want %#v", got, want)
	}
	if session.SourcePath() != "/tmp/session.log" {
		t.Fatalf("source path = %q, want SOT path", session.SourcePath())
	}
}

func TestStdioVisibilityDoesNotRescanSOTForEveryLine(t *testing.T) {
	// given
	appender := &recordingAppender{path: "/tmp/session.log"}
	observer := &recordingObserver{}
	source := &recordingSource{
		path:       "/tmp/session.log",
		failOnRead: true,
	}
	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   "/tmp/session.log",
	}, InputPorts{
		StdioSink:  appender,
		FileSource: source,
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	// when
	err = session.ConsumeStdio(context.Background(), strings.NewReader("first\nsecond\n"), observer)

	// then
	if err != nil {
		t.Fatalf("consume stdio: %v", err)
	}
	if got, want := observer.lines, []string{""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("visibility notifications = %#v, want one batch notification %#v", got, want)
	}
	if got, want := source.readCalls, 0; got != want {
		t.Fatalf("SOT read calls = %d, want %d", got, want)
	}
}

func TestStdioHighVolumePersistenceExceedsFormerStallSize(t *testing.T) {
	// given
	const lineCount = 80_000
	linePayload := strings.Repeat("x", 48)
	var input strings.Builder
	input.Grow(lineCount * (len(linePayload) + 8))
	for i := 1; i <= lineCount; i++ {
		input.WriteString("line-")
		input.WriteString(strconv.Itoa(i))
		input.WriteString(linePayload)
		input.WriteByte('\n')
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")
	appender := &recordingAppender{path: path}
	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   path,
	}, InputPorts{
		StdioSink:  appender,
		FileSource: &recordingSource{path: path},
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	// when
	err = session.ConsumeStdio(context.Background(), strings.NewReader(input.String()), nil)

	// then
	if err != nil {
		t.Fatalf("consume stdio: %v", err)
	}
	const formerStallBytes = 4 * 1024 * 1024
	estimatedBytes := 0
	for _, line := range appender.lines {
		estimatedBytes += len(line) + 1
	}
	if estimatedBytes <= formerStallBytes {
		t.Fatalf("persisted bytes = %d, want more than former stall point %d", estimatedBytes, formerStallBytes)
	}
	if got, want := appender.batchAppendCalls, (lineCount+stdioAppendBatchMaxLines-1)/stdioAppendBatchMaxLines; got != want {
		t.Fatalf("batch append calls = %d, want %d", got, want)
	}
}

func TestStdioConsumeUsesBatchedPersistenceWrites(t *testing.T) {
	// given
	const lineCount = stdioAppendBatchMaxLines + 5
	var input strings.Builder
	for i := 1; i <= lineCount; i++ {
		input.WriteString("line-")
		input.WriteString(strconv.Itoa(i))
		input.WriteByte('\n')
	}
	appender := &recordingAppender{path: "/tmp/session.log"}
	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   "/tmp/session.log",
	}, InputPorts{
		StdioSink:  appender,
		FileSource: &recordingSource{path: "/tmp/session.log"},
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	// when
	err = session.ConsumeStdio(context.Background(), strings.NewReader(input.String()), nil)

	// then
	if err != nil {
		t.Fatalf("consume stdio: %v", err)
	}
	if got, want := appender.batchAppendCalls, 2; got != want {
		t.Fatalf("batch append calls = %d, want %d", got, want)
	}
	if got, want := appender.appendLineCalls, 0; got != want {
		t.Fatalf("single-line append calls = %d, want %d", got, want)
	}
	if got, want := len(appender.lines), lineCount; got != want {
		t.Fatalf("persisted lines = %d, want %d", got, want)
	}
}

func TestStdioVisibilityUsesPersistedInputLineAfterAppend(t *testing.T) {
	appender := &recordingAppender{path: "/tmp/session.log"}
	observer := &recordingObserver{}
	source := &recordingSource{
		path:  "/tmp/session.log",
		lines: []string{"from-sot:first", "from-sot:second"},
	}

	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   "/tmp/session.log",
	}, InputPorts{
		StdioSink:  appender,
		FileSource: source,
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	err = session.ConsumeStdio(context.Background(), strings.NewReader("from-stdin:first\nfrom-stdin:second\n"), observer)
	if err != nil {
		t.Fatalf("consume stdio: %v", err)
	}

	if got, want := observer.lines, []string{""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("visibility notifications = %#v, want one batch notification %#v", got, want)
	}
	if got, want := appender.lines, []string{"from-stdin:first", "from-stdin:second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted lines = %#v, want original stdin lines %#v", got, want)
	}
}

func TestStdioAppendUsesWorkerBoundaryBeforeSourceVisibility(t *testing.T) {
	events := []string{}
	worker := &recordingWorker{
		path:   "/tmp/session.log",
		events: &events,
	}
	observer := &recordingObserver{events: &events}
	source := &recordingSource{
		path:   "/tmp/session.log",
		lines:  []string{"visible-one"},
		events: &events,
	}

	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   "/tmp/session.log",
	}, InputPorts{
		StdioWorker: worker,
		FileSource:  source,
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	err = session.ConsumeStdio(context.Background(), strings.NewReader("raw-one\n"), observer)
	if err != nil {
		t.Fatalf("consume stdio: %v", err)
	}

	wantEvents := []string{
		"worker-batch:1",
		"visible:",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want worker append before visibility %#v", events, wantEvents)
	}
	if got, want := worker.lines, []string{"raw-one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("worker lines = %#v, want %#v", got, want)
	}
}

func TestFileInputUsesOriginalFileAsSOTWithoutOutputCopy(t *testing.T) {
	session, err := NewInputSession(InputRequest{
		InputPath: "/var/log/app.log",
		OutPath:   "/tmp/should-not-exist.log",
	}, InputPorts{
		FileSource: &recordingSource{path: "/var/log/app.log"},
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	if session.Mode != InputModeFile {
		t.Fatalf("mode = %q, want %q", session.Mode, InputModeFile)
	}
	if session.SOTPath != "/var/log/app.log" {
		t.Fatalf("SOT path = %q, want original file", session.SOTPath)
	}
	if session.OutPath != "" {
		t.Fatalf("out path = %q, want empty for file input", session.OutPath)
	}
	if session.SourcePath() != "/var/log/app.log" {
		t.Fatalf("source path = %q, want original file", session.SourcePath())
	}
}

func TestStdioAppendFailureSuppressesVisibilityAndReportsError(t *testing.T) {
	events := []string{}
	appendErr := errors.New("disk full")
	appender := &recordingAppender{
		path:      "/tmp/session.log",
		events:    &events,
		failLines: map[string]error{"bad": appendErr},
	}
	observer := &recordingObserver{events: &events}

	session, err := NewInputSession(InputRequest{
		InputPath: "-",
		OutPath:   "/tmp/session.log",
	}, InputPorts{
		StdioSink:  appender,
		FileSource: &recordingSource{path: "/tmp/session.log", lines: []string{"ok"}, events: &events},
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	err = session.ConsumeStdio(context.Background(), strings.NewReader("ok\nbad\nlater\n"), observer)
	if !errors.Is(err, appendErr) {
		t.Fatalf("consume stdio error = %v, want wrapped %v", err, appendErr)
	}

	wantEvents := []string{
		"append-batch:3",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	if got, want := len(observer.lines), 0; got != want {
		t.Fatalf("visibility notifications = %d, want %d after failed batch", got, want)
	}
}

func TestDefaultStdioOutPathUsesWorkDirAndTimestamp(t *testing.T) {
	session, err := NewInputSession(InputRequest{
		InputPath: "",
		WorkDir:   "/work",
		Now:       time.Date(2026, 5, 24, 1, 2, 3, 0, time.UTC),
	}, InputPorts{
		StdioSink:  &recordingAppender{path: "/work/logsee-20260524-010203.log"},
		FileSource: &recordingSource{path: "/work/logsee-20260524-010203.log"},
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	if session.SOTPath != "/work/logsee-20260524-010203.log" {
		t.Fatalf("SOT path = %q, want default timestamped path", session.SOTPath)
	}
}

func TestInputSessionDetectsLogTypeFromFileBackedSOTSource(t *testing.T) {
	session, err := NewInputSession(InputRequest{
		InputPath: "/var/log/session.log",
	}, InputPorts{
		FileSource: &recordingSource{
			path: "/var/log/session.log",
			lines: []string{
				"",
				"05-24 12:34:56.789  1234  5678 I ActivityManager: start proc",
			},
		},
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	logType, err := session.DetectLogType(context.Background(), DefaultLogTypeConfig())
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypeADB {
		t.Fatalf("log type = %q, want %q", logType, LogTypeADB)
	}
}

type recordingAppender struct {
	path             string
	lines            []string
	events           *[]string
	failLines        map[string]error
	batchAppendCalls int
	appendLineCalls  int
}

func (a *recordingAppender) Path() string {
	return a.path
}

func (a *recordingAppender) AppendLine(ctx context.Context, line string) error {
	a.appendLineCalls++
	return a.AppendLines(ctx, []string{line})
}

func (a *recordingAppender) AppendLines(_ context.Context, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	a.batchAppendCalls++
	if a.events != nil {
		*a.events = append(*a.events, "append-batch:"+strconv.Itoa(len(lines)))
	}
	for _, line := range lines {
		if err := a.failLines[line]; err != nil {
			return err
		}
		a.lines = append(a.lines, line)
	}
	return nil
}

type recordingWorker struct {
	path   string
	lines  []string
	events *[]string
}

func (w *recordingWorker) Path() string {
	return w.path
}

func (w *recordingWorker) AppendLine(ctx context.Context, line string) error {
	return w.AppendLines(ctx, []string{line})
}

func (w *recordingWorker) AppendLines(_ context.Context, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	if w.events != nil {
		*w.events = append(*w.events, "worker-batch:"+strconv.Itoa(len(lines)))
	}
	w.lines = append(w.lines, lines...)
	return nil
}

type recordingObserver struct {
	lines  []string
	events *[]string
}

func (o *recordingObserver) SourceLineAvailable(line string) {
	if o.events != nil {
		*o.events = append(*o.events, "visible:"+line)
	}
	o.lines = append(o.lines, line)
}

type recordingSource struct {
	path       string
	lines      []string
	events     *[]string
	failOnRead bool
	readCalls  int
}

func (s *recordingSource) Path() string {
	return s.path
}

func (s *recordingSource) ReadLine(_ context.Context, lineNumber int) (string, error) {
	s.readCalls++
	if s.events != nil {
		*s.events = append(*s.events, "read:"+strconv.Itoa(lineNumber))
	}
	if s.failOnRead {
		return "", errors.New("unexpected SOT read")
	}
	if lineNumber < 1 || lineNumber > len(s.lines) {
		return "", errors.New("line not available")
	}
	return s.lines[lineNumber-1], nil
}

func (s *recordingSource) SampleLines(_ context.Context, maxNonEmpty int) ([]string, error) {
	if maxNonEmpty < 1 {
		return nil, errors.New("invalid max non-empty lines")
	}
	sampled := []string{}
	for _, line := range s.lines {
		if line == "" {
			continue
		}
		sampled = append(sampled, line)
		if len(sampled) == maxNonEmpty {
			break
		}
	}
	return sampled, nil
}
