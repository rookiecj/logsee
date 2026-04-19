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
	Fetch(firstSeq, lastSeq int64) ([]domain.Line, error)
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

func (p *FileSliceProvider) Fetch(firstSeq, lastSeq int64) ([]domain.Line, error) {
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
// Disk fallback: if [SetDiskFallback] is invoked at construction, seqs evicted from the ring
// are resolved through the --out file via [fileindex.IncrementalOffsetIndex]. Seqs lost to
// pre-rotation files remain unreachable; the horizon tracks the earliest reachable seq.
//
// TotalLines reflects the cumulative count of lines received from the stream — it does not
// shrink when the ring evicts older entries. FileSize/EstimateBytes cover the disk portion
// when fallback is enabled; otherwise they return 0 (the filePartial gate prevents disk-scan
// guardrails from tripping on pure-ring input).
type RingStreamProvider struct {
	buf       *buffer.Ring
	mu        sync.Mutex
	totalRecv int64

	outPath string
	index   *fileindex.IncrementalOffsetIndex
	seqBase int64 // seq of file line 1 (1 for fresh file; bumps on rotation)
	horizon int64 // earliest seq reachable via disk fallback (1 unless rotation occurred)
}

// NewRingStreamProvider wraps buf. buf must not be nil.
func NewRingStreamProvider(buf *buffer.Ring) *RingStreamProvider {
	return &RingStreamProvider{buf: buf, seqBase: 1, horizon: 1}
}

// SetDiskFallback enables disk-backed scrollback. path is the --out file, idx tracks its line
// offsets, startSeq is the seq assigned to the file's first logical line (1 for a fresh file).
func (p *RingStreamProvider) SetDiskFallback(path string, idx *fileindex.IncrementalOffsetIndex, startSeq int64) {
	if p == nil {
		return
	}
	if startSeq < 1 {
		startSeq = 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outPath = path
	p.index = idx
	p.seqBase = startSeq
	p.horizon = startSeq
}

// Push pushes text into the ring and bumps the cumulative receive count. It holds the
// provider mutex so Fetch goroutines see a consistent ring.
func (p *RingStreamProvider) Push(text string) domain.Line {
	if p == nil || p.buf == nil {
		return domain.Line{}
	}
	p.mu.Lock()
	rec := p.buf.Push(text)
	p.mu.Unlock()
	atomic.AddInt64(&p.totalRecv, 1)
	return rec
}

// AssignSeq advances the sequence counter without touching the ring. Used by stdin
// scrollback mode so the historical window loaded into the ring stays stable while
// new lines continue to be persisted to disk and indexed.
func (p *RingStreamProvider) AssignSeq() int64 {
	if p == nil || p.buf == nil {
		return 0
	}
	p.mu.Lock()
	seq := p.buf.AdvanceSeq()
	p.mu.Unlock()
	atomic.AddInt64(&p.totalRecv, 1)
	return seq
}

// HasDiskFallback reports whether disk-backed scrollback is available.
func (p *RingStreamProvider) HasDiskFallback() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.outPath != "" && p.index != nil
}

// NoteReceived bumps the cumulative receive count by n. Retained for callers that push
// directly into the ring (e.g. tests); new code should prefer [Push].
func (p *RingStreamProvider) NoteReceived(n int) {
	if p == nil || n <= 0 {
		return
	}
	atomic.AddInt64(&p.totalRecv, int64(n))
}

// NoteRotation records that firstSeqInNewFile is the seq of the line now at file offset 0.
// Seqs below firstSeqInNewFile become unreachable via disk fallback (they lived in a
// pre-rotation file). The offset index is reset to scan the new file from offset 0.
func (p *RingStreamProvider) NoteRotation(firstSeqInNewFile int64) {
	if p == nil || firstSeqInNewFile < 1 {
		return
	}
	p.mu.Lock()
	p.seqBase = firstSeqInNewFile
	p.horizon = firstSeqInNewFile
	idx := p.index
	p.mu.Unlock()
	if idx != nil {
		idx.Reset(0)
	}
}

// RefreshIndex grows the offset index to cover newly flushed bytes in the backing file.
func (p *RingStreamProvider) RefreshIndex(size int64) error {
	if p == nil || p.index == nil {
		return nil
	}
	return p.index.RefreshTo(size)
}

// Horizon returns the earliest seq reachable via disk fallback.
func (p *RingStreamProvider) Horizon() int64 {
	if p == nil {
		return 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.horizon < 1 {
		return 1
	}
	return p.horizon
}

// Fetch returns records for seqs in [firstSeq, lastSeq]. Seqs present in the live ring
// come from the ring; seqs outside the ring (evicted, or arrived while scrollback froze
// the ring) resolve through the --out file when disk fallback is enabled. Seqs below the
// rotation horizon are silently absent.
func (p *RingStreamProvider) Fetch(firstSeq, lastSeq int64) ([]domain.Line, error) {
	if p == nil || p.buf == nil || lastSeq < firstSeq || firstSeq < 1 {
		return nil, nil
	}
	p.mu.Lock()
	var ringRecs []domain.Line
	var ringFirst, ringLast int64
	if n := p.buf.Len(); n > 0 {
		ringFirst = p.buf.At(0).Seq
		ringLast = p.buf.At(n - 1).Seq
		for i := range n {
			rec := p.buf.At(i)
			if rec.Seq < firstSeq {
				continue
			}
			if rec.Seq > lastSeq {
				break
			}
			ringRecs = append(ringRecs, rec)
		}
	}
	outPath := p.outPath
	idx := p.index
	seqBase := p.seqBase
	horizon := p.horizon
	p.mu.Unlock()

	readDisk := func(dFirst, dLast int64) ([]domain.Line, error) {
		if outPath == "" || idx == nil {
			return nil, nil
		}
		dFirst = max(dFirst, horizon, seqBase)
		if dFirst > dLast {
			return nil, nil
		}
		offsets := idx.Snapshot()
		if len(offsets) == 0 {
			return nil, nil
		}
		fileFirst := int(dFirst - seqBase + 1)
		fileLast := int(dLast - seqBase + 1)
		recs, err := fileindex.ReadWindowRecords(outPath, offsets, fileFirst, fileLast)
		if err != nil {
			return nil, err
		}
		shift := seqBase - 1
		for i := range recs {
			recs[i].Seq += shift
		}
		return recs, nil
	}

	// Disk portion before the ring: seqs in [firstSeq, min(ringFirst-1, lastSeq)].
	var diskBefore []domain.Line
	if ringFirst == 0 || firstSeq < ringFirst {
		dLast := lastSeq
		if ringFirst > 0 && dLast >= ringFirst {
			dLast = ringFirst - 1
		}
		recs, err := readDisk(firstSeq, dLast)
		if err != nil {
			return nil, err
		}
		diskBefore = recs
	}

	// Disk portion after the ring: seqs > ringLast (scrollback-mode arrivals not yet in ring).
	var diskAfter []domain.Line
	if ringLast > 0 && lastSeq > ringLast {
		recs, err := readDisk(ringLast+1, lastSeq)
		if err != nil {
			return nil, err
		}
		diskAfter = recs
	}

	out := make([]domain.Line, 0, len(diskBefore)+len(ringRecs)+len(diskAfter))
	out = append(out, diskBefore...)
	out = append(out, ringRecs...)
	out = append(out, diskAfter...)
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

// FileSize returns the on-disk size of the --out file when disk fallback is enabled, else 0.
func (p *RingStreamProvider) FileSize() int64 {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	idx := p.index
	p.mu.Unlock()
	if idx == nil {
		return 0
	}
	return idx.IndexedBytes()
}

// EstimateBytes approximates the byte span of seqs on disk (ring seqs contribute 0 — not a
// disk-backed source). Used for the PRD §4.1 100 MiB disk-scan guardrail.
func (p *RingStreamProvider) EstimateBytes(firstSeq, lastSeq int64) int64 {
	if p == nil || lastSeq < firstSeq {
		return 0
	}
	p.mu.Lock()
	idx := p.index
	seqBase := p.seqBase
	horizon := p.horizon
	p.mu.Unlock()
	if idx == nil {
		return 0
	}
	diskFirst := max(firstSeq, horizon, seqBase)
	if diskFirst > lastSeq {
		return 0
	}
	offsets := idx.Snapshot()
	n := int64(len(offsets))
	if n == 0 {
		return 0
	}
	fileFirst := diskFirst - seqBase + 1
	fileLast := lastSeq - seqBase + 1
	if fileFirst > n {
		return 0
	}
	if fileLast > n {
		fileLast = n
	}
	start := offsets[fileFirst-1]
	var end int64
	if fileLast < n {
		end = offsets[fileLast]
	} else {
		end = idx.IndexedBytes()
	}
	if end < start {
		return 0
	}
	return end - start
}

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
