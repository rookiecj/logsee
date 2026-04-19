package store

import "git.inpt.fr/42dottools/log/internal/domain"

// Store bundles the per-session indexes the analysis pipeline writes to.
// Additional indexes (Spans, Findings) attach in later phases; for now
// Lines and Records cover Phase 3's scope.
type Store struct {
	Lines   *MemIndex[domain.Line]
	Records *MemIndex[domain.Record]
}

// New allocates an empty in-memory Store ready for single-writer use.
func New() *Store {
	return &Store{
		Lines:   NewMemIndex(func(l domain.Line) domain.Seq { return l.Seq }),
		Records: NewMemIndex(func(r domain.Record) domain.Seq { return r.Seq }),
	}
}
