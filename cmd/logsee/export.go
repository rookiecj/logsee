package main

import (
	"context"
	"encoding/json"
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
	"git.inpt.fr/42dottools/log/internal/ui"
)

// exportRecord is the on-wire envelope for `--export-anomalies`. One JSON
// object is written per line; consumers switch on Type.
type exportRecord struct {
	Type    string          `json:"type"`
	Finding *domain.Finding `json:"finding,omitempty"`
	Span    *domain.Span    `json:"span,omitempty"`
}

// runExportAnomalies bypasses the TUI and writes detected Findings/Spans
// as JSONL to stdout. It exits 0 on clean completion, non-zero on error.
// The summary line on stderr matches the logsee-ana format so tooling can
// reuse parsers.
//
// format selects which parser and analyzer set to run. Empty or "android"
// uses the adb path (threadtime + native/java/ANR block analyzers).
// "journal" swaps in the short-iso parser plus kernel-panic and
// systemd-coredump block analyzers.
func runExportAnomalies(args []string, format domain.LineFormat) error {
	src, err := openExportSource(args)
	if err != nil {
		return err
	}
	if format == domain.LineFormatUnknown {
		format = domain.LineFormatAndroid
	}

	enc := json.NewEncoder(os.Stdout)
	cfg := pipeline.Config{
		Source:    src,
		Format:    format,
		Builder:   pipeline.DefaultRecordBuilder(),
		Analyzers: exportAnalyzers(format),
		OnFinding: func(f domain.Finding) {
			_ = enc.Encode(exportRecord{Type: "finding", Finding: &f})
		},
		OnSpan: func(s domain.Span) {
			_ = enc.Encode(exportRecord{Type: "span", Span: &s})
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	p := pipeline.New(cfg)
	if err := p.Run(ctx); err != nil && err != context.Canceled {
		return err
	}
	m := p.Metrics()
	fmt.Fprintf(os.Stderr, "logsee: exported %d findings, %d spans from %d lines\n",
		m.Findings, m.Spans, m.Lines)
	return nil
}

func openExportSource(args []string) (source.LogSource, error) {
	if len(args) == 0 || args[0] == "-" {
		return source.NewReader(os.Stdin), nil
	}
	return source.NewFile(args[0]), nil
}

func exportAnalyzers(format domain.LineFormat) []analysis.Analyzer {
	common := []analysis.Analyzer{classify.New()}
	switch format {
	case domain.LineFormatJournal:
		return append(common, block.NewKernelPanic(), block.NewSystemdCoredump())
	default:
		return append(common, block.NewNativeCrash(), block.NewJavaFatal(), block.NewANR())
	}
}

// exportFormatFromLogType maps the CLI --log-type value to the
// domain.LineFormat the exporter should feed the pipeline. Auto / unknown
// falls back to Android so existing scripts that omit the flag keep
// working.
func exportFormatFromLogType(k ui.LogTypeKind) domain.LineFormat {
	switch k {
	case ui.LogTypeJournal:
		return domain.LineFormatJournal
	case ui.LogTypePlain:
		return domain.LineFormatPlain
	default:
		return domain.LineFormatAndroid
	}
}
