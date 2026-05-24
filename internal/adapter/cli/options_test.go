package cli

import (
	"context"
	"strings"
	"testing"

	"logsee/internal/usecase"
)

func TestUsageDocumentsLogTypeOptionAsInputLogType(t *testing.T) {
	usage := Usage()

	for _, want := range []string{
		"--log-type <auto|plain|adb|kernel>",
		"입력 로그의 타입",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage %q does not contain %q", usage, want)
		}
	}
}

func TestUsageDocumentsVersionOption(t *testing.T) {
	// Given
	usage := Usage()

	// When / Then
	for _, want := range []string{
		"--version",
		"print version and exit",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage %q does not contain %q", usage, want)
		}
	}
}

func TestUsageDocumentsOutDefaultPattern(t *testing.T) {
	// Given
	usage := Usage()

	// When / Then
	for _, want := range []string{
		"--out <path>",
		"default: ./logsee-YYYYMMDD-HHMMSS.log",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage %q does not contain %q", usage, want)
		}
	}
}

func TestUsageDocumentsInputFileArgumentAndSTDIOFallback(t *testing.T) {
	usage := Usage()

	for _, want := range []string{
		"Usage: logsee [options] [input-file|-]",
		"[input-file|-]",
		"로그 파일 지정",
		"지정하지 않거나 -이면 STDIO",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage %q does not contain %q", usage, want)
		}
	}
}

func TestParseArgsReadsEPIC002FlagsAndInputPath(t *testing.T) {
	options, err := ParseArgs([]string{
		"--out", "session.log",
		"--config", "config.toml",
		"--log-type", "kernel",
		"--ignore-case",
		"input.log",
	})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}

	if options.InputPath != "input.log" {
		t.Fatalf("input path = %q, want input.log", options.InputPath)
	}
	if options.OutPath != "session.log" {
		t.Fatalf("out path = %q, want session.log", options.OutPath)
	}
	if options.ConfigPath != "config.toml" {
		t.Fatalf("config path = %q, want config.toml", options.ConfigPath)
	}
	if options.LogType != "kernel" {
		t.Fatalf("log type = %q, want kernel", options.LogType)
	}
	if !options.IgnoreCase {
		t.Fatal("ignore case = false, want true")
	}
}

func TestParseArgsReadsVersionFlag(t *testing.T) {
	// Given / When
	options, err := ParseArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}

	// Then
	if !options.Version {
		t.Fatal("version = false, want true")
	}
}

func TestParseArgsPreservesPositionalInputPathContract(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "omitted input stays empty for stdio mode resolution",
			args: nil,
			want: "",
		},
		{
			name: "dash input stays dash for explicit stdio mode resolution",
			args: []string{"-"},
			want: "-",
		},
		{
			name: "one file path is preserved for file mode startup",
			args: []string{"/var/log/app.log"},
			want: "/var/log/app.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options, err := ParseArgs(tt.args)
			if err != nil {
				t.Fatalf("parse args: %v", err)
			}
			if options.InputPath != tt.want {
				t.Fatalf("input path = %q, want %q", options.InputPath, tt.want)
			}
		})
	}
}

func TestParseArgsAcceptsSupportedLogTypeValues(t *testing.T) {
	for _, logType := range []string{"auto", "plain", "adb", "kernel"} {
		t.Run(logType, func(t *testing.T) {
			options, err := ParseArgs([]string{"--log-type", logType, "input.log"})
			if err != nil {
				t.Fatalf("parse args: %v", err)
			}
			if options.LogType != logType {
				t.Fatalf("log type = %q, want %q", options.LogType, logType)
			}
		})
	}
}

func TestParseArgsRejectsInvalidLogTypeWithSupportedValues(t *testing.T) {
	_, err := ParseArgs([]string{"--log-type", "json", "input.log"})
	if err == nil {
		t.Fatal("parse args error = nil, want error")
	}

	message := err.Error()
	for _, want := range []string{
		`invalid --log-type "json"`,
		"supported values",
		"auto",
		"plain",
		"adb",
		"kernel",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
}

func TestCLILogTypeOverridesConfigDefault(t *testing.T) {
	options, err := ParseArgs([]string{"--log-type", "kernel", "input.log"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}

	resolved, err := usecase.ResolveLogTypeConfig(usecase.LogType(options.LogType), usecase.LogTypeConfig{
		Default: usecase.LogTypeADB,
	})
	if err != nil {
		t.Fatalf("resolve log type config: %v", err)
	}

	if resolved.Default != usecase.LogTypeKernel {
		t.Fatalf("default log type = %q, want %q", resolved.Default, usecase.LogTypeKernel)
	}
}

func TestCLIAutoLogTypeUsesSOTBackedDetection(t *testing.T) {
	options, err := ParseArgs([]string{"--log-type", "auto", "input.log"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}

	resolved, err := usecase.ResolveLogTypeConfig(usecase.LogType(options.LogType), usecase.LogTypeConfig{
		Default: usecase.LogTypeKernel,
	})
	if err != nil {
		t.Fatalf("resolve log type config: %v", err)
	}

	source := &sampleRecordingSource{
		path: "input.log",
		lines: []string{
			"05-24 12:34:56.789  1234  5678 I ActivityManager: start proc",
		},
	}
	session, err := usecase.NewInputSession(usecase.InputRequest{
		InputPath: options.InputPath,
	}, usecase.InputPorts{
		FileSource: source,
	})
	if err != nil {
		t.Fatalf("new input session: %v", err)
	}

	logType, err := session.DetectLogType(context.Background(), resolved)
	if err != nil {
		t.Fatalf("detect log type: %v", err)
	}

	if !source.sampled {
		t.Fatal("source sampled = false, want SOT-backed auto detection")
	}
	if logType != usecase.LogTypeADB {
		t.Fatalf("log type = %q, want %q", logType, usecase.LogTypeADB)
	}
}

func TestParseArgsRejectsMultipleInputPaths(t *testing.T) {
	_, err := ParseArgs([]string{"one.log", "two.log"})
	if err == nil {
		t.Fatal("parse args error = nil, want error")
	}
	for _, want := range []string{
		"expected at most one input path",
		"got 2",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}

type sampleRecordingSource struct {
	path    string
	lines   []string
	sampled bool
}

func (s *sampleRecordingSource) Path() string {
	return s.path
}

func (s *sampleRecordingSource) ReadLine(_ context.Context, lineNumber int) (string, error) {
	if lineNumber < 1 || lineNumber > len(s.lines) {
		return "", nil
	}
	return s.lines[lineNumber-1], nil
}

func (s *sampleRecordingSource) SampleLines(_ context.Context, maxNonEmpty int) ([]string, error) {
	s.sampled = true
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
