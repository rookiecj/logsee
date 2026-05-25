package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"logsee/internal/usecase"
)

type persistedInputHistory struct {
	Filter persistedChannelHistory `json:"filter"`
	Search persistedChannelHistory `json:"search"`
}

type persistedChannelHistory struct {
	Last    string   `json:"last"`
	History []string `json:"history"`
}

func ResolveInputHistoryPath(homeDir string) string {
	return filepath.Join(homeDir, ".local", "logsee", "input_history.json")
}

func LoadInputHistory(path string) (usecase.InputHistorySnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return usecase.InputHistorySnapshot{}, nil
		}
		return usecase.InputHistorySnapshot{}, fmt.Errorf("read input history %q: %w", path, err)
	}

	var persisted persistedInputHistory
	if err := json.Unmarshal(data, &persisted); err != nil {
		return usecase.InputHistorySnapshot{}, fmt.Errorf("parse input history %q: %w", path, err)
	}
	return usecase.InputHistorySnapshot{
		Filter: usecase.InputChannelHistory{
			Last:    persisted.Filter.Last,
			History: append([]string(nil), persisted.Filter.History...),
		},
		Search: usecase.InputChannelHistory{
			Last:    persisted.Search.Last,
			History: append([]string(nil), persisted.Search.History...),
		},
	}, nil
}

func SaveInputHistory(path string, snapshot usecase.InputHistorySnapshot) error {
	persisted := persistedInputHistory{
		Filter: persistedChannelHistory{
			Last:    snapshot.Filter.Last,
			History: append([]string(nil), snapshot.Filter.History...),
		},
		Search: persistedChannelHistory{
			Last:    snapshot.Search.Last,
			History: append([]string(nil), snapshot.Search.History...),
		},
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal input history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create input history dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write input history %q: %w", path, err)
	}
	return nil
}
