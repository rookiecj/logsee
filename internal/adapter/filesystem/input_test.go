package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendFilePersistsLinesAndReportsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.log")
	appender, err := NewAppendFile(path)
	if err != nil {
		t.Fatalf("new append file: %v", err)
	}
	defer appender.Close()

	if err := appender.AppendLine(context.Background(), "first"); err != nil {
		t.Fatalf("append first line: %v", err)
	}
	if err := appender.AppendLine(context.Background(), "second"); err != nil {
		t.Fatalf("append second line: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read SOT file: %v", err)
	}
	if got, want := string(content), "first\nsecond\n"; got != want {
		t.Fatalf("SOT content = %q, want %q", got, want)
	}
	if appender.Path() != path {
		t.Fatalf("appender path = %q, want %q", appender.Path(), path)
	}
}

func TestAppendFileAppendsLinesInOneBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.log")
	appender, err := NewAppendFile(path)
	if err != nil {
		t.Fatalf("new append file: %v", err)
	}
	defer appender.Close()

	if err := appender.AppendLines(context.Background(), []string{"first", "second", "third"}); err != nil {
		t.Fatalf("append lines batch: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read SOT file: %v", err)
	}
	if got, want := string(content), "first\nsecond\nthird\n"; got != want {
		t.Fatalf("SOT content = %q, want %q", got, want)
	}
}

func TestFileSourceReportsOriginalPath(t *testing.T) {
	source := NewFileSource("/var/log/app.log")
	if source.Path() != "/var/log/app.log" {
		t.Fatalf("source path = %q, want original path", source.Path())
	}
}

func TestFileSourceReadsLineFromOriginalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.log")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatalf("write SOT file: %v", err)
	}

	source := NewFileSource(path)
	line, err := source.ReadLine(context.Background(), 2)
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	if line != "second" {
		t.Fatalf("line = %q, want %q", line, "second")
	}
}

func TestFileSourceReadsRawByteRanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.log")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatalf("write SOT file: %v", err)
	}

	source := NewFileSource(path)
	size, err := source.Size(context.Background())
	if err != nil {
		t.Fatalf("source size: %v", err)
	}
	if size != 6 {
		t.Fatalf("size = %d, want 6", size)
	}

	data, err := source.ReadAt(context.Background(), 2, 3)
	if err != nil {
		t.Fatalf("read byte range: %v", err)
	}
	if string(data) != "cde" {
		t.Fatalf("data = %q, want %q", string(data), "cde")
	}
}
