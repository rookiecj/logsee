package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDetectLogTypeScoresADBSamples(t *testing.T) {
	logType, err := DetectLogType(context.Background(), memoryLineSampler{
		lines: []string{
			"",
			"05-24 12:34:56.789  1234  5678 I ActivityManager: start proc",
			"I/ExampleTag: legacy logcat line",
		},
	}, DefaultLogTypeConfig())
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypeADB {
		t.Fatalf("log type = %q, want %q", logType, LogTypeADB)
	}
}

func TestDetectLogTypeScoresKernelSamples(t *testing.T) {
	logType, err := DetectLogType(context.Background(), memoryLineSampler{
		lines: []string{
			"[    0.123456] usb 1-1: new high-speed USB device",
			"May 24 12:34:56 host kernel: eth0: link up",
		},
	}, DefaultLogTypeConfig())
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypeKernel {
		t.Fatalf("log type = %q, want %q", logType, LogTypeKernel)
	}
}

func TestDetectLogTypeFallsBackToPlainWhenNoPatternScores(t *testing.T) {
	logType, err := DetectLogType(context.Background(), memoryLineSampler{
		lines: []string{"just text", "without a known prefix"},
	}, DefaultLogTypeConfig())
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypePlain {
		t.Fatalf("log type = %q, want %q", logType, LogTypePlain)
	}
}

func TestDetectLogTypeFallsBackToPlainOnTie(t *testing.T) {
	config := DefaultLogTypeConfig()
	config.Patterns = map[LogType][]string{
		LogTypeADB:    {`^same`},
		LogTypeKernel: {`^same`},
	}

	logType, err := DetectLogType(context.Background(), memoryLineSampler{
		lines: []string{"same line"},
	}, config)
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypePlain {
		t.Fatalf("log type = %q, want %q", logType, LogTypePlain)
	}
}

func TestDetectLogTypeUsesConfigPatternOverrides(t *testing.T) {
	config := DefaultLogTypeConfig()
	config.Patterns = map[LogType][]string{
		LogTypeADB: {`^APPADB:`},
	}

	logType, err := DetectLogType(context.Background(), memoryLineSampler{
		lines: []string{"APPADB: custom device line"},
	}, config)
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypeADB {
		t.Fatalf("log type = %q, want %q", logType, LogTypeADB)
	}
}

func TestDetectLogTypeSamplesOnlyConfiguredNonEmptyProbeLines(t *testing.T) {
	config := DefaultLogTypeConfig()
	config.ProbeLines = 1

	logType, err := DetectLogType(context.Background(), memoryLineSampler{
		lines: []string{
			"",
			"plain first non-empty line",
			"05-24 12:34:56.789  1234  5678 I ActivityManager: start proc",
		},
	}, config)
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if logType != LogTypePlain {
		t.Fatalf("log type = %q, want %q", logType, LogTypePlain)
	}
}

func TestResolveLogTypeOptionsAppliesCLIConfigDefaultPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		cli    LogType
		config LogTypeConfig
		want   LogTypeConfig
	}{
		{
			name: "cli log type overrides config",
			cli:  LogTypeKernel,
			config: LogTypeConfig{
				Default:    LogTypeADB,
				ProbeLines: 12,
				Patterns:   map[LogType][]string{LogTypeADB: {`^config-adb`}},
			},
			want: LogTypeConfig{
				Default:    LogTypeKernel,
				ProbeLines: 12,
				Patterns: map[LogType][]string{
					LogTypePlain:  {},
					LogTypeADB:    {`^config-adb`},
					LogTypeKernel: defaultKernelPatterns(),
				},
			},
		},
		{
			name: "config overrides built-in defaults",
			config: LogTypeConfig{
				Default:    LogTypeADB,
				ProbeLines: 3,
				Patterns:   map[LogType][]string{LogTypeKernel: {`^config-kernel`}},
			},
			want: LogTypeConfig{
				Default:    LogTypeADB,
				ProbeLines: 3,
				Patterns: map[LogType][]string{
					LogTypePlain:  {},
					LogTypeADB:    defaultADBPatterns(),
					LogTypeKernel: {`^config-kernel`},
				},
			},
		},
		{
			name:   "built-in defaults apply without cli or config",
			config: LogTypeConfig{},
			want:   DefaultLogTypeConfig(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveLogTypeConfig(tt.cli, tt.config)
			if err != nil {
				t.Fatalf("resolve log type config: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("config = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveLogTypeOptionsRejectsInvalidModesAndRegex(t *testing.T) {
	tests := []struct {
		name   string
		cli    LogType
		config LogTypeConfig
	}{
		{name: "invalid cli type", cli: LogType("json")},
		{name: "invalid config default", config: LogTypeConfig{Default: LogType("json")}},
		{name: "invalid regex", config: LogTypeConfig{Patterns: map[LogType][]string{LogTypeADB: {`[`}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveLogTypeConfig(tt.cli, tt.config)
			if err == nil {
				t.Fatal("resolve log type config error = nil, want error")
			}
		})
	}
}

func TestExtractLogLevelUsesDetectedLogTypeRules(t *testing.T) {
	tests := []struct {
		name    string
		logType LogType
		line    string
		want    string
		wantOK  bool
	}{
		{
			name:    "adb modern format",
			logType: LogTypeADB,
			line:    "05-24 12:34:56.789  1234  5678 E ActivityManager: crash",
			want:    "ERROR",
			wantOK:  true,
		},
		{
			name:    "adb legacy format",
			logType: LogTypeADB,
			line:    "W/ExampleTag: slow call",
			want:    "WARN",
			wantOK:  true,
		},
		{
			name:    "kernel bracket level",
			logType: LogTypeKernel,
			line:    "[    0.123456] ERROR: failed to initialize",
			want:    "ERROR",
			wantOK:  true,
		},
		{
			name:    "plain has no structured level",
			logType: LogTypePlain,
			line:    "level: ERROR in text is not structured",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractLogLevel(tt.logType, tt.line)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("level = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

type memoryLineSampler struct {
	lines []string
	err   error
}

func (s memoryLineSampler) SampleLines(_ context.Context, maxNonEmpty int) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	if maxNonEmpty < 1 {
		return nil, errors.New("invalid maxNonEmpty")
	}
	sampled := []string{}
	for _, line := range s.lines {
		if line == "" {
			continue
		}
		sampled = append(sampled, line)
		if len(sampled) == maxNonEmpty {
			break
		}
	}
	return sampled, nil
}
