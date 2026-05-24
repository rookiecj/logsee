package cli

import (
	"flag"
	"fmt"
	"io"
)

type Options struct {
	InputPath  string
	OutPath    string
	ConfigPath string
	LogType    string
	IgnoreCase bool
	Version    bool
}

const usageText = `Usage: logsee [options] [input-file|-]

Arguments:
  [input-file|-]                         로그 파일 지정. 지정하지 않거나 -이면 STDIO에서 로그를 읽어 들인다.

Options:
  --config <path>                        config.toml path
  --log-type <auto|plain|adb|kernel>     입력 로그의 타입
  --out <path>                           stdio SOT output path (default: ./logsee-YYYYMMDD-HHMMSS.log)
  --ignore-case                          ignore case for filtering
  --version                              print version and exit
`

func Usage() string {
	return usageText
}

func ParseArgs(args []string) (Options, error) {
	var options Options
	flags := newFlagSet(&options)

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if options.LogType != "" && !isSupportedLogType(options.LogType) {
		return Options{}, fmt.Errorf("invalid --log-type %q: supported values are auto, plain, adb, and kernel", options.LogType)
	}

	remaining := flags.Args()
	if len(remaining) > 1 {
		return Options{}, fmt.Errorf("expected at most one input path, got %d", len(remaining))
	}
	if len(remaining) == 1 {
		options.InputPath = remaining[0]
	}
	return options, nil
}

func newFlagSet(options *Options) *flag.FlagSet {
	flags := flag.NewFlagSet("logsee", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), Usage())
	}
	flags.StringVar(&options.OutPath, "out", "", "stdio SOT output path")
	flags.StringVar(&options.ConfigPath, "config", "", "config file path")
	flags.StringVar(&options.LogType, "log-type", "", "input log type")
	flags.BoolVar(&options.IgnoreCase, "ignore-case", false, "ignore case for filtering")
	flags.BoolVar(&options.Version, "version", false, "print version and exit")
	return flags
}

func isSupportedLogType(logType string) bool {
	switch logType {
	case "auto", "plain", "adb", "kernel":
		return true
	default:
		return false
	}
}
