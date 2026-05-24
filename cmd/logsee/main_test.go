package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"logsee/internal/adapter/app"
	"logsee/internal/adapter/cli"
)

func TestRunPrintsUsageForHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{
		"--log-type <auto|plain|adb|kernel>",
		"입력 로그의 타입",
		"default: ./logsee-YYYYMMDD-HHMMSS.log",
		"--version",
		"[input-file|-]",
		"로그 파일 지정",
		"지정하지 않거나 -이면 STDIO",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout %q does not contain %q", stdout.String(), want)
		}
	}
}

func TestRunReportsInvalidLogType(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--log-type", "json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{
		`invalid --log-type "json"`,
		"auto",
		"plain",
		"adb",
		"kernel",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr %q does not contain %q", stderr.String(), want)
		}
	}
}

func TestRunPrintsVersion(t *testing.T) {
	// Given
	originalVersion := version
	version = "9.8.7-test"
	t.Cleanup(func() {
		version = originalVersion
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// When
	code := run([]string{"--version"}, &stdout, &stderr)

	// Then
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := stdout.String(); got != "logsee 9.8.7-test\n" {
		t.Fatalf("stdout = %q, want version output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUsesTTYInputForPipedStdioBubbleTea(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var captured app.RunOptions
	var capturedKeys string

	code := runWithDeps(
		[]string{"--log-type", "plain"},
		strings.NewReader("log-line\n"),
		&stdout,
		&stderr,
		commandDeps{
			openTTY: func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("q")), nil
			},
			runApp: func(ctx context.Context, options cli.Options, stdin io.Reader, stdout io.Writer, runOptions app.RunOptions) error {
				captured = runOptions
				if runOptions.KeyInput != nil {
					keyBytes, err := io.ReadAll(runOptions.KeyInput)
					if err != nil {
						t.Fatalf("read key input: %v", err)
					}
					capturedKeys = string(keyBytes)
				}
				return nil
			},
		},
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr %q", code, stderr.String())
	}
	if !captured.Interactive {
		t.Fatal("interactive option = false, want true")
	}
	if !captured.UseBubbleTea {
		t.Fatal("UseBubbleTea = false, want command path to use Bubble Tea")
	}
	if capturedKeys != "q" {
		t.Fatalf("captured key input = %q, want /dev/tty input", capturedKeys)
	}
}
