package ui

import (
	"sync"
	"sync/atomic"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/fileindex"
)

// WindowProvider abstracts random-access reads of logical lines from a log source indexed by
// 1-based sequence numbers. It is the Phase 2 boundary between the view/model layer and the
// underlying storage (file, in-memory stream, test fake).
//
// Methods must be safe to call from tea.Cmd closures (goroutines): implementations should not
// mutate shared state.
type WindowProvider interface {
	// Fetch returns records for seqs firstSeq..lastSeq inclusive (1-based). Out-of-range
	// seqs are clamped; firstSeq > total returns an empty slice.
	Fetch(firstSeq, lastSeq int64) ([]domain.Record, error)
	// TotalLines is the number of logical lines known to the provider (0 when not yet indexed).
	TotalLines() int64
	// FileSize is the byte size of the underlying source (0 when unknown or not file-backed).
	FileSize() int64
	// EstimateBytes approximates the byte span of seqs firstSeq..lastSeq inclusive.
	// Used for soft-limit gating on disk-heavy scans (PRD §4.1 search 100 MiB guardrail).
	EstimateBytes(firstSeq, lastSeq int64) int64
}

// FileSliceProvider reads from a file whose line-start byte offsets have been pre-computed
// (fileindex.BuildLineStartOffsets). Captures immutable snapshots at construction so concurrent
// command goroutines can iterate safely.
type FileSliceProvider struct {
	path      string
	offsets   []int64
	sizeBytes int64
}

// NewFileSliceProvider copies offsets so later model-side mutations don't race with in-flight fetches.
func NewFileSliceProvider(path string, offsets []int64, sizeBytes int64) *FileSliceProvider {
	cp := append([]int64(nil), offsets...)
	return &FileSliceProvider{path: path, offsets: cp, sizeBytes: sizeBytes}
}

func (p *FileSliceProvider) Fetch(firstSeq, lastSeq int64) ([]domain.Record, error) {
	if p == nil {
		return nil, nil
	}
	return fileindex.ReadWindowRecords(p.path, p.offsets, int(firstSeq), int(lastSeq))
}

func (p *FileSliceProvider) TotalLines() int64 {
	if p == nil {
		return 0
	}
	return int64(len(p.offsets))
}

func (p *FileSliceProvider) FileSize() int64 {
	if p == nil {
		return 0
	}
	return p.sizeBytes
}

func (p *FileSliceProvider) EstimateBytes(firstSeq, lastSeq int64) int64 {
	if p == nil || len(p.offsets) == 0 || firstSeq < 1 || lastSeq < firstSeq {
		return 0
	}
	start := p.offsets[firstSeq-1]
	var end int64
	if lastSeq < int64(len(p.offsets)) {
		end = p.offsets[lastSeq]
	} else {
		end = p.sizeBytes
	}
	if end < start {
		return 0
	}
	return end - start
}

// RingStreamProvider serves stdin-sourced lines from an in-memory [buffer.Ring]. It is the
// stdin-side counterpart to [FileSliceProvider]: both implement [WindowProvider] so the UI can
// treat file and stream inputs uniformly (docs/plans/stdin-fileprovider-unify-plan.md, Phase 1).
//
// TotalLines reflects the cumulative count of lines received from the stream — it does not
// shrink when the ring evicts older entries. Fetch copies records whose Seq falls in the
// requested range; seqs that have been evicted are silently absent (callers observe the same
// scrollback horizon as today's stdin path).
//
// FileSize and EstimateBytes return 0: the stream has no byte-addressable backing store, and
// the disk-scan byte guardrail (PRD §4.1) is not applicable here. The existing filePartial
// gate keeps stdin off the disk-scan code paths, so a zero estimate is safe.
type RingStreamProvider struct {
	buf       *buffer.Ring
	mu        sync.Mutex
	totalRecv int64
}

// NewRingStreamProvider wraps buf. buf must not be nil.
func NewRingStreamProvider(buf *buffer.Ring) *RingStreamProvider {
	return &RingStreamProvider{buf: buf}
}

// NoteReceived bumps the cumulative receive count by n. Called from the UI goroutine after
// applyIncomingLines pushes a batch into the ring.
func (p *RingStreamProvider) NoteReceived(n int) {
	if p == nil || n <= 0 {
		return
	}
	atomic.AddInt64(&p.totalRecv, int64(n))
}

// Fetch returns ring records whose Seq ∈ [firstSeq, lastSeq]. Out-of-range or evicted seqs
// are silently absent. Holds p.mu while scanning the ring so concurrent tea.Cmd goroutines
// and UI-thread pushes do not race on the ring slice.
func (p *RingStreamProvider) Fetch(firstSeq, lastSeq int64) ([]domain.Record, error) {
	if p == nil || p.buf == nil || lastSeq < firstSeq {
		return nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	n := p.buf.Len()
	if n == 0 {
		return nil, nil
	}
	var out []domain.Record
	for i := range n {
		rec := p.buf.At(i)
		if rec.Seq < firstSeq {
			continue
		}
		if rec.Seq > lastSeq {
			break
		}
		out = append(out, rec)
	}
	return out, nil
}

// TotalLines returns the cumulative number of lines received from the stream (ring evictions
// do not decrease this). Reported to the UI status strip as the "out of" denominator.
func (p *RingStreamProvider) TotalLines() int64 {
	if p == nil {
		return 0
	}
	return atomic.LoadInt64(&p.totalRecv)
}

// FileSize returns 0 — stream has no byte-addressable backing store.
func (p *RingStreamProvider) FileSize() int64 { return 0 }

// EstimateBytes returns 0 — the disk-scan guardrail does not apply to in-memory stream reads.
func (p *RingStreamProvider) EstimateBytes(firstSeq, lastSeq int64) int64 { return 0 }

// windowProviderOrFallback returns the Model's configured provider, falling back to one
// synthesized from raw fileOffsets/filePath/fileSizeBytes fields. This keeps existing tests
// (which seed the raw fields directly) working without ceremony while the seq-primary refactor
// migrates call sites onto the interface.
func (m *Model) windowProviderOrFallback() WindowProvider {
	if m.windowProvider != nil {
		return m.windowProvider
	}
	if len(m.fileOffsets) == 0 {
		return nil
	}
	return NewFileSliceProvider(m.filePath, m.fileOffsets, m.fileSizeBytes)
}
