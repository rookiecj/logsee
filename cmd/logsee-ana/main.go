// Command logsee-ana runs the analysis pipeline on a log file or stdin and
// writes one JSON object per Finding and Span detected. Experimental: it
// exists to exercise the pipeline end-to-end without the TUI. Production
// use-cases should wait for the --export-anomalies flag on the main logsee
// binary and the MCP server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/analysis/block"
	"git.inpt.fr/42dottools/log/internal/analysis/classify"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/pipeline"
	"git.inpt.fr/42dottools/log/internal/source"
)

type outRec struct {
	Type    string          `json:"type"`
	Finding *domain.Finding `json:"finding,omitempty"`
	Span    *domain.Span    `json:"span,omitempty"`
}

func main() {
	format := flag.String("format", "android", "input log format (android|plain|bracket)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: logsee-ana [--format FORMAT] <file>")
		fmt.Fprintln(os.Stderr, "  reads <file> (use - for stdin) and writes one JSON object per line")
		fmt.Fprintln(os.Stderr, "  for each Finding and Span detected. A one-line summary is written")
		fmt.Fprintln(os.Stderr, "  to stderr on completion.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "flags:")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	src, err := openSource(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "logsee-ana: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	cfg := pipeline.Config{
		Source:  src,
		Format:  parseFormat(*format),
		Builder: pipeline.DefaultRecordBuilder(),
		Analyzers: []analysis.Analyzer{
			classify.New(),
			block.NewNativeCrash(),
			block.NewJavaFatal(),
			block.NewANR(),
		},
		OnFinding: func(f domain.Finding) { _ = enc.Encode(outRec{Type: "finding", Finding: &f}) },
		OnSpan:    func(s domain.Span) { _ = enc.Encode(outRec{Type: "span", Span: &s}) },
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	p := pipeline.New(cfg)
	if err := p.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "logsee-ana: %v\n", err)
		os.Exit(1)
	}
	m := p.Metrics()
	fmt.Fprintf(os.Stderr, "logsee-ana: %d lines, %d findings, %d spans\n",
		m.Lines, m.Findings, m.Spans)
}

func openSource(path string) (source.LogSource, error) {
	if path == "-" {
		return source.NewReader(os.Stdin), nil
	}
	return source.NewFile(path), nil
}

func parseFormat(s string) domain.LineFormat {
	switch s {
	case "android":
		return domain.LineFormatAndroid
	case "plain":
		return domain.LineFormatPlain
	case "bracket":
		return domain.LineFormatBracket
	default:
		return domain.LineFormatAndroid
	}
}
