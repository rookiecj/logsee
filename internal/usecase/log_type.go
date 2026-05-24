package usecase

import (
	"context"
	"fmt"
	"regexp"
)

type LogType string

const (
	LogTypeAuto   LogType = "auto"
	LogTypePlain  LogType = "plain"
	LogTypeADB    LogType = "adb"
	LogTypeKernel LogType = "kernel"
)

const DefaultLogTypeProbeLines = 200

type LogTypeConfig struct {
	Default    LogType
	ProbeLines int
	Patterns   map[LogType][]string
}

type LogLineSampler interface {
	SampleLines(context.Context, int) ([]string, error)
}

func DefaultLogTypeConfig() LogTypeConfig {
	return LogTypeConfig{
		Default:    LogTypeAuto,
		ProbeLines: DefaultLogTypeProbeLines,
		Patterns: map[LogType][]string{
			LogTypePlain:  {},
			LogTypeADB:    defaultADBPatterns(),
			LogTypeKernel: defaultKernelPatterns(),
		},
	}
}

func ResolveLogTypeConfig(cliDefault LogType, config LogTypeConfig) (LogTypeConfig, error) {
	resolved := DefaultLogTypeConfig()

	if config.Default != "" {
		resolved.Default = config.Default
	}
	if config.ProbeLines != 0 {
		resolved.ProbeLines = config.ProbeLines
	}
	for logType, patterns := range config.Patterns {
		if resolved.Patterns == nil {
			resolved.Patterns = map[LogType][]string{}
		}
		resolved.Patterns[logType] = append([]string(nil), patterns...)
	}
	if cliDefault != "" {
		resolved.Default = cliDefault
	}

	if err := validateLogTypeConfig(resolved); err != nil {
		return LogTypeConfig{}, err
	}
	return resolved, nil
}

func DetectLogType(ctx context.Context, sampler LogLineSampler, config LogTypeConfig) (LogType, error) {
	resolved, err := ResolveLogTypeConfig("", config)
	if err != nil {
		return "", err
	}
	if resolved.Default != LogTypeAuto {
		return resolved.Default, nil
	}
	if sampler == nil {
		return "", fmt.Errorf("log type detection requires a line sampler")
	}

	patterns, err := compileLogTypePatterns(resolved.Patterns)
	if err != nil {
		return "", err
	}
	lines, err := sampler.SampleLines(ctx, resolved.ProbeLines)
	if err != nil {
		return "", fmt.Errorf("sample log lines for type detection: %w", err)
	}

	scores := map[LogType]int{}
	for _, line := range lines {
		for logType, typedPatterns := range patterns {
			for _, pattern := range typedPatterns {
				if pattern.MatchString(line) {
					scores[logType]++
				}
			}
		}
	}

	winner := LogTypePlain
	winningScore := 0
	tied := false
	for _, logType := range []LogType{LogTypeADB, LogTypeKernel} {
		score := scores[logType]
		switch {
		case score > winningScore:
			winner = logType
			winningScore = score
			tied = false
		case score > 0 && score == winningScore:
			tied = true
		}
	}
	if winningScore == 0 || tied {
		return LogTypePlain, nil
	}
	return winner, nil
}

func ExtractLogLevel(logType LogType, line string) (string, bool) {
	switch logType {
	case LogTypeADB:
		if match := adbModernLevelPattern.FindStringSubmatch(line); len(match) == 2 {
			return normalizeADBLevel(match[1]), true
		}
		if match := adbLegacyLevelPattern.FindStringSubmatch(line); len(match) == 2 {
			return normalizeADBLevel(match[1]), true
		}
	case LogTypeKernel:
		if match := kernelBracketLevelPattern.FindStringSubmatch(line); len(match) == 2 {
			return match[1], true
		}
	}
	return "", false
}

func validateLogTypeConfig(config LogTypeConfig) error {
	if !isValidLogTypeMode(config.Default, true) {
		return fmt.Errorf("invalid log type default %q: want auto, plain, adb, or kernel", config.Default)
	}
	if config.ProbeLines < 1 {
		return fmt.Errorf("log_type.probe_lines must be 1 or greater")
	}
	for logType := range config.Patterns {
		if !isValidLogTypeMode(logType, false) {
			return fmt.Errorf("invalid log type pattern key %q: want plain, adb, or kernel", logType)
		}
	}
	if _, err := compileLogTypePatterns(config.Patterns); err != nil {
		return err
	}
	return nil
}

func compileLogTypePatterns(patterns map[LogType][]string) (map[LogType][]*regexp.Regexp, error) {
	compiled := map[LogType][]*regexp.Regexp{}
	for logType, typedPatterns := range patterns {
		for _, pattern := range typedPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex for log type %s pattern %q: %w", logType, pattern, err)
			}
			compiled[logType] = append(compiled[logType], re)
		}
	}
	return compiled, nil
}

func isValidLogTypeMode(logType LogType, allowAuto bool) bool {
	switch logType {
	case LogTypePlain, LogTypeADB, LogTypeKernel:
		return true
	case LogTypeAuto:
		return allowAuto
	default:
		return false
	}
}

func defaultADBPatterns() []string {
	return []string{
		`^\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+\s+\d+\s+\d+\s+[VDIWEF]\s+`,
		`^[VDIWEF]/[^:]+:\s+`,
	}
}

func defaultKernelPatterns() []string {
	return []string{
		`^\[[\s\d.]+\]\s+`,
		`^\w{3}\s+\d+\s+\d{2}:\d{2}:\d{2}\s+\S+\s+kernel:`,
	}
}

func normalizeADBLevel(level string) string {
	switch level {
	case "V":
		return "VERBOSE"
	case "D":
		return "DEBUG"
	case "I":
		return "INFO"
	case "W":
		return "WARN"
	case "E":
		return "ERROR"
	case "F":
		return "FATAL"
	default:
		return ""
	}
}

var (
	adbModernLevelPattern     = regexp.MustCompile(`^\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+\s+\d+\s+\d+\s+([VDIWEF])\s+`)
	adbLegacyLevelPattern     = regexp.MustCompile(`^([VDIWEF])/[^:]+:\s+`)
	kernelBracketLevelPattern = regexp.MustCompile(`^\[[\s\d.]+\]\s+([A-Z]+):`)
)
