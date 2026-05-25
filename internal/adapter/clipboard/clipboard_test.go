package clipboard

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
)

func TestDefaultCommandNameMatchesPlatform(t *testing.T) {
	got := DefaultCommandName()
	switch runtime.GOOS {
	case "darwin":
		if got != "pbcopy" {
			t.Fatalf("command = %q, want pbcopy", got)
		}
	case "linux":
		if got != "xclip" {
			t.Fatalf("command = %q, want xclip", got)
		}
	default:
		if got != "xclip" {
			t.Fatalf("command = %q, want xclip fallback", got)
		}
	}
}

func TestDefaultWriterTypeMatchesPlatform(t *testing.T) {
	writer := DefaultWriter()
	switch runtime.GOOS {
	case "darwin":
		if _, ok := writer.(PbcopyWriter); !ok {
			t.Fatalf("writer type = %T, want PbcopyWriter", writer)
		}
	case "linux":
		if _, ok := writer.(XclipWriter); !ok {
			t.Fatalf("writer type = %T, want XclipWriter", writer)
		}
	}
}

func TestPbcopyWriterPropagatesMissingCommandError(t *testing.T) {
	err := (PbcopyWriter{Command: "/no/such/pbcopy"}).WriteText(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestPbcopyWriterWritesToClipboardOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pbcopy integration test runs on darwin only")
	}
	if _, err := exec.LookPath("pbcopy"); err != nil {
		t.Skip("pbcopy not in PATH")
	}
	if _, err := exec.LookPath("pbpaste"); err != nil {
		t.Skip("pbpaste not in PATH")
	}

	const want = "logsee-clipboard-test"
	if err := (PbcopyWriter{}).WriteText(context.Background(), want); err != nil {
		t.Fatalf("write clipboard: %v", err)
	}

	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		t.Fatalf("read clipboard: %v", err)
	}
	if got := string(out); got != want {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
}
