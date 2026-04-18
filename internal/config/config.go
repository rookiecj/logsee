package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"git.inpt.fr/42dottools/log/internal/filter"
)

const configFileName = "config.toml"

// ConfigHelp is shown next to --config / --print-default-config in the CLI usage text.
const ConfigHelp = `Configuration (TOML):
  Default file: $HOME/.local/logsee/config.toml (override with --config).
  If the file is missing, built-in defaults are used (same as --print-default-config).
  Priority for log-type options: CLI flags (--log-type, --log-type-probe-lines) > config > defaults.
  Sections:
    [log_type] default — "auto" | "plain" | "adb" (line shape for level tag / probing).
    [log_type] probe_lines — non-empty lines to sample when default is "auto" (>= 1).
    [log_type.patterns] — regexes: bracket_head, adb_head_time, adb_head_threadtime
      (used to score line formats and extract Android single-letter levels; first capture group where relevant).
    [highlight_color_names] — optional; name → "0".."255" for highlight queries like token#name.
      Built-in names are always available; config overrides the same key.
    --print-default-config: full commented TOML (Korean) for [log_type], patterns, and highlight_color_names.`

type LogTypeConfig struct {
	Default    string               `toml:"default"`
	ProbeLines int                  `toml:"probe_lines"`
	Patterns   filter.PatternConfig `toml:"patterns"`
}

type Config struct {
	LogType             LogTypeConfig     `toml:"log_type"`
	HighlightColorNames map[string]string `toml:"highlight_color_names"`
}

func Default() Config {
	return Config{
		LogType: LogTypeConfig{
			Default:    "auto",
			ProbeLines: 32,
			Patterns:   filter.DefaultPatternConfig(),
		},
		HighlightColorNames: cloneStringMap(BuiltinHighlightColorNames()),
	}
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "logsee", configFileName), nil
}

const defaultConfigHeader = `# logsee configuration (TOML)
#
# 설치: mkdir -p ~/.local/logsee && cp config.example.toml ~/.local/logsee/config.toml
# 또는: logsee --print-default-config > ~/.local/logsee/config.toml
#
# 이 파일이 없으면 아래와 동일한 값이 내장 기본값으로 씁니다.
# 우선순위: CLI (--log-type, --log-type-probe-lines) > 이 파일의 [log_type] > 그 외 내장 기본.
#
# 구성 요약:
#   [log_type]          — 줄 형식·프로브(레벨 태그 추출 / auto 판별)
#   [log_type.patterns] — 형식별 정규식(adb는 한 글자 레벨, bracket은 태그 식별자 등)
#   [highlight_color_names] — 하이라이트에서 #이름 으로 쓸 ANSI 256 색 이름
#

`

// defaultLogTypeTOML returns [log_type] and [log_type.patterns] with Korean comments.
// Values are taken from Default() so they stay in sync with runtime defaults.
func defaultLogTypeTOML() string {
	lt := Default().LogType
	p := lt.Patterns
	var b strings.Builder
	b.WriteString("# --- [log_type] 로그 줄 형식 ---\n")
	b.WriteString("#\n")
	b.WriteString("# default — 줄 앞부분을 어떻게 해석할지:\n")
	b.WriteString("#   \"auto\"  비어 있지 않은 앞쪽 probe_lines 줄로 plain / adb 형을 추론\n")
	b.WriteString("#   \"plain\" 레벨 태그 없이 한 줄 텍스트로 취급\n")
	b.WriteString("#   \"adb\"   Android logcat 스타일 헤더(날짜·시간·PID·레벨 한 글자 등)\n")
	b.WriteString("#\n")
	b.WriteString("# probe_lines — default가 auto일 때 샘플링에 쓰는 줄 수(>=1)\n")
	b.WriteString("#\n")
	b.WriteString("[log_type]\n")
	fmt.Fprintf(&b, "default = %q  # auto | plain | adb\n", lt.Default)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "probe_lines = %d  # auto 프로브 시 상단에서 읽는 줄 수\n", lt.ProbeLines)
	b.WriteString("\n")
	b.WriteString("# --- [log_type.patterns] 형식 판별·레벨 추출용 정규식 ---\n")
	b.WriteString("# bracket_head     — [..] … Name: 에서 Name(첫 캡처) — 레벨/태그 후보\n")
	b.WriteString("# adb_head_time    — 날짜+시간 형 Android 헤더의 레벨 한 글자(V/D/I/W/E/F)\n")
	b.WriteString("# adb_head_threadtime — MM-dd 시간 형 threadtime 헤더의 레벨 한 글자\n")
	b.WriteString("#\n")
	b.WriteString("[log_type.patterns]\n")
	fmt.Fprintf(&b, "bracket_head = %q\n", p.BracketHead)
	fmt.Fprintf(&b, "adb_head_time = %q\n", p.AndroidHeadTime)
	fmt.Fprintf(&b, "adb_head_threadtime = %q\n", p.AndroidHeadThreadtime)
	return b.String()
}

