package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/config"
	"git.inpt.fr/42dottools/log/internal/fileindex"
	"git.inpt.fr/42dottools/log/internal/filter"
	"git.inpt.fr/42dottools/log/internal/loginput"
	"git.inpt.fr/42dottools/log/internal/storage"
	"git.inpt.fr/42dottools/log/internal/ui"
	"git.inpt.fr/42dottools/log/internal/userstate"
	"git.inpt.fr/42dottools/log/internal/version"
	"github.com/charmbracelet/bubbletea"
)

type trackedStringFlag struct {
	value string
	set   bool
}

func (f *trackedStringFlag) String() string { return f.value }
func (f *trackedStringFlag) Set(v string) error {
	f.value = v
	f.set = true
	return nil
}

type trackedIntFlag struct {
	value int
	set   bool
}

func (f *trackedIntFlag) String() string { return fmt.Sprintf("%d", f.value) }
func (f *trackedIntFlag) Set(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	f.value = n
	f.set = true
	return nil
}

func main() {
	// Subcommand dispatch: `logsee mcp` serves the Model Context Protocol
	// over stdio. Must be handled before flag.Parse since flag.Parse would
	// treat "mcp" as a positional input file.
	if len(os.Args) >= 2 && os.Args[1] == "mcp" {
		if err := runMCP(); err != nil {
			fmt.Fprintf(os.Stderr, "logsee: %v\n", err)
			os.Exit(1)
		}
		return
	}

	exportAnomalies := flag.Bool("export-anomalies", false, "skip the TUI, run the anomaly pipeline over the input, and write detected Findings/Spans as JSONL to stdout (AI-analysis mode)")
	outPath := flag.String("out", "", "append received lines to this file when reading from stdin; if empty with stdin, creates logsee-YYYYMMDD-HHMMSS.log in cwd. Ignored when reading from an input file (no duplicate on disk)")
	maxLines := flag.Int("max-lines", 100_000, "maximum lines retained in memory")
	ignoreCase := flag.Bool("ignore-case", false, "case-insensitive filter matching only; highlight search is always case-sensitive")
	noLineNumbers := flag.Bool("no-line-numbers", false, "hide sequence column")
	syncInterval := flag.Duration("sync-interval", 0, "if >0, fsync the output file on this interval (e.g. 1s)")
	outMaxBytes := flag.Int("out-max-bytes", 0, "rotate --out when the flushed file plus the next line would exceed this size (bytes); 0 disables rotation (default, single growing file)")
	stdinBatchMS := flag.Int("stdin-batch-ms", 40, "coalesce input lines (stdin or file) for UI updates (0 = send each line immediately)")
	configPathFlag := flag.String("config", "", "path to config.toml (default: $HOME/.local/logsee/config.toml)")
	printDefaultConfig := flag.Bool("print-default-config", false, "print a commented TOML file with built-in defaults to stdout and exit (same as missing config file)")
	logTypeStr := &trackedStringFlag{}
	logTypeProbe := &trackedIntFlag{}
	flag.Var(logTypeStr, "log-type", "log line shape for level tag: auto (probe), plain, adb")
	flag.Var(logTypeProbe, "log-type-probe-lines", "non-empty lines to sample when log-type=auto (minimum 1)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: logsee [flags] [input-file]\n\n")
		fmt.Fprintf(os.Stderr, "  input-file   Path to a log file to read. If omitted, read from stdin.\n")
		fmt.Fprintf(os.Stderr, "               Use \"-\" to read stdin explicitly.\n\n")
		fmt.Fprintf(os.Stderr, "TUI (keyboard on /dev/tty): F1 in-app help (with version); Ctrl+W toggles line wrap; Enter opens filter entry; / opens highlight entry, Enter commits highlight draft; Esc cancels highlight entry; n/p (or Ctrl+n/Ctrl+p) next/prev match when highlight committed; c copies selected lines (Shift+arrows); full keymap in docs/plans/stdio-log-viewer-prd.md\n\n")
		fmt.Fprintf(os.Stderr, "%s\n\n", config.ConfigHelp)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *printDefaultConfig {
		doc, err := config.DefaultConfigTOML()
		if err != nil {
			fmt.Fprintf(os.Stderr, "logsee: %v\n", err)
			os.Exit(2)
		}
		fmt.Print(doc)
		os.Exit(0)
	}

	if *maxLines < 1 {
		fmt.Fprintln(os.Stderr, "--max-lines must be >= 1")
		os.Exit(2)
	}
	if *stdinBatchMS < 0 {
		fmt.Fprintln(os.Stderr, "--stdin-batch-ms must be >= 0")
		os.Exit(2)
	}
	if *outMaxBytes < 0 {
		fmt.Fprintln(os.Stderr, "--out-max-bytes must be >= 0")
		os.Exit(2)
	}
	cfgPath := strings.TrimSpace(*configPathFlag)
	if cfgPath == "" {
		var derr error
		cfgPath, derr = config.DefaultConfigPath()
		if derr != nil {
			fmt.Fprintf(os.Stderr, "logsee: default config path: %v\n", derr)
			os.Exit(2)
		}
	}
	if config.IsLegacyJSONName(cfgPath) {
		fmt.Fprintf(os.Stderr, "logsee: warning: config path %q looks like legacy JSON; use %s (see --print-default-config).\n", cfgPath, config.FileName())
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logsee: %v\n", err)
		os.Exit(2)
	}
	defaultType := cfg.LogType.Default
	if logTypeStr.set {
		defaultType = logTypeStr.value
	}
	probeLines := cfg.LogType.ProbeLines
	if logTypeProbe.set {
		probeLines = logTypeProbe.value
	}
	if probeLines < 1 {
		fmt.Fprintln(os.Stderr, "--log-type-probe-lines must be >= 1")
		os.Exit(2)
	}
	ltKind, err := ui.ParseLogTypeKind(defaultType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logsee: %v\n", err)
		os.Exit(2)
	}
	if err := filter.SetPatternConfig(cfg.LogType.Patterns); err != nil {
		fmt.Fprintf(os.Stderr, "logsee: invalid log_type.patterns: %v\n", err)
		os.Exit(2)
	}
	logTypeOpts := &ui.LogTypeOpts{Kind: ltKind, ProbeLines: probeLines}

	args := flag.Args()
	if len(args) > 1 {
		flag.Usage()
		os.Exit(2)
	}

	if *exportAnomalies {
		if err := runExportAnomalies(args); err != nil {
			fmt.Fprintf(os.Stderr, "logsee: %v\n", err)
			os.Exit(1)
		}
		return
	}

	input, inputLabel, closeInput, err := openInput(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if closeInput != nil {
		defer func() { _ = closeInput() }()
	}

	fromFile := len(args) == 1 && args[0] != "-"

	effectiveOut := strings.TrimSpace(*outPath)
	var store *storage.LineAppender
	if !fromFile {
		if effectiveOut == "" {
			effectiveOut = fmt.Sprintf("logsee-%s.log", time.Now().Format("20060102-150405"))
		}
		var errOpen error
		store, errOpen = storage.NewLineAppender(effectiveOut, *syncInterval, int64(*outMaxBytes))
		if errOpen != nil {
			fmt.Fprintf(os.Stderr, "open output file %q: %v\n", effectiveOut, errOpen)
			os.Exit(1)
		}
		defer func() { _ = store.Close() }()
	} else {
		effectiveOut = ""
	}

	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open /dev/tty for keyboard input: %v\n", err)
		os.Exit(1)
	}
	defer tty.Close()

	inputSource := "stdin"
	if len(args) == 1 && args[0] != "-" {
		inputSource = filepath.Clean(args[0])
	}

	statePath := ""
	var snap userstate.Snapshot
	if strings.TrimSpace(cfg.History.Dir) != "" {
		statePath = userstate.StateFilePath(filepath.Clean(strings.TrimSpace(cfg.History.Dir)))
	} else {
		d, derr := userstate.DefaultStateDir()
		if derr != nil {
			fmt.Fprintf(os.Stderr, "logsee: default state directory: %v (filter/highlight persistence disabled)\n", derr)
		} else {
			statePath = userstate.StateFilePath(d)
		}
	}
	if statePath != "" {
		loaded, err := userstate.Load(statePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logsee: load state %q: %v\n", statePath, err)
		} else {
			snap = loaded
		}
	}
	var hist *ui.HistoryOpts
	if statePath != "" {
		hist = &ui.HistoryOpts{StateFile: statePath, Initial: snap}
	}

	ring := buffer.NewRing(*maxLines)
	mod := ui.NewModel(ring, store, *ignoreCase, *noLineNumbers, effectiveOut, inputSource, version.Line(), hist, logTypeOpts, cfg.HighlightColorNames)
	if !fromFile {
		rsp := ui.NewRingStreamProvider(ring)
		if store != nil && effectiveOut != "" {
			// Disk-backed scrollback: seqs evicted from the ring resolve via the --out file.
			// Pre-existing bytes (when --out points at an existing file) are opaque context;
			// session seqs begin at 1 and map to file lines written after startByte.
			startByte := store.Size()
			idx := fileindex.NewIncrementalOffsetIndex(effectiveOut, startByte)
			rsp.SetDiskFallback(effectiveOut, idx, 1)
		}
		mod.SetWindowProvider(rsp)
	}
	p := tea.NewProgram(mod, tea.WithInput(tty), tea.WithAltScreen())

	if fromFile {
		path := inputSource
		const initialLines = 42 // ~2× default viewport (PRD: first window + one page)
		go func() {
			off, err := fileindex.BuildLineStartOffsets(path)
			if err != nil {
				p.Send(ui.FileIndexReadyMsg{Err: err})
				return
			}
			p.Send(ui.FileIndexReadyMsg{Offsets: off})
		}()
		go func() {
			lines, err := fileindex.ReadFirstNLines(path, initialLines)
			if err != nil {
				p.Send(ui.FilePartialBootstrapMsg{Path: path, Err: err})
				return
			}
			p.Send(ui.FilePartialBootstrapMsg{Path: path, Lines: lines})
		}()
	} else {
		go readInput(p, *stdinBatchMS, input, inputLabel)
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ui: %v\n", err)
		os.Exit(1)
	}
}

