package usecase

import (
	"context"
	"fmt"
	"logsee/internal/port"
	"strings"
	"testing"
)

type measuredSource struct {
	data   string
	bytes  int64
	maxEnd int64
}

func (s *measuredSource) Path() string                        { return "measured" }
func (s *measuredSource) Size(context.Context) (int64, error) { return int64(len(s.data)), nil }
func (s *measuredSource) ReadAt(ctx context.Context, offset int64, n int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	end := min(int64(len(s.data)), offset+int64(n))
	s.bytes += end - offset
	s.maxEnd = max(s.maxEnd, end)
	return []byte(s.data[offset:end]), nil
}
func TestSourceSnapshotBounds(t *testing.T) {
	raw := &measuredSource{data: "one\ntwo\nSECRET\n"}
	s, _ := NewLogSource(raw)
	var got []string
	err := s.WalkLines(context.Background(), port.LogSnapshot{Generation: 1, ByteLimit: 8}, 1, 99, func(n int, text string) error { got = append(got, text); return nil })
	if err != nil || strings.Join(got, "|") != "one|two" || raw.maxEnd > 8 {
		t.Fatalf("got=%v err=%v readEnd=%d", got, err, raw.maxEnd)
	}
}
func TestPageWindowBound(t *testing.T) {
	raw := &measuredSource{data: strings.Repeat("x\n", DefaultRawPageSize*10)}
	s, _ := NewLogSource(raw)
	snap := port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}
	err := s.WalkLines(context.Background(), snap, 1, DefaultRawPageSize*10, func(n int, text string) error {
		if len(s.pages.LoadedPageIndexes()) > 3 {
			return fmt.Errorf("cache exceeded 3 pages")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.pages.metadata) > 21 {
		t.Fatalf("metadata is not sparse: %d", len(s.pages.metadata))
	}
}
func TestAppendRetainsRawCoordinates(t *testing.T) {
	raw := &measuredSource{data: strings.Repeat("x", DefaultRawPageSize*4) + " tail"}
	s, _ := NewLogSource(raw)
	if _, err := s.Refresh(context.Background(), port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}); err != nil {
		t.Fatal(err)
	}
	raw.data += " more\nnext\n"
	var got []string
	err := s.WalkLines(context.Background(), port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}, 1, 3, func(n int, text string) error { got = append(got, text); return nil })
	if err != nil || len(got) != 2 || got[1] != "next" || !strings.HasSuffix(got[0], "tail more") {
		t.Fatalf("lines=%d err=%v", len(got), err)
	}
}
func TestSourceReplacementInvalidatesSnapshot(t *testing.T) {
	raw := &measuredSource{data: "old\n"}
	s, _ := NewLogSource(raw)
	s.Refresh(context.Background(), port.LogSnapshot{Generation: 1, ByteLimit: 4})
	raw.data = "new\n"
	err := s.WalkLines(context.Background(), port.LogSnapshot{Generation: 2, ByteLimit: 4}, 1, 1, func(_ int, text string) error {
		if text != "new" {
			return fmt.Errorf("stale %q", text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
