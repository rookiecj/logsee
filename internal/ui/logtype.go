package ui

import (
	"fmt"
	"strings"

	"git.inpt.fr/42dottools/log/internal/filter"
)

// LogTypeKind selects how filter.LogFormat is chosen for reserved tag level extraction (PRD §7.1).
type LogTypeKind int

const (
	// LogTypeAuto: sample first non-empty lines and pick adb, else plain.
	LogTypeAuto LogTypeKind = iota
	// LogTypePlain: no structured level extraction.
	LogTypePlain
	// LogTypeADB: Android logcat "-v time"-style lines.
	LogTypeADB
)

// LogTypeOpts configures CLI --log-type and --log-type-probe-lines.
type LogTypeOpts struct {
	Kind       LogTypeKind
	ProbeLines int
}

// DefaultLogTypeOpts returns auto detection with 32 non-empty probe lines.
func DefaultLogTypeOpts() LogTypeOpts {
	return LogTypeOpts{Kind: LogTypeAuto, ProbeLines: 32}
}

// ParseLogTypeKind parses CLI values: auto, plain, adb (android).
func ParseLogTypeKind(s string) (LogTypeKind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto":
		return LogTypeAuto, nil
	case "plain":
		return LogTypePlain, nil
	case "adb", "android":
		return LogTypeADB, nil
	default:
		return 0, fmt.Errorf("unknown log type %q (want auto, plain, adb)", s)
	}
}

func initialLogFormat(kind LogTypeKind) (filter.LogFormat, bool) {
	switch kind {
	case LogTypePlain:
		return filter.FormatPlain, true
	case LogTypeADB:
		return filter.FormatAndroid, true
	case LogTypeAuto:
		return filter.FormatPlain, false
	default:
		return filter.FormatPlain, false
	}
}

func formatShortName(f filter.LogFormat) string {
	switch f {
	case filter.FormatAndroid:
		return "adb"
	case filter.FormatPlain:
		return "plain"
	default:
		return "plain"
	}
}
