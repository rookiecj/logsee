package filter

import "testing"

func TestDetectLogFormat_JournalDominant(t *testing.T) {
	lines := []string{
		`2024-04-19T14:22:10.001234+0900 build01 systemd[1]: Starting nginx.service`,
		`2024-04-19T14:22:10.045612+0900 build01 nginx[4521]: config syntax ok`,
		`2024-04-19T14:22:10.189234+0900 build01 systemd[1]: Started nginx.service`,
	}
	if got := DetectLogFormat(lines); got != FormatSystemdJournal {
		t.Errorf("DetectLogFormat = %v, want FormatSystemdJournal", got)
	}
}

func TestDetectLogFormat_AndroidStillDominantWithoutJournal(t *testing.T) {
	lines := []string{
		`04-19 14:23:40.012  1245  1301 I SystemServer: Entered`,
		`04-19 14:23:40.145 12345 12345 D ActivityThread: handleBindApplication`,
	}
	if got := DetectLogFormat(lines); got != FormatAndroid {
		t.Errorf("DetectLogFormat = %v, want FormatAndroid", got)
	}
}

func TestDetectLogFormat_MixedFormatsTieFallsToUnknown(t *testing.T) {
	lines := []string{
		`04-19 14:23:40.012  1245  1301 I SystemServer: Entered`,
		`2024-04-19T14:22:10.001234+0900 build01 systemd[1]: Started`,
	}
	if got := DetectLogFormat(lines); got != FormatUnknown {
		t.Errorf("mixed formats should tie to Unknown, got %v", got)
	}
}

func TestExtractRawLevel_JournalFormatReturnsUnsupported(t *testing.T) {
	// short-iso has no PRIORITY; Level extraction for this format is
	// documented as unsupported.
	raw, ok := ExtractRawLevel(
		`2024-04-19T14:22:10.001234+0900 build01 systemd[1]: started`,
		FormatSystemdJournal,
	)
	if ok || raw != "" {
		t.Errorf("journal level extraction should return !ok, got raw=%q ok=%v", raw, ok)
	}
}

func TestDefaultPatternConfig_JournalCompiles(t *testing.T) {
	cfg := DefaultPatternConfig()
	if cfg.JournalHead == "" {
		t.Fatal("DefaultPatternConfig.JournalHead must be populated")
	}
	if _, err := CompilePatternConfig(cfg); err != nil {
		t.Errorf("default cfg must compile: %v", err)
	}
}
