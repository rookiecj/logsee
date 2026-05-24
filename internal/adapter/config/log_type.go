package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	var config usecase.LogTypeConfig
	section := ""
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(stripComment(lines[i]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}

		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return usecase.LogTypeConfig{}, fmt.Errorf("line %d: expected key = value", i+1)
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)

		switch section {
		case "log_type":
			switch key {
			case "default":
				value, err := parseString(rawValue)
				if err != nil {
					return usecase.LogTypeConfig{}, fmt.Errorf("line %d: parse log_type.default: %w", i+1, err)
				}
				config.Default = usecase.LogType(value)
			case "probe_lines":
				value, err := strconv.Atoi(rawValue)
				if err != nil {
					return usecase.LogTypeConfig{}, fmt.Errorf("line %d: parse log_type.probe_lines: %w", i+1, err)
				}
				config.ProbeLines = value
			}
		case "log_type.patterns":
			valueText := rawValue
			for !strings.Contains(valueText, "]") && i+1 < len(lines) {
				i++
				valueText += "\n" + strings.TrimSpace(stripComment(lines[i]))
			}
			values, err := parseStringArray(valueText)
			if err != nil {
				return usecase.LogTypeConfig{}, fmt.Errorf("line %d: parse log_type.patterns.%s: %w", i+1, key, err)
			}
			if config.Patterns == nil {
				config.Patterns = map[usecase.LogType][]string{}
			}
			config.Patterns[usecase.LogType(key)] = values
		}
	}
	return config, nil
}

func parseString(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(value, `"""`) && strings.HasSuffix(value, `"""`) && len(value) >= 6:
		return strings.TrimSuffix(strings.TrimPrefix(value, `"""`), `"""`), nil
	case strings.HasPrefix(value, `'''`) && strings.HasSuffix(value, `'''`) && len(value) >= 6:
		return strings.TrimSuffix(strings.TrimPrefix(value, `'''`), `'''`), nil
	case strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) && len(value) >= 2:
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return unquoted, nil
	case strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`) && len(value) >= 2:
		return strings.TrimSuffix(strings.TrimPrefix(value, `'`), `'`), nil
	default:
		return "", fmt.Errorf("expected quoted string")
	}
}

func parseStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.Contains(value, "]") {
		return nil, fmt.Errorf("expected string array")
	}
	value = value[1:strings.LastIndex(value, "]")]
	matches := stringLiteralPattern.FindAllStringSubmatch(value, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		switch {
		case match[1] != "":
			values = append(values, match[1])
		case match[2] != "":
			values = append(values, match[2])
		case match[3] != "":
			values = append(values, match[3])
		}
	}
	if strings.TrimSpace(strings.Trim(value, ",")) != "" && len(matches) == 0 {
		return nil, fmt.Errorf("expected quoted string values")
	}
	return values, nil
}

func stripComment(line string) string {
	inSingle := false
	inDouble := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

var stringLiteralPattern = regexp.MustCompile(`'''([\s\S]*?)'''|"([^"\\]*(?:\\.[^"\\]*)*)"|'([^']*)'`)
