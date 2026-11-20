package app

import (
	"context"
	"logsee/internal/adapter/cli"
	"os"
	"testing"
)

// TestPTYHelper is driven by testdata/pty_smoke.py with a real controlling PTY.
func TestPTYHelper(t *testing.T) {
	if os.Getenv("LOGSEE_PTY_HELPER") != "1" {
		t.Skip("PTY subprocess helper")
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	err = Run(context.Background(), cli.Options{InputPath: os.Getenv("LOGSEE_TEST_INPUT"), OutPath: os.Getenv("LOGSEE_TEST_OUT")}, os.Stdin, os.Stdout, RunOptions{Interactive: true, UseBubbleTea: true, KeyInput: tty, HomeDir: os.Getenv("LOGSEE_TEST_HOME"), Clipboard: ptyClipboard{os.Getenv("LOGSEE_TEST_COPY")}})
	if err != nil {
		t.Fatal(err)
	}
}

type ptyClipboard struct{ path string }

func (c ptyClipboard) WriteText(_ context.Context, text string) error {
	return os.WriteFile(c.path, []byte(text), 0600)
}
