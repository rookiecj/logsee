package fileindex

import (
	"io"
	"os"
	"sync"
)

// IncrementalOffsetIndex maintains a line-start offset table for a file that is
// being appended to over time. RefreshTo extends the index to cover newly
// written bytes. Only bytes at or after the start position (passed at
// construction or Reset) are indexed; earlier bytes are opaque context.
//
// Designed for logsee's stdin tee: the writer appends full "text\n" records
// and calls RefreshTo with the latest flushed file size. Readers can then
// resolve evicted ring seqs through ReadWindowRecords using the offsets.
type IncrementalOffsetIndex struct {
	path         string
	mu           sync.Mutex
	offsets      []int64
	indexedBytes int64
	pendingStart int64
}

// NewIncrementalOffsetIndex starts indexing from startByte. Pass the file size
// observed at the moment indexing should begin (0 for an empty or fresh file).
func NewIncrementalOffsetIndex(path string, startByte int64) *IncrementalOffsetIndex {
	if startByte < 0 {
		startByte = 0
	}
	return &IncrementalOffsetIndex{
		path:         path,
		indexedBytes: startByte,
		pendingStart: startByte,
	}
}

// Path returns the file path this index is attached to.
func (idx *IncrementalOffsetIndex) Path() string {
	if idx == nil {
		return ""
	}
	return idx.path
}

// RefreshTo extends the index to cover bytes in [indexedBytes, size). No-op if
// size is not ahead of the last scan.
func (idx *IncrementalOffsetIndex) RefreshTo(size int64) error {
	if idx == nil {
		return nil
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if size <= idx.indexedBytes {
		return nil
	}
	f, err := os.Open(idx.path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(idx.indexedBytes, io.SeekStart); err != nil {
		return err
	}
	remaining := size - idx.indexedBytes
	buf := make([]byte, 8192)
	abs := idx.indexedBytes
	for remaining > 0 {
		toRead := min(int64(len(buf)), remaining)
		n, rerr := f.Read(buf[:toRead])
		for i := range n {
			p := abs + int64(i)
			if p == idx.pendingStart {
				idx.offsets = append(idx.offsets, p)
			}
			if buf[i] == '\n' {
				idx.pendingStart = p + 1
			}
		}
		abs += int64(n)
		remaining -= int64(n)
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}
	idx.indexedBytes = size
	return nil
}

// Len returns the number of lines currently indexed.
func (idx *IncrementalOffsetIndex) Len() int {
	if idx == nil {
		return 0
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return len(idx.offsets)
}

// IndexedBytes returns the absolute byte position scanned through so far.
func (idx *IncrementalOffsetIndex) IndexedBytes() int64 {
	if idx == nil {
		return 0
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.indexedBytes
}

// Snapshot returns a copy of the offsets table safe for concurrent readers.
func (idx *IncrementalOffsetIndex) Snapshot() []int64 {
	if idx == nil {
		return nil
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return append([]int64(nil), idx.offsets...)
}

// Reset clears the indexed offsets and restarts scanning from startByte. Used
// when the underlying file rotates and the index must follow the new file.
func (idx *IncrementalOffsetIndex) Reset(startByte int64) {
	if idx == nil {
		return
	}
	if startByte < 0 {
		startByte = 0
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.offsets = idx.offsets[:0]
	idx.indexedBytes = startByte
	idx.pendingStart = startByte
}
