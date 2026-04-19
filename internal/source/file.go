package source

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// FileSource reads a finite file line-by-line and emits each line via the
// LogSource channel. Seq starts at 1 and increments once per line; EOF
// closes the channel.
type FileSource struct {
	path string
	f    *os.File
	cap  int
}

// NewFile opens path for reading. The file is not opened until Lines is
// called so the constructor is cheap and test-friendly.
func NewFile(path string) *FileSource {
	return &FileSource{path: path, cap: 1024}
}

// WithChannelCapacity overrides the default producer channel capacity
// (1024). Useful for tests that want synchronous backpressure.
func (fs *FileSource) WithChannelCapacity(n int) *FileSource {
	if n > 0 {
		fs.cap = n
	}
	return fs
}

func (fs *FileSource) Lines(ctx context.Context) (<-chan domain.Line, error) {
	f, err := os.Open(fs.path)
	if err != nil {
		return nil, fmt.Errorf("source.File open %s: %w", fs.path, err)
	}
	fs.f = f
	ch := make(chan domain.Line, fs.cap)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var seq int64
		for scanner.Scan() {
			seq++
			line := domain.Line{Seq: seq, Text: scanner.Text()}
			select {
			case <-ctx.Done():
				return
			case ch <- line:
			}
		}
	}()
	return ch, nil
}

func (fs *FileSource) Close() error {
	if fs.f != nil {
		err := fs.f.Close()
		fs.f = nil
		return err
	}
	return nil
}