// openInput returns the reader to stream log lines from, a label for errors, and an optional closer (nil for stdin).
func openInput(args []string) (io.Reader, string, func() error, error) {
	if len(args) == 0 {
		return os.Stdin, "stdin", nil, nil
	}
	path := args[0]
	if path == "-" {
		return os.Stdin, "stdin", nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", nil, fmt.Errorf("open input file %q: %w", path, err)
	}
	return f, path, f.Close, nil
}

func readInput(p *tea.Program, batchMS int, input io.Reader, label string) {
	if batchMS == 0 {
		readInputImmediate(p, input, label)
		return
	}
	lineCh := make(chan string, 8192)
	go pumpLines(lineCh, input, label)

	tick := time.NewTicker(time.Duration(batchMS) * time.Millisecond)
	defer tick.Stop()
	var batch []string
	for {
		select {
		case s, ok := <-lineCh:
			if !ok {
				if len(batch) > 0 {
					p.Send(ui.LineBatchMsg(batch))
				}
				p.Send(ui.StdinClosedMsg{})
				return
			}
			batch = append(batch, s)
		case <-tick.C:
			if len(batch) == 0 {
				continue
			}
			p.Send(ui.LineBatchMsg(batch))
			batch = batch[:0]
		}
	}
}

func pumpLines(lineCh chan<- string, input io.Reader, label string) {
	defer close(lineCh)
	err := loginput.ScanLines(input, func(s string) error {
		lineCh <- s
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
	}
}

func readInputImmediate(p *tea.Program, input io.Reader, label string) {
	err := loginput.ScanLines(input, func(s string) error {
		p.Send(ui.LineMsg(s))
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
	}
	p.Send(ui.StdinClosedMsg{})
}