// defaultHighlightColorNamesTOML returns [highlight_color_names] with one-line Korean comments per entry.
func defaultHighlightColorNamesTOML() string {
	type row struct {
		key, val, comment string
	}
	rows := []row{
		{"red", "196", "에러·경고 강조"},
		{"green", "40", "성공·정상"},
		{"yellow", "226", "주의·강조"},
		{"blue", "27", "정보"},
		{"magenta", "201", "특수 키워드"},
		{"cyan", "51", "구분선·디버그"},
		{"orange", "208", "경고(주황)"},
		{"purple", "93", "보조 강조"},
		{"gray", "245", "흐린 텍스트(밝은 회색 배경)"},
		{"grey", "245", "gray와 동일(영국식 철자)"},
	}
	var b strings.Builder
	b.WriteString("\n# --- highlight_color_names: 하이라이트에서 #이름 으로 쓰는 ANSI 256 색(0–255) ---\n")
	b.WriteString("# TUI에서 / 로 검색(highlight) 확정 시: needle#숫자 또는 needle#이름 (토큰 규칙은 필터와 동일).\n")
	b.WriteString("# 이 섹션을 빼도 동일한 내장 이름이 적용됩니다. 여기서 값을 바꾸면 그 이름의 색만 덮어씁니다.\n")
	b.WriteString("[highlight_color_names]\n")
	for _, r := range rows {
		b.WriteString(r.key)
		b.WriteString(` = "`)
		b.WriteString(r.val)
		b.WriteString(`"  # `)
		b.WriteString(r.comment)
		b.WriteByte('\n')
	}
	return b.String()
}

// DefaultConfigTOML returns the default config file text: header, [log_type] + patterns, [highlight_color_names],
// all with Korean comments and values matching Default().
func DefaultConfigTOML() (string, error) {
	return defaultConfigHeader + defaultLogTypeTOML() + defaultHighlightColorNamesTOML(), nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, err
	}
	var raw Config
	if err := toml.Unmarshal(b, &raw); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	merge(&cfg, raw)
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

func merge(dst *Config, src Config) {
	if src.LogType.Default != "" {
		dst.LogType.Default = src.LogType.Default
	}
	if src.LogType.ProbeLines > 0 {
		dst.LogType.ProbeLines = src.LogType.ProbeLines
	}
	if src.LogType.Patterns.BracketHead != "" {
		dst.LogType.Patterns.BracketHead = src.LogType.Patterns.BracketHead
	}
	if src.LogType.Patterns.AndroidHeadTime != "" {
		dst.LogType.Patterns.AndroidHeadTime = src.LogType.Patterns.AndroidHeadTime
	}
	if src.LogType.Patterns.AndroidHeadThreadtime != "" {
		dst.LogType.Patterns.AndroidHeadThreadtime = src.LogType.Patterns.AndroidHeadThreadtime
	}
	if len(src.HighlightColorNames) > 0 {
		if dst.HighlightColorNames == nil {
			dst.HighlightColorNames = make(map[string]string)
		}
		for k, v := range src.HighlightColorNames {
			dst.HighlightColorNames[k] = v
		}
	}
}

func validate(cfg Config) error {
	if cfg.LogType.ProbeLines < 1 {
		return fmt.Errorf("log_type.probe_lines must be >= 1")
	}
	if _, err := filter.CompilePatternConfig(cfg.LogType.Patterns); err != nil {
		return err
	}
	for name, val := range cfg.HighlightColorNames {
		if err := validateANSIColor256(val); err != nil {
			return fmt.Errorf("highlight_color_names[%q]: %w", name, err)
		}
	}
	return nil
}

// BuiltinHighlightColorNames returns the built-in ANSI256 name → index string map (copied).
func BuiltinHighlightColorNames() map[string]string {
	return map[string]string{
		"red":     "196",
		"green":   "40",
		"yellow":  "226",
		"blue":    "27",
		"magenta": "201",
		"cyan":    "51",
		"orange":  "208",
		"purple":  "93",
		"gray":    "245",
		"grey":    "245",
	}
}

// MergeHighlightColorNames returns built-in entries plus override (keys compared case-insensitively).
// override may be nil.
func MergeHighlightColorNames(override map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range BuiltinHighlightColorNames() {
		out[strings.ToLower(k)] = v
	}
	for k, v := range override {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[strings.ToLower(k)] = v
	}
	return out
}

func validateANSIColor256(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("empty color value")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("not a decimal ANSI index: %w", err)
	}
	if n < 0 || n > 255 {
		return fmt.Errorf("ANSI index must be 0..255, got %d", n)
	}
	return nil
}

// FileName returns the default config file basename (for help text).
func FileName() string { return configFileName }

// IsLegacyJSONName reports whether path looks like the old JSON config filename.
func IsLegacyJSONName(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".json")
}
