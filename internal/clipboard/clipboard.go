package clipboard

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
)

// SetText writes text to the system clipboard (best-effort per OS).
func SetText(s string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = bytes.NewReader([]byte(s))
		return cmd.Run()
	case "linux":
		if path, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command(path)
			cmd.Stdin = bytes.NewReader([]byte(s))
			return cmd.Run()
		}
		if path, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command(path, "-selection", "clipboard")
			cmd.Stdin = bytes.NewReader([]byte(s))
			return cmd.Run()
		}
		return fmt.Errorf("clipboard: install wl-copy or xclip")
	default:
		return fmt.Errorf("clipboard: unsupported OS %s", runtime.GOOS)
	}
}
