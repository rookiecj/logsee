package buffer

import "git.inpt.fr/42dottools/log/internal/domain"

// Ring keeps the last max records in arrival order (oldest at index 0).
type Ring struct {
	max     int
	lines   []domain.Record
	nextSeq int64
}

// NewRing returns a ring with capacity max (max<=0 means no retention).
func NewRing(max int) *Ring {
	return &Ring{max: max}
}

// Push appends a line; drops oldest when over capacity.
func (r *Ring) Push(text string) domain.Record {
	r.nextSeq++
	rec := domain.Record{Seq: r.nextSeq, Text: text}
	if r.max <= 0 {
		return rec
	}
	if len(r.lines) == r.max {
		r.lines = r.lines[1:]
	}
	r.lines = append(r.lines, rec)
	return rec
}

// Len returns the number of stored lines.
func (r *Ring) Len() int {
	return len(r.lines)
}

// At returns the i-th oldest record (0 <= i < Len).
func (r *Ring) At(i int) domain.Record {
	return r.lines[i]
}

// ReplaceRecords replaces the entire buffer with recs (file window mode). Sequence numbers are preserved from recs.
func (r *Ring) ReplaceRecords(recs []domain.Record) {
	if r.max <= 0 {
		r.lines = append([]domain.Record(nil), recs...)
		if len(recs) > 0 {
			r.nextSeq = recs[len(recs)-1].Seq + 1
		} else {
			r.nextSeq = 1
		}
		return
	}
	r.lines = append([]domain.Record(nil), recs...)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
	if len(r.lines) > 0 {
		r.nextSeq = r.lines[len(r.lines)-1].Seq + 1
	} else {
		r.nextSeq = 1
	}
}
