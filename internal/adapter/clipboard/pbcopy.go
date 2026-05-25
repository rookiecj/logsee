package clipboard

import (
	"context"
	"fmt"
	"os/exec"
)

type PbcopyWriter struct {
	Command string
}

func (w PbcopyWriter) WriteText(ctx context.Context, text string) error {
	command := w.Command
	if command == "" {
		command = "pbcopy"
	}
	cmd := exec.CommandContext(ctx, command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open clipboard stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start clipboard command %q: %w; %s", command, err, InstallHint())
	}
	if _, err := stdin.Write([]byte(text)); err != nil {
		stdin.Close()
		cmd.Wait()
		return fmt.Errorf("write clipboard command %q: %w", command, err)
	}
	if err := stdin.Close(); err != nil {
		cmd.Wait()
		return fmt.Errorf("close clipboard stdin: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", command, err)
	}
	return nil
}
