package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"logsee/internal/adapter/cli"
)

func TestRunFileInputRendersInitialTUIFrame(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(logPath, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(""), &stdout, RunOptions{
		Width:   300,
		Height:  8,
		HomeDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("run app: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"filter:\n",
		"search:\n",
		"1   first\n",
		"2   second\n",
		"in:file:eof",
		"out:-",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
	if strings.Contains(output, "sot:") {
		t.Fatalf("output %q must not contain removed statusbar field sot", output)
	}
}

func TestRunSTDIOInputPersistsThenRendersFromSOT(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "session.log")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{
		InputPath: "-",
		OutPath:   outPath,
	}, strings.NewReader("alpha\nbeta\n"), &stdout, RunOptions{
		Width:   300,
		Height:  8,
		HomeDir: t.TempDir(),
		WorkDir: dir,
		Now:     time.Date(2026, 5, 24, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("run app: %v", err)
	}

	persisted, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read SOT: %v", err)
	}
	if got, want := string(persisted), "alpha\nbeta\n"; got != want {
		t.Fatalf("persisted SOT = %q, want %q", got, want)
	}

	output := stdout.String()
	for _, want := range []string{
		"1   alpha\n",
		"2   beta\n",
		"in:stdio:eof",
		"out:",
		"session.log",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
	if strings.Contains(output, outPath) {
		t.Fatalf("output %q must abbreviate long output path %q", output, outPath)
	}
	if !strings.Contains(output, "...") {
		t.Fatalf("output %q must middle-abbreviate long output path", output)
	}
	if strings.Contains(output, "sot:") {
		t.Fatalf("output %q must not contain removed statusbar field sot", output)
	}
}

func TestRunReflectsCLILogTypeInStatusbar(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "kernel.log")
	if err := os.WriteFile(logPath, []byte("plain text\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{
		InputPath: logPath,
		LogType:   "kernel",
	}, strings.NewReader(""), &stdout, RunOptions{
		Width:   120,
		Height:  8,
		HomeDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("run app: %v", err)
	}

	if !strings.Contains(stdout.String(), "type:kernel") {
		t.Fatalf("output %q does not contain type:kernel", stdout.String())
	}
}

func TestRunInteractiveUsesBubbleTeaRuntimeWhenRequested(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(logPath, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: logPath}, strings.NewReader(""), &stdout, RunOptions{
		Width:        120,
		Height:       6,
		HomeDir:      t.TempDir(),
		Interactive:  true,
		KeyInput:     strings.NewReader("q"),
		UseBubbleTea: true,
	})
	if err != nil {
		t.Fatalf("run app with Bubble Tea runtime: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"first",
		"second",
		"FILTER INPUT",
		"SEARCH INPUT",
		"\x1b[?1049h",
		"\x1b[?1049l",
		"\x1b[",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestRunReturnsContextForUnreadableInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.log")

	var stdout bytes.Buffer
	err := Run(context.Background(), cli.Options{InputPath: missing}, strings.NewReader(""), &stdout, RunOptions{
		Width:   120,
		Height:  8,
		HomeDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("run app error = nil, want error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"open input file", missing} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}
