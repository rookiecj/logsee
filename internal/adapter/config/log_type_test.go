package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"logsee/internal/usecase"
)

func TestLoadLogTypeConfigReadsConfigTOML(t *testing.T) {
	path := writeConfig(t, `
[log_type]
default = "adb"
probe_lines = 7

[log_type.patterns]
adb = [
  '''^APPADB:''',
]
kernel = [
  '''^APPKERNEL:''',
]
`)

	config, err := LoadLogTypeConfig(path)
	if err != nil {
		t.Fatalf("load log type config: %v", err)
	}

	if config.Default != usecase.LogTypeADB {
		t.Fatalf("default = %q, want %q", config.Default, usecase.LogTypeADB)
	}
	if config.ProbeLines != 7 {
		t.Fatalf("probe lines = %d, want 7", config.ProbeLines)
	}
	if got, want := config.Patterns[usecase.LogTypeADB], []string{`^APPADB:`}; !equalStrings(got, want) {
		t.Fatalf("adb patterns = %#v, want %#v", got, want)
	}
	if got, want := config.Patterns[usecase.LogTypeKernel], []string{`^APPKERNEL:`}; !equalStrings(got, want) {
		t.Fatalf("kernel patterns = %#v, want %#v", got, want)
	}
}

func TestLoadLogTypeConfigFailsForInvalidRegexWithClearError(t *testing.T) {
	path := writeConfig(t, `
[log_type.patterns]
adb = ['''[''']
`)

	_, err := LoadLogTypeConfig(path)
	if err == nil {
		t.Fatal("load log type config error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid regex") || !strings.Contains(err.Error(), "adb") {
		t.Fatalf("error = %q, want clear invalid regex message for adb", err.Error())
	}
}

func TestResolveConfigPathUsesExplicitPathBeforeDefaultHomePath(t *testing.T) {
	home := t.TempDir()

	explicit := ResolveConfigPath("/tmp/custom.toml", home)
	if explicit != "/tmp/custom.toml" {
		t.Fatalf("explicit path = %q, want /tmp/custom.toml", explicit)
	}

	defaultPath := ResolveConfigPath("", home)
	want := filepath.Join(home, ".local", "logsee", "config.toml")
	if defaultPath != want {
		t.Fatalf("default path = %q, want %q", defaultPath, want)
	}
}

func TestLoadLogTypeConfigReturnsEmptyWhenDefaultConfigIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".local", "logsee", "config.toml")

	config, err := LoadLogTypeConfig(path)
	if err != nil {
		t.Fatalf("load missing default config: %v", err)
	}

	if config.Default != "" || config.ProbeLines != 0 || len(config.Patterns) != 0 {
		t.Fatalf("missing config = %#v, want empty config overrides", config)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
