package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"strings"

	"logsee/internal/usecase"
)

func ResolveConfigPath(explicitPath, homeDir string) string {
	if strings.TrimSpace(explicitPath) != "" {
		return explicitPath
	}
	return filepath.Join(homeDir, ".local", "logsee", "config.toml")
}

func LoadLogTypeConfig(path string) (usecase.LogTypeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return usecase.LogTypeConfig{}, nil
		}
		return usecase.LogTypeConfig{}, fmt.Errorf("read config %q: %w", path, err)
	}

	config, err := parseLogTypeConfig(string(data))
	if err != nil {
		return usecase.LogTypeConfig{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if _, err := usecase.ResolveLogTypeConfig("", config); err != nil {
		return usecase.LogTypeConfig{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return config, nil
}

func parseLogTypeConfig(content string) (usecase.LogTypeConfig, error) {
	var document struct {
		LogType struct {
			Default    usecase.LogType              `toml:"default"`
			ProbeLines *int                         `toml:"probe_lines"`
			Patterns   map[usecase.LogType][]string `toml:"patterns"`
		} `toml:"log_type"`
	}
	if err := toml.Unmarshal([]byte(content), &document); err != nil {
		return usecase.LogTypeConfig{}, err
	}
	cfg := usecase.LogTypeConfig{Default: document.LogType.Default, Patterns: document.LogType.Patterns}
	if document.LogType.ProbeLines != nil {
		cfg.ProbeLines = *document.LogType.ProbeLines
		if cfg.ProbeLines < 1 {
			return cfg, fmt.Errorf("log_type.probe_lines must be 1 or greater")
		}
	}
	return cfg, nil
}
