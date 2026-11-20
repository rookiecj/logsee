package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"logsee/internal/port"
	"sort"
	"strings"
)

// LogSource owns a three-page raw cache and one small checkpoint per byte page.
// Its owner serializes calls; consumers pass immutable snapshots and receive values.
type LogSource struct {
	pages    *RawPagePipeline
	snapshot port.LogSnapshot
	starts   []PageMetadata
	total    int
}

func NewLogSource(source port.RawPageSource) (*LogSource, error) {
	p, err := NewRawPagePipeline(source, RawPageOptions{})
	if err != nil {
		return nil, err
	}
	return &LogSource{pages: p}, nil
}
func (s *LogSource) Refresh(ctx context.Context, snap port.LogSnapshot) (int, error) {
	if snap.ByteLimit < 0 {
		return 0, fmt.Errorf("negative snapshot size")
	}
	changed := snap != s.snapshot || !s.pages.fileSizeKnown
	if !changed {
		return s.total, nil
	}
	reset := snap.Generation != s.snapshot.Generation || snap.ByteLimit < s.snapshot.ByteLimit
	s.pages.setSnapshot(snap.ByteLimit, reset)
	if err := s.pages.ensureMetadataThrough(ctx, s.pages.totalPages-1); err != nil {
		return 0, err
	}
	s.starts = s.starts[:0]
	s.total = 0
	for i := 0; i < s.pages.totalPages; i++ {
		m := s.pages.metadata[i]
		s.total += m.RawLineCount
		if m.RawLineCount > 0 {
			s.starts = append(s.starts, m)
		}
	}
	s.snapshot = snap
	return s.total, nil
}
func (s *LogSource) WalkLines(ctx context.Context, snap port.LogSnapshot, first, last int, visit func(int, string) error) error {
	return s.walk(ctx, snap, first, last, func(n int, _ int64, text string) error { return visit(n, text) })
}

var errWalkDone = errors.New("walk done")

func (s *LogSource) walk(ctx context.Context, snap port.LogSnapshot, first, last int, visit func(int, int64, string) error) error {
	total, err := s.Refresh(ctx, snap)
	if err != nil {
		return err
	}
	first = max(1, first)
	last = min(last, total)
	if first > last || len(s.starts) == 0 {
		return nil
	}
	index := sort.Search(len(s.starts), func(i int) bool { return s.starts[i].FirstRawLineNumber > first }) - 1
	if index < 0 {
		index = 0
	}
	m := s.starts[index]
	n := m.FirstRawLineNumber
	offset := m.FirstRawLineOffset
	lineStart := offset
	var pending []byte
	for pageIndex := int(offset / int64(s.pages.pageSize)); offset < snap.ByteLimit; pageIndex++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.pages.LoadPageWindow(ctx, pageIndex); err != nil {
			return err
		}
		page, _ := s.pages.LoadedPage(pageIndex)
		data := page.Data[offset-page.Metadata.ByteStart:]
		for len(data) > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := bytes.IndexByte(data, '\n')
			if end < 0 {
				pending = append(pending, data...)
				offset += int64(len(data))
				break
			}
			if n >= first {
				var text string
				if len(pending) > 0 {
					pending = append(pending, data[:end]...)
					text = string(pending)
				} else {
					text = string(data[:end])
				}
				if err := visit(n, lineStart, strings.TrimSuffix(text, "\r")); err != nil {
					if errors.Is(err, errWalkDone) {
						return nil
					}
					return err
				}
			}
			pending = pending[:0]
			offset += int64(end + 1)
			data = data[end+1:]
			n++
			lineStart = offset
			if n > last {
				return nil
			}
		}
	}
	if len(pending) > 0 && n >= first && n <= last {
		err := visit(n, lineStart, strings.TrimSuffix(string(pending), "\r"))
		if !errors.Is(err, errWalkDone) {
			return err
		}
	}
	return nil
}
