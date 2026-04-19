package buffer

import "git.inpt.fr/42dottools/log/internal/domain"

// Ring keeps the last max records in arrival order (oldest at index 0).
type Ring struct {
	max     int
	lines   []domain.Line
	nextSeq int64
}

// NewRing returns a ring with capacity max (max<=0 means no retention).
func NewRing(max int) *Ring {
	return &Ring{max: max}
}

// Push appends a line; drops oldest when over capacity.
func (r *Ring) Push(text string) domain.Line {
	r.nextSeq++
	rec := domain.Line{Seq: r.nextSeq, Text: text}
	if r.max <= 0 {
		return rec
	}
	if len(r.lines) == r.max {
		r.lines = r.lines[1:]
	}
	r.lines = append(r.lines, rec)
	return rec
}

// AdvanceSeq increments the sequence counter without appending to the ring.
// Used by stdin scrollback mode where new lines persist to disk and bump the
// seq counter but must not disturb the historical window shown in the ring.
func (r *Ring) AdvanceSeq() int64 {
	r.nextSeq++
	return r.nextSeq
}

// NextSeq returns the last assigned sequence number (0 before the first Push).
func (r *Ring) NextSeq() int64 { return r.nextSeq }

// SetNextSeq overrides the sequence counter. Callers use this to preserve seq
// continuity across a [Ring.ReplaceRecords] call when the replacement window
// does not include the live tail (stdin scrollback view).
func (r *Ring) SetNextSeq(seq int64) {
	if seq < 0 {
		seq = 0
	}
	r.nextSeq = seq
}

// Len returns the number of stored lines.
func (r *Ring) Len() int {
	return len(r.lines)
}

// At returns the i-th oldest record (0 <= i < Len).
func (r *Ring) At(i int) domain.Line {
	return r.lines[i]
}

// ReplaceRecords replaces the entire buffer with recs (file window mode). Sequence numbers are preserved from recs.
func (r *Ring) ReplaceRecords(recs []domain.Line) {
	if r.max <= 0 {
		r.lines = append([]domain.Line(nil), recs...)
		if len(recs) > 0 {
			r.nextSeq = recs[len(recs)-1].Seq + 1
		} else {
			r.nextSeq = 1
		}
		return
	}
	r.lines = append([]domain.Line(nil), recs...)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
	if len(r.lines) > 0 {
		r.nextSeq = r.lines[len(r.lines)-1].Seq + 1
	} else {
		r.nextSeq = 1
	}
}
