package clipboard

import (
	"fmt"
	"runtime"

	"logsee/internal/port"
)

// DefaultWriter returns the platform clipboard adapter for interactive copy (c).
func DefaultWriter() port.ClipboardWriter {
	switch runtime.GOOS {
	case "darwin":
		return PbcopyWriter{}
	case "linux":
		return XclipWriter{}
	default:
		return XclipWriter{}
	}
}

// DefaultCommandName reports the external command used by DefaultWriter on this OS.
func DefaultCommandName() string {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy"
	case "linux":
		return "xclip"
	default:
		return "xclip"
	}
}

// InstallHint returns a short note when the default clipboard command may be missing.
func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS includes /usr/bin/pbcopy; if copy fails, verify pbcopy is available in PATH"
	case "linux":
		return "install xclip, for example: apt install xclip  or  brew install xclip"
	default:
		return fmt.Sprintf("install %s for clipboard copy on %s", DefaultCommandName(), runtime.GOOS)
	}
}
