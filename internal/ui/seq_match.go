package ui

import (
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

// SeqPredicate tests whether a record should be considered a match for a given purpose
// (filter pass, search hit, future bookmark/custom predicate). Returning true accepts the record.
//
// Phase 3 boundary: any scan — ring-local or disk-driven — should take a SeqPredicate so the
// matching rule stays orthogonal to the scan mechanism. In-memory scans (via fidx) and pull-driven
// disk scans (WindowProvider.Fetch) can share the same predicate.
type SeqPredicate func(rec domain.Line) bool

// filterPredicate closes over the currently applied filter program so a single predicate can be
// reused across ring scans and disk scans without reading Model state from a goroutine.
func (m *Model) filterPredicate() SeqPredicate {
	prog := m.prog
	ignoreCase := m.ignoreCase
	logFmt := m.effectiveLogFormat()
	return func(rec domain.Line) bool {
		return filter.Match(rec.Text, prog, ignoreCase, logFmt)
	}
}

// searchPredicate closes over the committed highlight query + color names so disk scans can
// test records without touching shared Model state.
func (m *Model) searchPredicate() SeqPredicate {
	query := m.searchBuf
	names := m.highlightNames
	return func(rec domain.Line) bool {
		return SearchMatchesLineWithNames(rec.Text, query, false, names)
	}
}

// nextMatchIdxInFidx returns the fidx index adjacent to fromSeq in direction dir (+1 forward,
// -1 backward) whose record satisfies pred. Pass pred == nil to accept any record already in
// fidx (useful for "next filter-matching seq" when fidx is a filter projection).
//
// Returns -1 when no such record exists within the loaded window. Callers handle the on-disk
// fallback (lazy search, filter top-up, boundary nav) — this helper stays ring-local.
func (m *Model) nextMatchIdxInFidx(fidx []int, fromSeq int64, dir int, pred SeqPredicate) int {
	if m.buf == nil || len(fidx) == 0 {
		return -1
	}
	accept := func(rec domain.Line) bool {
		return pred == nil || pred(rec)
	}
	bufLen := m.buf.Len()
	if dir > 0 {
		for i, ri := range fidx {
			if ri < 0 || ri >= bufLen {
				continue
			}
			rec := m.buf.At(ri)
			if rec.Seq > fromSeq && accept(rec) {
				return i
			}
		}
		return -1
	}
	for i := len(fidx) - 1; i >= 0; i-- {
		ri := fidx[i]
		if ri < 0 || ri >= bufLen {
			continue
		}
		rec := m.buf.At(ri)
		if rec.Seq < fromSeq && accept(rec) {
			return i
		}
	}
	return -1
}
