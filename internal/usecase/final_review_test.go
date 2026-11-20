package usecase

import (
	"context"
	"errors"
	"logsee/internal/port"
	"strings"
	"testing"
)

type failPageSource struct {
	data       string
	failOffset int64
	fail       bool
}

func (s *failPageSource) Path() string                        { return "fake" }
func (s *failPageSource) Size(context.Context) (int64, error) { return int64(len(s.data)), nil }
func (s *failPageSource) ReadAt(_ context.Context, o int64, n int) ([]byte, error) {
	if s.fail && o >= s.failOffset {
		s.fail = false
		return nil, context.Canceled
	}
	return []byte(s.data[o:min(int64(len(s.data)), o+int64(n))]), nil
}
func TestFinalReviewCanceledAppendPreservesMetadata(t *testing.T) {
	raw := &failPageSource{data: strings.Repeat("x\n", DefaultRawPageSize/2)}
	src, _ := NewLogSource(raw)
	snap := port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}
	if _, err := src.Refresh(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	raw.data += strings.Repeat("x\n", DefaultRawPageSize*3/2)
	snap.ByteLimit = int64(len(raw.data))
	raw.failOffset = DefaultRawPageSize
	raw.fail = true
	if _, err := src.Refresh(context.Background(), snap); !errors.Is(err, context.Canceled) {
		t.Fatalf("fixture %v", err)
	}
	total, err := src.Refresh(context.Background(), snap)
	if err != nil || total != len(raw.data)/2 {
		t.Fatalf("retry after canceled append: lines=%d want=%d err=%v", total, len(raw.data)/2, err)
	}
}
