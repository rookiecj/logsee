package pipeline

import (
	"context"
	"errors"
	"fmt"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/source"
	"git.inpt.fr/42dottools/log/internal/store"
)

// Config wires a run of the pipeline. All fields may be zero-valued; the
// pipeline fills in sane defaults (in-memory Store, default RecordBuilder,
// LineFormatAndroid, no analyzers).
type Config struct {
	Source    source.LogSource
	Store     *store.Store
	Format    domain.LineFormat
	Builder   RecordBuilder
	Analyzers []analysis.Analyzer

	// OnFinding and OnSpan observe each emission in-order. Nil means the
	// emission is only persisted to Store (when Spans/Findings indexes
	// exist on Store) — for v1 the Store holds Lines+Records only, so
	// callers typically supply sinks to avoid losing findings.
	OnFinding func(domain.Finding)
	OnSpan    func(domain.Span)
}

// Metrics is a snapshot of pipeline activity. Read via Pipeline.Metrics.
type Metrics struct {
	Lines    int64
	Records  int64
	Findings int64
	Spans    int64
}

// Pipeline is the synchronous L0→L1→analysis driver. A single goroutine
// consumes Lines from the Source, parses each into a Record, and runs
// every Analyzer over the Record in order. Analyzers emit Findings and
// Spans which are routed to the optional sinks and counted in Metrics.
type Pipeline struct {
	cfg     Config
	metrics Metrics
}

// New returns a ready-to-Run pipeline with defaults applied.
func New(cfg Config) *Pipeline {
	if cfg.Store == nil {
		cfg.Store = store.New()
	}
	if cfg.Format == domain.LineFormatUnknown {
		cfg.Format = domain.LineFormatAndroid
	}
	if cfg.Builder == (RecordBuilder{}) {
		cfg.Builder = DefaultRecordBuilder()
	}
	return &Pipeline{cfg: cfg}
}

// Store exposes the underlying session Store so callers can query it
// after Run completes.
func (p *Pipeline) Store() *store.Store { return p.cfg.Store }

// Metrics returns a snapshot of the counters; not safe to call
// concurrently with Run.
func (p *Pipeline) Metrics() Metrics { return p.metrics }

// Run consumes the Source to completion or until ctx is cancelled. It
// returns nil on normal source exhaustion, ctx.Err() on cancellation, or
// the first error from Store.Append / Source.Lines.
func (p *Pipeline) Run(ctx context.Context) error {
	if p.cfg.Source == nil {
		return errors.New("pipeline: Config.Source is required")
	}
	lineCh, err := p.cfg.Source.Lines(ctx)
	if err != nil {
		return fmt.Errorf("source.Lines: %w", err)
	}
	defer p.cfg.Source.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lineCh:
			if !ok {
				return p.flushAll()
			}
			if err := p.processLine(line); err != nil {
				return err
			}
		}
	}
}

func (p *Pipeline) processLine(line domain.Line) error {
	if err := p.cfg.Store.Lines.Append(line); err != nil {
		return fmt.Errorf("store line %d: %w", line.Seq, err)
	}
	p.metrics.Lines++

	rec := p.cfg.Builder.Build(line, p.cfg.Format)
	if err := p.cfg.Store.Records.Append(rec); err != nil {
		return fmt.Errorf("store record %d: %w", rec.Seq, err)
	}
	p.metrics.Records++

	for _, a := range p.cfg.Analyzers {
		p.deliver(a.OnRecord(rec))
	}
	return nil
}

func (p *Pipeline) flushAll() error {
	for _, a := range p.cfg.Analyzers {
		p.deliver(a.Flush())
	}
	return nil
}

func (p *Pipeline) deliver(out analysis.Output) {
	for _, f := range out.Findings {
		p.metrics.Findings++
		if p.cfg.OnFinding != nil {
			p.cfg.OnFinding(f)
		}
	}
	for _, s := range out.Spans {
		p.metrics.Spans++
		if p.cfg.OnSpan != nil {
			p.cfg.OnSpan(s)
		}
	}
}
