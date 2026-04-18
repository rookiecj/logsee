package userstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const stateFileName = "state.json"

// DefaultStateDir returns $HOME/.local/logsee (no trailing slash).
func DefaultStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "logsee"), nil
}

// StateFilePath joins dir with the fixed state file name.
func StateFilePath(dir string) string {
	return filepath.Join(dir, stateFileName)
}

// Load reads a Snapshot from path. Missing file yields EmptySnapshot and nil error.
func Load(path string) (Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EmptySnapshot(), nil
		}
		return Snapshot{}, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, fmt.Errorf("userstate: parse %q: %w", path, err)
	}
	if s.Version == 0 {
		s.Version = SnapshotVersion
	}
	return s, nil
}

// Save writes snapshot to path, creating parent directories. Uses atomic replace via temp file in the same directory.
func Save(path string, s Snapshot) error {
	s.Version = SnapshotVersion
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("userstate: mkdir %q: %w", dir, err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".logsee-state-*.tmp")
	if err != nil {
		return fmt.Errorf("userstate: temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("userstate: rename to %q: %w", path, err)
	}
	return nil
}
