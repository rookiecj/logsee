package storage

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	defaultRotateKeep = 32
)

// LineAppender appends complete lines to a file and optionally rotates by size.
type LineAppender struct {
	path       string
	maxBytes   int64 // 0 disables size-based rotation
	rotateKeep int   // keep path.1 .. path.rotateKeep when rotating
	mu         sync.Mutex
	f          *os.File
	w          *bufio.Writer
	ticker     *time.Ticker
	stop       chan struct{}
}

// NewLineAppender opens path for append. maxOutFileBytes > 0 enables rotation: before each line,
// if the flushed file size plus that line would exceed maxOutFileBytes, the current file is renamed
// to path.1 and older path.N → path.N+1 (up to rotateKeep), then a new empty file is opened at path.
// maxOutFileBytes 0 disables rotation.
func NewLineAppender(path string, syncInterval time.Duration, maxOutFileBytes int64) (*LineAppender, error) {
	return newLineAppender(path, syncInterval, maxOutFileBytes, defaultRotateKeep)
}

func newLineAppender(path string, syncInterval time.Duration, maxOutFileBytes int64, rotateKeep int) (*LineAppender, error) {
	if rotateKeep < 1 {
		rotateKeep = 1
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	a := &LineAppender{
		path:       path,
		maxBytes:   maxOutFileBytes,
		rotateKeep: rotateKeep,
		f:          f,
		w:          bufio.NewWriterSize(f, 1<<16),
	}
	if syncInterval > 0 {
		a.ticker = time.NewTicker(syncInterval)
		a.stop = make(chan struct{})
		go func() {
			for {
				select {
				case <-a.ticker.C:
					a.mu.Lock()
					_ = a.w.Flush()
					_ = a.f.Sync()
					a.mu.Unlock()
				case <-a.stop:
					return
				}
			}
		}()
	}
	return a, nil
}

// WriteLine appends a line with newline.
func (a *LineAppender) WriteLine(line string) error {
	if a == nil {
		return nil
	}
	next := int64(len(line)) + 1
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.w.Flush(); err != nil {
		return err
	}
	if a.maxBytes > 0 {
		st, err := a.f.Stat()
		if err != nil {
			return err
		}
		if st.Size() > 0 && st.Size()+next > a.maxBytes {
			if err := a.rotateLocked(); err != nil {
				return err
			}
		}
	}
	if _, err := a.w.WriteString(line); err != nil {
		return err
	}
	if err := a.w.WriteByte('\n'); err != nil {
		return err
	}
	return nil
}

func (a *LineAppender) rotateLocked() error {
	if err := a.w.Flush(); err != nil {
		return err
	}
	if err := a.f.Sync(); err != nil {
		return err
	}
	if err := a.f.Close(); err != nil {
		return err
	}
	if err := shiftRotated(a.path, a.rotateKeep); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	a.f = f
	a.w = bufio.NewWriterSize(f, 1<<16)
	return nil
}

func shiftRotated(path string, keep int) error {
	tail := fmt.Sprintf("%s.%d", path, keep)
	if err := os.Remove(tail); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := keep - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		to := fmt.Sprintf("%s.%d", path, i+1)
		if err := renameIfExists(from, to); err != nil {
			return err
		}
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("storage: output %q missing after close", path)
		}
		return err
	}
	return os.Rename(path, path+".1")
}

func renameIfExists(from, to string) error {
	_, err := os.Stat(from)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Flush flushes the write buffer to the underlying file (does not fsync unless sync ticker does).
func (a *LineAppender) Flush() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.w.Flush()
}

// Close flushes and closes the file.
func (a *LineAppender) Close() error {
	if a == nil {
		return nil
	}
	if a.ticker != nil {
		a.ticker.Stop()
		close(a.stop)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.w.Flush(); err != nil {
		_ = a.f.Close()
		return err
	}
	if err := a.f.Sync(); err != nil {
		_ = a.f.Close()
		return err
	}
	return a.f.Close()
}
