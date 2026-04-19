package filter

import (
	"fmt"
	"regexp"
	"strings"
)

// LogFormat is the detected log line shape used for reserved tag "level" extraction.
type LogFormat int

const (
	// FormatUnknown: no clear winner from probe lines (use EffectiveFormatFromDetect for auto mode).
	FormatUnknown LogFormat = iota
	// FormatBracket: "[timestamp] LEVEL: message" (LEVEL is the token before the first ':' after ']').
	FormatBracket
	// FormatAndroid: logcat "-v time"-style "date time PID-TID tag package D message".
	FormatAndroid
	// FormatPlain: no structured level extraction for reserved tag "level" (ExtractRawLevel always fails).
	FormatPlain
	// FormatSystemdJournal: `journalctl -o short-iso` / `short-iso-precise`
	// — "2024-04-19T14:24:10(.123456)?(+0900|Z) host tag[pid]: msg".
	// Level extraction is not supported for this format (short-iso has no
	// PRIORITY field); ExtractRawLevel returns ok=false.
	FormatSystemdJournal
)

const formatProbeLines = 32

// PatternConfig defines regex patterns for log type scoring and Android level extraction.
type PatternConfig struct {
	BracketHead           string `toml:"bracket_head" json:"bracket_head"`
	AndroidHeadTime       string `toml:"adb_head_time" json:"adb_head_time"`
	AndroidHeadThreadtime string `toml:"adb_head_threadtime" json:"adb_head_threadtime"`
	JournalHead           string `toml:"journal_head" json:"journal_head"`
}

// DefaultPatternConfig returns built-in patterns used when no config is provided.
func DefaultPatternConfig() PatternConfig {
	return PatternConfig{
		BracketHead:           `^\[[^\]]+\]\s+([A-Za-z][A-Za-z0-9_]*)\s*:`,
		AndroidHeadTime:       `^\d{4}-\d{2}-\d{2}\s+\d{1,2}:\d{2}:\d{2}\.\d{3}\s+\d+-\d+\s+\S+\s+\S+\s+([VDIWEF])\s`,
		AndroidHeadThreadtime: `^\d{2}-\d{2}\s+\d{1,2}:\d{2}:\d{2}\.\d{3}\s+\d+\s+\d+\s+([VDIWEF])\s+\S+:`,
		JournalHead:           `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d{4}|Z)\s+\S+\s+[^\s\[]+(?:\[\d+\])?:`,
	}
}

type compiledPatterns struct {
	bracketHead           *regexp.Regexp
	androidHeadTime       *regexp.Regexp
	androidHeadThreadtime *regexp.Regexp
	journalHead           *regexp.Regexp
}

var activePatterns = mustCompilePatterns(DefaultPatternConfig())

func mustCompilePatterns(cfg PatternConfig) compiledPatterns {
	p, err := compilePatterns(cfg)
	if err != nil {
		panic(err)
	}
	return p
}

func compilePatterns(cfg PatternConfig) (compiledPatterns, error) {
	if strings.TrimSpace(cfg.BracketHead) == "" {
		return compiledPatterns{}, fmt.Errorf("empty pattern bracket_head")
	}
	if strings.TrimSpace(cfg.AndroidHeadTime) == "" {
		return compiledPatterns{}, fmt.Errorf("empty pattern adb_head_time")
	}
	if strings.TrimSpace(cfg.AndroidHeadThreadtime) == "" {
		return compiledPatterns{}, fmt.Errorf("empty pattern adb_head_threadtime")
	}
	bracketHead, err := regexp.Compile(cfg.BracketHead)
	if err != nil {
		return compiledPatterns{}, fmt.Errorf("compile bracket_head: %w", err)
	}
	androidHeadTime, err := regexp.Compile(cfg.AndroidHeadTime)
	if err != nil {
		return compiledPatterns{}, fmt.Errorf("compile adb_head_time: %w", err)
	}
	androidHeadThreadtime, err := regexp.Compile(cfg.AndroidHeadThreadtime)
	if err != nil {
		return compiledPatterns{}, fmt.Errorf("compile adb_head_threadtime: %w", err)
	}
	journalPattern := cfg.JournalHead
	if strings.TrimSpace(journalPattern) == "" {
		// Journal support is additive; keep the field optional so
		// existing configs without the key continue to compile.
		journalPattern = DefaultPatternConfig().JournalHead
	}
	journalHead, err := regexp.Compile(journalPattern)
	if err != nil {
		return compiledPatterns{}, fmt.Errorf("compile journal_head: %w", err)
	}
	return compiledPatterns{
		bracketHead:           bracketHead,
		androidHeadTime:       androidHeadTime,
		androidHeadThreadtime: androidHeadThreadtime,
		journalHead:           journalHead,
	}, nil
}

