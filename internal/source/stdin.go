package source

import (
	"bufio"
	"context"
	"io"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// ReaderSource adapts any io.Reader to the LogSource port. It does not
// own the reader — Close is a no-op. Use this for stdin, pipes, and tests.
type ReaderSource struct {
	r   io.Reader
	cap int
}

// NewReader wraps r as a LogSource.
func NewReader(r io.Reader) *ReaderSource {
	return &ReaderSource{r: r, cap: 1024}
}

func (s *ReaderSource) WithChannelCapacity(n int) *ReaderSource {
	if n > 0 {
		s.cap = n
	}
	return s
}

func (s *ReaderSource) Lines(ctx context.Context) (<-chan domain.Line, error) {
	ch := make(chan domain.Line, s.cap)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(s.r)
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

func (s *ReaderSource) Close() error { return nil }
