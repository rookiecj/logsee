package usecase

import (
	"context"
	"logsee/internal/port"
	"sort"
)

type FilterRequest struct {
	FilterRevision   uint64
	Snapshot         port.LogSnapshot
	StartOutputIndex int
	Capacity         int
}
type FilterWindow struct {
	Request                     FilterRequest
	Records                     []OutputLogRecord
	TotalRawLines, TotalMatches int
	ScannedBytes                int64
}
type filterPage struct{ page, firstRaw, prefix, count int }
type FilteredSource struct {
	source          *LogSource
	snapshot        port.LogSnapshot
	revision        uint64
	ready           bool
	pages           []filterPage
	total, totalRaw int
	cache           FilterWindow
	rescanFirst     int
}

func NewFilteredSource(source *LogSource) *FilteredSource { return &FilteredSource{source: source} }
func (f *FilteredSource) index(ctx context.Context, req FilterRequest, filter CompiledFilter) (int64, error) {
	if f.ready && req.Snapshot == f.snapshot && req.FilterRevision == f.revision {
		return 0, nil
	}
	total, err := f.source.Refresh(ctx, req.Snapshot)
	if err != nil {
		return 0, err
	}
	start, scanned := 1, int64(0)
	reset := !f.ready || req.FilterRevision != f.revision || req.Snapshot.Generation != f.snapshot.Generation || req.Snapshot.ByteLimit < f.snapshot.ByteLimit
	if reset {
		f.pages = nil
		f.total = 0
	} else if len(f.pages) > 0 {
		// The final line may span several pages and may change on append.
		tail := f.pages[len(f.pages)-1]
		start = tail.firstRaw
		f.total = tail.prefix
		f.pages = f.pages[:len(f.pages)-1]
	}
	f.rescanFirst = start
	f.ready = false // A canceled partial index is never reused.
	err = f.source.walk(ctx, req.Snapshot, start, total, func(n int, offset int64, text string) error {
		page := int(offset / DefaultRawPageSize)
		if len(f.pages) == 0 || f.pages[len(f.pages)-1].page != page {
			f.pages = append(f.pages, filterPage{page: page, firstRaw: n, prefix: f.total})
		}
		if filter.Match(text) {
			f.pages[len(f.pages)-1].count++
			f.total++
		}
		scanned += int64(len(text) + 1)
		return nil
	})
	if err != nil {
		return scanned, err
	}
	f.snapshot = req.Snapshot
	f.revision = req.FilterRevision
	f.totalRaw = total
	f.ready = true
	return scanned, nil
}
func (f *FilteredSource) ScanFilter(ctx context.Context, req FilterRequest, filter CompiledFilter) (FilterWindow, error) {
	scanned, err := f.index(ctx, req, filter)
	result := FilterWindow{Request: req, TotalMatches: f.total, TotalRawLines: f.totalRaw, ScannedBytes: scanned}
	if err != nil {
		return result, err
	}
	if req.Capacity <= 0 || f.total == 0 {
		return result, nil
	}
	cached := f.cache
	if cached.Request.FilterRevision == req.FilterRevision && cached.Request.Snapshot.Generation == req.Snapshot.Generation && cached.Request.StartOutputIndex == req.StartOutputIndex && cached.Request.Capacity == req.Capacity && len(cached.Records) > 0 {
		sameSnapshot := cached.Request.Snapshot == req.Snapshot
		unchanged := len(cached.Records) == req.Capacity && cached.Records[len(cached.Records)-1].RawLineNumber < f.rescanFirst && cached.Request.Snapshot.ByteLimit < req.Snapshot.ByteLimit
		if sameSnapshot || unchanged {
			result.Records = cached.Records
			f.cache = result
			return result, nil
		}
	}

	start := max(0, req.StartOutputIndex)
	if start >= f.total {
		return result, nil
	}
	p := sort.Search(len(f.pages), func(i int) bool { return f.pages[i].prefix+f.pages[i].count > start })
	if p == len(f.pages) {
		return result, nil
	}
	output := f.pages[p].prefix
	err = f.source.WalkLines(ctx, req.Snapshot, f.pages[p].firstRaw, f.totalRaw, func(n int, text string) error {
		if filter.Match(text) {
			if output >= start {
				result.Records = append(result.Records, OutputLogRecord{n, text})
			}
			output++
		}
		if len(result.Records) >= req.Capacity {
			return errWalkDone
		}
		return nil
	})
	if err == nil {
		f.cache = result
	}
	return result, err
}
func (f *FilteredSource) Search(ctx context.Context, req FilterRequest, filter CompiledFilter, matcher SearchMatcher, current int, direction SearchDirection) (int, bool, error) {
	if _, err := f.index(ctx, req, filter); err != nil {
		return current, false, err
	}
	if f.total == 0 {
		return current, false, nil
	}
	// Search bounded output windows. Previous navigation never rescans the prefix
	// merely to recover output coordinates.
	const block = 256
	if direction == SearchDirectionPrevious {
		end := current
		for end > 0 {
			req.StartOutputIndex = max(0, end-block)
			req.Capacity = end - req.StartOutputIndex
			w, err := f.ScanFilter(ctx, req, filter)
			if err != nil {
				return current, false, err
			}
			for i := len(w.Records) - 1; i >= 0; i-- {
				if matcher.Match(w.Records[i].Text) {
					return req.StartOutputIndex + i, true, nil
				}
			}
			end = req.StartOutputIndex
		}
	} else {
		for start := current + 1; start < f.total; start += block {
			req.StartOutputIndex = start
			req.Capacity = block
			w, err := f.ScanFilter(ctx, req, filter)
			if err != nil {
				return current, false, err
			}
			for i, r := range w.Records {
				if matcher.Match(r.Text) {
					return start + i, true, nil
				}
			}
		}
	}
	return current, false, nil
}
