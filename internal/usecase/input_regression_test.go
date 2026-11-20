package usecase

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type observedAppender struct{ batches chan []string }

func (s *observedAppender) Path() string { return "test.log" }
func (s *observedAppender) AppendLine(c context.Context, v string) error {
	return s.AppendLines(c, []string{v})
}
func (s *observedAppender) AppendLines(_ context.Context, v []string) error {
	s.batches <- append([]string(nil), v...)
	return nil
}

func TestStdioIdleFlush(t *testing.T) {
	sink := &observedAppender{make(chan []string, 8)}
	session, err := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{path: "test.log"}})
	if err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	defer r.Close()
	done := make(chan error, 1)
	go func() { done <- session.ConsumeStdio(context.Background(), r, nil) }()
	if _, err = w.Write([]byte("one\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-sink.batches:
		if len(got) != 1 || got[0] != "one" {
			t.Errorf("batch=%v", got)
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("complete line was not persisted while pipe remained open")
	}
	w.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
func TestStdioLongLine(t *testing.T) {
	sink := &recordingAppender{path: "test.log"}
	session, err := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{path: "test.log"}})
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 1024*1024)
	if err = session.ConsumeStdio(context.Background(), strings.NewReader("keep\n"+long+"\n"), nil); err != nil {
		t.Fatal(err)
	}
	if len(sink.lines) != 2 || sink.lines[0] != "keep" || sink.lines[1] != long {
		t.Fatal("long line or earlier line lost")
	}
}

type failingReader struct{ sent bool }

var errRead = errors.New("injected read failure")

func (r *failingReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, errRead
	}
	r.sent = true
	return copy(p, "keep\n"), errRead
}
func TestStdioReadErrorFlushesAcceptedData(t *testing.T) {
	sink := &recordingAppender{path: "test.log"}
	s, e := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{path: "test.log"}})
	if e != nil {
		t.Fatal(e)
	}
	e = s.ConsumeStdio(context.Background(), &failingReader{}, nil)
	if !errors.Is(e, errRead) {
		t.Fatalf("error=%v", e)
	}
	if len(sink.lines) != 1 || sink.lines[0] != "keep" {
		t.Fatalf("accepted data lost: %v", sink.lines)
	}
}

type rawTestSink struct {
	maxBatch int
	recordingAppender
	data    []byte
	fail    error
	initial int64
	entered chan struct{}
	release chan struct{}
}

func (s *rawTestSink) Size(context.Context) (int64, error) { return s.initial, nil }
func (s *rawTestSink) AppendBytes(_ context.Context, data []byte) error {
	s.maxBatch = max(s.maxBatch, len(data))
	if s.entered != nil {
		close(s.entered)
		<-s.release
	}
	if s.fail != nil {
		return s.fail
	}
	s.data = append(s.data, data...)
	return nil
}
func TestStdioSyncFailureNotPublished(t *testing.T) {
	failure := errors.New("sync failed")
	sink := &rawTestSink{initial: 123, fail: failure}
	session, err := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{}})
	if err != nil {
		t.Fatal(err)
	}
	err = session.ConsumeStdio(context.Background(), strings.NewReader("one\n"), nil)
	if !errors.Is(err, failure) || session.Progress().PublishedBytes != 123 {
		t.Fatalf("err=%v progress=%+v", err, session.Progress())
	}
}
func TestStdioPublishesOnlyAfterSync(t *testing.T) {
	sink := &rawTestSink{entered: make(chan struct{}), release: make(chan struct{}), initial: 7}
	session, _ := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{}})
	done := make(chan error, 1)
	go func() { done <- session.ConsumeStdio(context.Background(), strings.NewReader("a\r\ntail"), nil) }()
	<-sink.entered
	if session.Progress().PublishedBytes != 7 {
		t.Fatal("uncommitted bytes published")
	}
	close(sink.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if string(sink.data) != "a\r\ntail" || session.Progress().PublishedBytes != 14 || !session.Progress().EOF {
		t.Fatalf("bytes=%q progress=%+v", sink.data, session.Progress())
	}
}
func TestStdioQuitDrainsAcceptedData(t *testing.T) {
	sink := &rawTestSink{}
	session, _ := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{}})
	input, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.ConsumeStdio(ctx, input, nil) }()
	accepted := "one\r\npartial"
	if _, err := writer.Write([]byte(accepted)); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reader did not stop")
	}
	if string(sink.data) != accepted {
		t.Fatalf("saved=%q accepted=%q", sink.data, accepted)
	}
	writer.Close()
}
func TestIgnoreCaseUnicodePrefix(t *testing.T) {
	filter, err := CompileFilter("service:prod", FilterOptions{IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"İ service=prod", "K service=prod"} {
		if !filter.Match(line) {
			t.Errorf("lost byte coordinates: %q", line)
		}
	}
}
func TestStdioReadAndSyncErrorsPreserved(t *testing.T) {
	syncErr := errors.New("sync failed")
	sink := &rawTestSink{fail: syncErr}
	session, _ := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{}})
	err := session.ConsumeStdio(context.Background(), &failingReader{}, nil)
	if !errors.Is(err, syncErr) || !errors.Is(err, errRead) {
		t.Fatalf("lost original failure: %v", err)
	}
}
func TestStdioBatchByteLimitWithUnevenReads(t *testing.T) {
	sink := &rawTestSink{}
	session, _ := NewInputSession(InputRequest{}, InputPorts{StdioSink: sink, FileSource: &recordingSource{}})
	input := io.MultiReader(strings.NewReader("x"), strings.NewReader(strings.Repeat("z", 128*1024)))
	if err := session.ConsumeStdio(context.Background(), input, nil); err != nil {
		t.Fatal(err)
	}
	if sink.maxBatch > 64*1024 {
		t.Fatalf("byte batch exceeded limit: %d", sink.maxBatch)
	}
}