// CompilePatternConfig validates a pattern config and returns error details when invalid.
func CompilePatternConfig(cfg PatternConfig) (PatternConfig, error) {
	if _, err := compilePatterns(cfg); err != nil {
		return PatternConfig{}, err
	}
	return cfg, nil
}

// SetPatternConfig replaces runtime patterns used by DetectLogFormat/ExtractRawLevel.
func SetPatternConfig(cfg PatternConfig) error {
	p, err := compilePatterns(cfg)
	if err != nil {
		return err
	}
	activePatterns = p
	return nil
}

// DetectLogFormat scores the first non-empty lines among sampleLines and returns the dominant format.
// At most formatProbeLines non-empty lines are considered.
func DetectLogFormat(sampleLines []string) LogFormat {
	return DetectLogFormatN(sampleLines, formatProbeLines)
}

// DetectLogFormatN is like DetectLogFormat but limits the probe to maxNonEmpty non-empty lines (minimum 1).
func DetectLogFormatN(sampleLines []string, maxNonEmpty int) LogFormat {
	if maxNonEmpty < 1 {
		maxNonEmpty = formatProbeLines
	}
	var a, b, j int
	n := 0
	for _, line := range sampleLines {
		if n >= maxNonEmpty {
			break
		}
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		n++
		if matchAndroidHead(s, activePatterns) {
			a++
		}
		if activePatterns.journalHead.MatchString(s) {
			j++
		}
		if activePatterns.bracketHead.MatchString(s) {
			b++
		}
	}
	// Pick the highest-scoring format; ties return Unknown so callers can
	// apply their own tie-break (e.g. EffectiveFormatFromDetect).
	best := FormatUnknown
	bestScore := 0
	if a > bestScore {
		best, bestScore = FormatAndroid, a
	}
	if j > bestScore {
		best, bestScore = FormatSystemdJournal, j
	}
	if b > bestScore {
		best, bestScore = FormatBracket, b
	}
	// Tie detection: any other format equal to bestScore and >0 means
	// ambiguous — fall back to Unknown.
	if bestScore > 0 {
		ties := 0
		for _, s := range [...]int{a, j, b} {
			if s == bestScore {
				ties++
			}
		}
		if ties > 1 {
			return FormatUnknown
		}
	}
	return best
}

func matchAndroidHead(line string, p compiledPatterns) bool {
	return p.androidHeadTime.MatchString(line) || p.androidHeadThreadtime.MatchString(line)
}

func extractAndroidLevel(line string, p compiledPatterns) (string, bool) {
	if m := p.androidHeadTime.FindStringSubmatch(line); len(m) >= 2 {
		return m[1], true
	}
	if m := p.androidHeadThreadtime.FindStringSubmatch(line); len(m) >= 2 {
		return m[1], true
	}
	return "", false
}

// EffectiveFormatFromDetect maps detection result to a concrete format for filtering.
// A tie or no match (FormatUnknown) becomes FormatPlain — no structural level patterns.
// Bracket also collapses to Plain (no level-extraction rule is shipped for it by default).
// FormatSystemdJournal maps through unchanged; callers that want to fall back
// do it explicitly.
func EffectiveFormatFromDetect(d LogFormat) LogFormat {
	if d == FormatUnknown || d == FormatBracket {
		return FormatPlain
	}
	return d
}

// ExtractRawLevel returns the raw severity token from the line for the given format hint.
// ok is false when the pattern for fmt does not match (or fmt is unknown and neither pattern matches).
func ExtractRawLevel(line string, fmt LogFormat) (raw string, ok bool) {
	switch fmt {
	case FormatPlain:
		return "", false
	case FormatAndroid:
		return extractAndroidLevel(line, activePatterns)
	case FormatBracket:
		m := activePatterns.bracketHead.FindStringSubmatch(line)
		if len(m) < 2 {
			return "", false
		}
		return m[1], true
	default:
		if m := activePatterns.bracketHead.FindStringSubmatch(line); len(m) >= 2 {
			return m[1], true
		}
		if raw, ok := extractAndroidLevel(line, activePatterns); ok {
			return raw, true
		}
		return "", false
	}
}

// normalizeSeverity maps a raw token (line or filter) to a canonical uppercase name for comparison.
func normalizeSeverity(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 1 {
		switch strings.ToUpper(s) {
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
		}
	}
	up := strings.ToUpper(s)
	switch up {
	case "V", "VERBOSE":
		return "VERBOSE"
	case "D", "DEBUG":
		return "DEBUG"
	case "I", "INFO":
		return "INFO"
	case "W", "WARN", "WARNING":
		return "WARN"
	case "E", "ERROR":
		return "ERROR"
	case "F", "FATAL":
		return "FATAL"
	case "T", "TRACE":
		return "TRACE"
	default:
		return up
	}
}

func severityEquals(a, b string, ignoreCase bool) bool {
	ca, cb := normalizeSeverity(a), normalizeSeverity(b)
	if ignoreCase {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}
