package store

import (
	"errors"
	"sync"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// ErrOutOfOrder is returned by Append when the incoming value's Seq is not
// strictly greater than the last appended Seq. The analysis pipeline owns
// a single writer per index, so out-of-order arrivals signal a bug rather
// than an expected condition.
var ErrOutOfOrder = errors.New("store: Seq out of order")

// MemIndex is an append-only, Seq-keyed, in-memory slice. Values appended
// must carry a strictly increasing Seq; reads are safe across goroutines
// while writes must come from a single writer (the pipeline).
//
// seqOf runs under the index lock, so keep it cheap (a struct-field read).
type MemIndex[T any] struct {
	mu      sync.RWMutex
	seqOf   func(T) domain.Seq
	items   []T
	lastSeq domain.Seq
}

// NewMemIndex builds an index with seqOf extracting the Seq from each
// value. Callers typically use a single-field accessor such as
// `func(l domain.Line) domain.Seq { return l.Seq }`.
func NewMemIndex[T any](seqOf func(T) domain.Seq) *MemIndex[T] {
	return &MemIndex[T]{seqOf: seqOf}
}

// Append writes one value. Returns ErrOutOfOrder when the value's Seq is
// not strictly greater than the previous Seq (matters only after the first
// append). The zero-value Seq (0) is treated as "unset"; the first Append
// always succeeds regardless of its Seq.
func (m *MemIndex[T]) Append(v T) error {
	s := m.seqOf(v)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.items) > 0 && s <= m.lastSeq {
		return ErrOutOfOrder
	}
	m.items = append(m.items, v)
	m.lastSeq = s
	return nil
}

// Len returns the number of values currently held.
func (m *MemIndex[T]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// FirstSeq returns the Seq of the oldest value, or 0 when empty.
func (m *MemIndex[T]) FirstSeq() domain.Seq {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.items) == 0 {
		return 0
	}
	return m.seqOf(m.items[0])
}

// LastSeq returns the Seq of the most recently appended value, or 0 when
// empty.
func (m *MemIndex[T]) LastSeq() domain.Seq {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSeq
}

// Get returns the value whose Seq equals s, or (zero, false) when not
// present. Uses binary search on the append-ordered backing slice.
func (m *MemIndex[T]) Get(s domain.Seq) (T, bool) {
	var zero T
	m.mu.RLock()
	defer m.mu.RUnlock()
	i := lowerBound(m.items, m.seqOf, s)
	if i < len(m.items) && m.seqOf(m.items[i]) == s {
		return m.items[i], true
	}
	return zero, false
}

// Range returns a copy of the values whose Seq lies in [from, to]
// (inclusive on both ends). Returns nil for an empty or inverted range.
// The result is a fresh slice so callers can mutate it safely.
func (m *MemIndex[T]) Range(from, to domain.Seq) []T {
	if from > to {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	a := lowerBound(m.items, m.seqOf, from)
	b := lowerBound(m.items, m.seqOf, to+1)
	if a >= b {
		return nil
	}
	out := make([]T, b-a)
	copy(out, m.items[a:b])
	return out
}

func lowerBound[T any](items []T, seqOf func(T) domain.Seq, target domain.Seq) int {
	lo, hi := 0, len(items)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if seqOf(items[mid]) < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
