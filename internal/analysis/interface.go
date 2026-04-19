package analysis

import "git.inpt.fr/42dottools/log/internal/domain"

// Output bundles everything an Analyzer emits from a single OnRecord call
// (or from Flush at end-of-stream). Either slice may be nil; callers must
// treat the Output as read-only and not retain references across calls.
type Output struct {
	Findings []domain.Finding
	Spans    []domain.Span
}

// Empty reports whether both slices are empty.
func (o Output) Empty() bool { return len(o.Findings) == 0 && len(o.Spans) == 0 }

// Analyzer consumes Records in strictly increasing Seq order and emits
// Findings and/or Spans. Implementations must be pure: no I/O, no global
// state, deterministic on the same input sequence. The pipeline owns all
// goroutines and I/O; analyzers do not.
type Analyzer interface {
	Name() string
	OnRecord(r domain.Record) Output
	Flush() Output
}

// StatefulAnalyzer retains state across OnRecord calls and can be
// serialized for checkpoint/restore. Snapshot/Restore round-trips must
// preserve OnRecord output exactly.
type StatefulAnalyzer interface {
	Analyzer
	Snapshot() ([]byte, error)
	Restore(b []byte) error
}
