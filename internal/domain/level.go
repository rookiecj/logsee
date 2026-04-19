package domain

import "strings"

// Level is the severity of a parsed log record. LevelUnknown is the zero
// value so callers that do not (or cannot) extract a severity still yield
// a sensible record.
type Level int8

const (
	LevelUnknown Level = iota
	LevelVerbose
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the single-letter canonical name ("V", "D", …) used in
// Android adb output and most log viewers. LevelUnknown returns "?".
func (l Level) String() string {
	switch l {
	case LevelVerbose:
		return "V"
	case LevelDebug:
		return "D"
	case LevelInfo:
		return "I"
	case LevelWarn:
		return "W"
	case LevelError:
		return "E"
	case LevelFatal:
		return "F"
	default:
		return "?"
	}
}

// LevelFromRaw parses the raw severity token from a log line or a filter
// expression. Accepts both the one-letter form ("V", "w", "E") and the
// long form ("VERBOSE", "warn", "Error", "FATAL"). Unknown tokens yield
// LevelUnknown; caller decides whether that is an error.
func LevelFromRaw(s string) Level {
	s = strings.TrimSpace(s)
	if s == "" {
		return LevelUnknown
	}
	up := strings.ToUpper(s)
	switch up {
	case "V", "VERBOSE", "TRACE", "T":
		return LevelVerbose
	case "D", "DEBUG":
		return LevelDebug
	case "I", "INFO":
		return LevelInfo
	case "W", "WARN", "WARNING":
		return LevelWarn
	case "E", "ERROR", "ERR":
		return LevelError
	case "F", "FATAL":
		return LevelFatal
	}
	return LevelUnknown
}

// MarshalText encodes the level as its short canonical letter so JSON
// output stays human-readable without a separate translation step.
func (l Level) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// UnmarshalText accepts any form [LevelFromRaw] accepts.
func (l *Level) UnmarshalText(b []byte) error {
	*l = LevelFromRaw(string(b))
	return nil
}
