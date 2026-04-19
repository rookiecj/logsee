package pipeline

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/analysis/block"
	"git.inpt.fr/42dottools/log/internal/analysis/classify"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/source"
)

func TestPipeline_RequiresSource(t *testing.T) {
	p := New(Config{})
	err := p.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Source is required") {
		t.Errorf("missing source should return explicit error, got %v", err)
	}
}

func TestPipeline_RunFillsStoreAndMetrics(t *testing.T) {
	input := "04-19 14:23:40.012  1245  1301 I ActivityManager: foo\n" +
		"04-19 14:23:40.013  1245  1301 I ActivityManager: bar\n"
	src := source.NewReader(strings.NewReader(input))
	p := New(Config{
		Source:  src,
		Format:  domain.LineFormatAndroid,
		Builder: RecordBuilder{RefYear: 2024, Location: time.UTC},
	})
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	m := p.Metrics()
	if m.Lines != 2 || m.Records != 2 {
		t.Errorf("metrics: got %+v, want Lines=2 Records=2", m)
	}
	st := p.Store()
	if st.Lines.Len() != 2 || st.Records.Len() != 2 {
		t.Errorf("store lens: lines=%d records=%d, want 2/2", st.Lines.Len(), st.Records.Len())
	}
	r, ok := st.Records.Get(1)
	if !ok || r.Tag != "ActivityManager" || r.PID != 1245 {
		t.Errorf("store Records.Get(1) = %+v/%v, want ActivityManager tag, PID 1245", r, ok)
	}
}

func TestPipeline_SampleFile_EndToEnd(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "android", "anr_input_dispatch.log")
	src := source.NewFile(path)
	var findings []domain.Finding
	var spans []domain.Span
	p := New(Config{
		Source:  src,
		Format:  domain.LineFormatAndroid,
		Builder: RecordBuilder{RefYear: 2024, Location: time.UTC},
		Analyzers: []analysis.Analyzer{
			classify.New(),
			block.NewANR(),
			block.NewJavaFatal(),
			block.NewNativeCrash(),
		},
		OnFinding: func(f domain.Finding) { findings = append(findings, f) },
		OnSpan:    func(s domain.Span) { spans = append(spans, s) },
	})
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Sample must produce at least the ANR span and the ANR finding.
	sawANRFinding := false
	for _, f := range findings {
		if f.Kind == domain.FindingANR {
			sawANRFinding = true
			break
		}
	}
	if !sawANRFinding {
		t.Error("expected at least one ANR Finding on the sample")
	}
	anrSpans := 0
	for _, s := range spans {
		if s.Kind == domain.SpanANR {
			anrSpans++
		}
	}
	if anrSpans != 1 {
		t.Errorf("expected exactly 1 ANR span on sample, got %d", anrSpans)
	}
	m := p.Metrics()
	if m.Lines == 0 || m.Records == 0 {
		t.Errorf("metrics zero after Run: %+v", m)
	}
	if int64(len(findings)) != m.Findings || int64(len(spans)) != m.Spans {
		t.Errorf("sink count vs metrics mismatch: findings %d vs %d, spans %d vs %d",
			len(findings), m.Findings, len(spans), m.Spans)
	}
}

func TestPipeline_ContextCancellationStopsSource(t *testing.T) {
	// Long input — cancel mid-stream.
	var b strings.Builder
	for i := 0; i < 10_000; i++ {
		b.WriteString("04-19 14:23:40.012  1245  1301 I X: hi\n")
	}
	src := source.NewReader(strings.NewReader(b.String())).WithChannelCapacity(1)
	p := New(Config{Source: src, Format: domain.LineFormatAndroid})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the loop exits on first iteration.
	cancel()
	err := p.Run(ctx)
	if err == nil {
		t.Error("cancelled run should return context error")
	}
}
