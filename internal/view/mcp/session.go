package mcp

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/analysis/block"
	"git.inpt.fr/42dottools/log/internal/analysis/classify"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/pipeline"
	"git.inpt.fr/42dottools/log/internal/source"
	"git.inpt.fr/42dottools/log/internal/store"
)

// Session holds the result of running the analysis pipeline on one log
// source. It is immutable after load_session completes.
type Session struct {
	ID       string
	Path     string
	Findings []domain.Finding
	Spans    []domain.Span

	store    *store.Store
	spanByID map[int64]domain.Span
}

var sessionCounter int64

func newSessionID() string {
	n := atomic.AddInt64(&sessionCounter, 1)
	return fmt.Sprintf("s-%d-%d", time.Now().Unix(), n)
}

// loadSession runs the pipeline over the given path and returns the
// populated Session. Caller inserts it into Server.sessions under lock.
func loadSession(ctx context.Context, path string) (*Session, pipeline.Metrics, error) {
	sess := &Session{
		ID:       newSessionID(),
		Path:     path,
		spanByID: map[int64]domain.Span{},
	}
	var nextSpanID int64
	cfg := pipeline.Config{
		Source:  source.NewFile(path),
		Store:   store.New(),
		Format:  domain.LineFormatAndroid,
		Builder: pipeline.DefaultRecordBuilder(),
		Analyzers: []analysis.Analyzer{
			classify.New(),
			block.NewNativeCrash(),
			block.NewJavaFatal(),
			block.NewANR(),
		},
		OnFinding: func(f domain.Finding) {
			sess.Findings = append(sess.Findings, f)
		},
		OnSpan: func(s domain.Span) {
			nextSpanID++
			s.ID = nextSpanID
			sess.Spans = append(sess.Spans, s)
			sess.spanByID[s.ID] = s
		},
	}
	p := pipeline.New(cfg)
	if err := p.Run(ctx); err != nil && err != context.Canceled {
		return nil, pipeline.Metrics{}, err
	}
	sess.store = p.Store()
	return sess, p.Metrics(), nil
}

// session looks up a session by id under lock.
func (s *Server) session(id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return sess, nil
}
