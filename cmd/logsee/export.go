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
func runExportAnomalies(args []string) error {
	src, err := openExportSource(args)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	cfg := pipeline.Config{
		Source:  src,
		Format:  domain.LineFormatAndroid,
		Builder: pipeline.DefaultRecordBuilder(),
		Analyzers: []analysis.Analyzer{
			classify.New(),
			block.NewNativeCrash(),
			block.NewJavaFatal(),
			block.NewANR(),
		},
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
