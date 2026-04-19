package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestLoad_ConfigMergesWithDefaults(t *testing.T) {
	// Given: config file that overrides log type default only
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[log_type]
default = "adb"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// When: load config
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Then: default overridden and other fields keep defaults
	if cfg.LogType.Default != "adb" {
		t.Fatalf("want adb, got %q", cfg.LogType.Default)
	}
	if cfg.LogType.ProbeLines != 32 {
		t.Fatalf("want default probe 32, got %d", cfg.LogType.ProbeLines)
	}
}

func TestLoad_historyDirFromConfig(t *testing.T) {
	// Given: config sets [history] dir
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[history]
dir = "/tmp/logsee-state"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// When: load
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Then: history dir reflected, unrelated defaults preserved
	if cfg.History.Dir != "/tmp/logsee-state" {
		t.Fatalf("want /tmp/logsee-state, got %q", cfg.History.Dir)
	}
	if cfg.LogType.Default != "auto" {
		t.Fatalf("log_type default should stay auto, got %q", cfg.LogType.Default)
	}
}

func TestLoad_historyDirDefaultIsEmpty(t *testing.T) {
	// Given: config without [history]
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[log_type]`+"\n"+`default = "plain"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// When: load
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Then: History.Dir is empty (caller falls back to userstate.DefaultStateDir)
	if cfg.History.Dir != "" {
		t.Fatalf("want empty History.Dir, got %q", cfg.History.Dir)
	}
}

func TestLoad_highlightColorNames_invalidValue(t *testing.T) {
	// Given: config with out-of-range ANSI index
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[highlight_color_names]
bad = "999"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// When: load config
	// Then: validation fails
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for highlight_color_names out of range")
	}
}

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	// Given: missing config file path
	path := filepath.Join(t.TempDir(), "nope.toml")
	// When: load config
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Then: returns defaults
	if cfg.LogType.Default != "auto" {
		t.Fatalf("want auto, got %q", cfg.LogType.Default)
	}
}

func TestDefaultConfigTOML_ParseMatchesDefault(t *testing.T) {
	// Given: generated default TOML document
	doc, err := DefaultConfigTOML()
	if err != nil {
		t.Fatal(err)
	}
	// When: unmarshaled
	var raw Config
	if err := toml.Unmarshal([]byte(doc), &raw); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Then: merged with empty equals full default (file is self-contained)
	cfg := Config{}
	merge(&cfg, raw)
	if err := validate(cfg); err != nil {
		t.Fatal(err)
	}
	want := Default()
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("parsed default doc != Default(): got %+v want %+v", cfg, want)
	}
}

func TestFileName(t *testing.T) {
	if !strings.HasSuffix(FileName(), ".toml") {
		t.Fatalf("unexpected FileName: %q", FileName())
	}
}

func TestIsLegacyJSONName(t *testing.T) {
	if !IsLegacyJSONName("/home/u/.local/logsee/config.json") {
		t.Fatal("expected legacy json path")
	}
	if IsLegacyJSONName("/home/u/.local/logsee/config.toml") {
		t.Fatal("toml should not be legacy")
	}
}
