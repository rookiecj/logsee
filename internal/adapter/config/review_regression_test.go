package config

import "testing"

func TestReviewReadmeConfig(t *testing.T) {
	_, err := LoadLogTypeConfig("../../../testdata/config/readme.toml")
	if err != nil {
		t.Fatalf("README config rejected: %v", err)
	}
}

func TestTOMLRegexStringEscapes(t *testing.T) {
	cfg, err := parseLogTypeConfig("[log_type.patterns]\nadb = [\"^\\\\d+#[A-Z]$\"]\n")
	if err != nil || cfg.Patterns["adb"][0] != `^\d+#[A-Z]$` {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	for _, body := range []string{"[log_type]\nprobe_lines=0", "[log_type]\nprobe_lines=-1", "[log_type.patterns]\nadb=[123]", "[log_type]\ndefault=\"plain\"\ndefault=\"adb\""} {
		if _, err := parseLogTypeConfig(body); err == nil {
			t.Errorf("invalid config accepted: %s", body)
		}
	}
}
