package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"logsee/internal/adapter/filesystem"
	"logsee/internal/port"
	"logsee/internal/usecase"
)

type selectionOperationKind int

const (
	selectionExtend selectionOperationKind = iota
	selectionToggle
	selectionClearRange
)

type selectionOperation struct {
	Kind                            selectionOperationKind
	AnchorRaw, AnchorOutput, Output int
}
type sourceRequest struct {
	SelectionOps                           []selectionOperation
	IntentRevision, AcknowledgedGeneration uint64
	AnchorOutput                           int
	ID, FilterRevision                     uint64
	Session                                usecase.InputSession
	LogType                                usecase.LogType
	FilterText                             string
	Navigation                             usecase.NavigationOptions
	Search                                 string
	SearchDirection                        usecase.SearchDirection
	SearchRequested                        bool
	Copy, Select                           bool
	Anchor                                 int
	Selection                              *usecase.SelectionState
	Clipboard                              port.ClipboardWriter
}
type sourceResult struct {
	IntentRevision                uint64
	ID, FilterRevision            uint64
	Snapshot                      port.LogSnapshot
	Records                       []usecase.OutputLogRecord
	Start, TotalRaw, TotalMatches int
	Navigation                    *usecase.NavigationState
	LogType                       usecase.LogType
	Selection                     *usecase.SelectionState
	Message                       string
	Replaced, Retry               bool
	Err                           error
}
type sourceJobs struct {
	ctx        context.Context
	cancel     context.CancelFunc
	requests   chan sourceRequest
	results    chan sourceResult
	done       chan struct{}
	mu         sync.Mutex
	active     context.CancelFunc
	latest     uint64
	path       string
	source     *usecase.LogSource
	filtered   *usecase.FilteredSource
	info       os.FileInfo
	generation uint64
	finalType  usecase.LogType
}

func newSourceJobs(ctx context.Context, path string) *sourceJobs {
	return newSourceJobsWithSource(ctx, path, filesystem.NewFileSource(path))
}
func newSourceJobsWithSource(ctx context.Context, path string, raw port.RawPageSource) *sourceJobs {
	ctx, cancel := context.WithCancel(ctx)
	j := &sourceJobs{ctx: ctx, cancel: cancel, path: path, requests: make(chan sourceRequest, 1), results: make(chan sourceResult, 1), done: make(chan struct{}), generation: 1}
	j.source, _ = usecase.NewLogSource(raw)
	j.filtered = usecase.NewFilteredSource(j.source)
	go j.run()
	return j
}
func (j *sourceJobs) Submit(req sourceRequest) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.latest = req.ID
	if j.active != nil {
		j.active()
	}
	select {
	case <-j.requests:
	default:
	}
	select {
	case j.requests <- req:
	case <-j.ctx.Done():
	}
}
func (j *sourceJobs) Close() { j.cancel(); <-j.done }
func (j *sourceJobs) run() {
	defer close(j.done)
	defer close(j.results)
	for {
		select {
		case <-j.ctx.Done():
			return
		case req := <-j.requests:
			j.mu.Lock()
			if req.ID != j.latest {
				j.mu.Unlock()
				continue
			}
			ctx, cancel := context.WithCancel(j.ctx)
			j.active = cancel
			j.mu.Unlock()
			result := j.execute(ctx, req)
			if ctx.Err() == nil {
				select {
				case j.results <- result:
				case <-ctx.Done():
				}
			}
			cancel()
		}
	}
}
func sourceReplaced(before, after os.FileInfo) bool {
	return before != nil && (!os.SameFile(before, after) || after.Size() < before.Size() || after.Size() == before.Size() && !after.ModTime().Equal(before.ModTime()))
}
func (j *sourceJobs) execute(ctx context.Context, req sourceRequest) (result sourceResult) {
	result.ID = req.ID
	result.IntentRevision = req.IntentRevision
	result.FilterRevision = req.FilterRevision
	result.LogType = req.LogType
	info, err := os.Stat(j.path)
	if err != nil {
		result.Err = fmt.Errorf("stat rendered SOT: %w", err)
		return
	}
	result.Replaced = sourceReplaced(j.info, info)
	if result.Replaced {
		j.generation++
	}
	result.Replaced = result.Replaced || req.AcknowledgedGeneration != 0 && req.AcknowledgedGeneration != j.generation
	j.info = info
	size := info.Size()
	if limit, ok := req.Session.PublishedLimit(); ok {
		size = min(size, limit)
	}
	result.Snapshot = port.LogSnapshot{Generation: j.generation, ByteLimit: size}
	if j.finalType != "" {
		result.LogType = j.finalType
	}
	if result.LogType == usecase.LogTypeAuto {
		cfg, e := usecase.ResolveLogTypeConfig("", req.Session.LogTypeConfig)
		if e != nil {
			result.Err = e
			return
		}
		sampler := filesystem.NewFileSource(j.path).WithLimit(func() int64 { return size })
		var lines []string
		if req.Session.Progress().EOF {
			lines, e = sampler.SampleLines(ctx, cfg.ProbeLines)
		} else {
			lines, e = sampler.SampleCompleteLines(ctx, cfg.ProbeLines)
		}
		if e != nil {
			result.Err = e
			return
		}
		if req.Session.Progress().EOF || len(lines) >= cfg.ProbeLines {
			result.LogType, e = usecase.DetectLogType(ctx, sampler, cfg)
			if e != nil {
				result.Err = e
				return
			}
			j.finalType = result.LogType
		}
	}
	filter, e := usecase.CompileFilter(req.FilterText, usecase.FilterOptions{LogType: result.LogType, IgnoreCase: req.Session.IgnoreCase})
	if e != nil {
		result.Err = e
		return
	}
	// A type transition changes level-filter semantics even with the same query.
	revision := req.FilterRevision * 2
	if result.LogType != usecase.LogTypeAuto {
		revision++
	}
	fReq := usecase.FilterRequest{Snapshot: result.Snapshot, FilterRevision: revision}
	emptyFilter := strings.TrimSpace(req.FilterText) == ""
	if emptyFilter {
		result.TotalRaw, err = j.source.Refresh(ctx, result.Snapshot)
		result.TotalMatches = result.TotalRaw
	} else {
		var w usecase.FilterWindow
		w, err = j.filtered.ScanFilter(ctx, fReq, filter)
		result.TotalRaw = w.TotalRawLines
		result.TotalMatches = w.TotalMatches
	}
	if err != nil {
		result.Err = err
		return
	}
	navOptions := req.Navigation
	navOptions.OutputCount = result.TotalMatches
	if result.Replaced {
		navOptions.CursorOutputIndex = 0
		navOptions.ScrollOffset = 0
		req.Selection = usecase.NewSelectionState()
		req.Copy = false
		req.Select = false
	}
	nav, _ := usecase.NewNavigationState(navOptions)
	if req.SearchRequested {
		target, moved, e := j.filtered.Search(ctx, fReq, filter, usecase.NewSearchMatcher(req.Search), nav.CursorOutputIndex(), req.SearchDirection)
		if e != nil {
			result.Err = e
			return
		}
		if moved {
			navOptions.CursorOutputIndex = target
			navOptions.Follow = false
			navOptions.ScrollOffset = nav.ScrollOffset()
			nav, _ = usecase.NewNavigationState(navOptions)
		} else {
			result.Message = "no match"
		}
	}
	result.Navigation = nav
	result.Start = nav.ScrollOffset()
	capacity := usecase.DefaultOutputLogCacheCapacity
	if emptyFilter {
		capacity = req.Navigation.ViewportHeight
	}
	fReq.StartOutputIndex = result.Start
	fReq.Capacity = capacity
	if emptyFilter {
		err = j.source.WalkLines(ctx, result.Snapshot, result.Start+1, result.Start+capacity, func(n int, text string) error {
			result.Records = append(result.Records, usecase.OutputLogRecord{RawLineNumber: n, Text: text})
			return nil
		})
	} else {
		var w usecase.FilterWindow
		w, err = j.filtered.ScanFilter(ctx, fReq, filter)
		result.Records = w.Records
	}
	if err != nil {
		result.Err = err
		return
	}
	cursor := 0
	for i, r := range result.Records {
		if result.Start+i == nav.CursorOutputIndex() {
			cursor = r.RawLineNumber
			break
		}
	}
	resolveRaw := func(output int) (int, error) {
		if output < 0 || output >= result.TotalMatches {
			return 0, nil
		}
		if emptyFilter {
			return output + 1, nil
		}
		seek := fReq
		seek.StartOutputIndex = output
		seek.Capacity = 1
		window, e := j.filtered.ScanFilter(ctx, seek, filter)
		if e != nil {
			return 0, e
		}
		if len(window.Records) == 0 {
			return 0, nil
		}
		return window.Records[0].RawLineNumber, nil
	}
	if req.Select {
		selection := req.Selection
		for _, op := range req.SelectionOps {
			if op.Kind == selectionClearRange {
				selection.ClearRange()
				continue
			}
			end, e := resolveRaw(op.Output)
			if e != nil {
				result.Err = e
				return
			}
			if end == 0 {
				continue
			}
			if op.Kind == selectionToggle {
				selection.ClearRange()
				selection.TogglePicked(end)
				continue
			}
			anchor := op.AnchorRaw
			if anchor == 0 {
				anchor, e = resolveRaw(op.AnchorOutput)
				if e != nil {
					result.Err = e
					return
				}
			}
			if anchor == 0 {
				continue
			}
			selection.StartOrUpdateRange(anchor, end)
			err = j.source.WalkLines(ctx, result.Snapshot, min(anchor, end), max(anchor, end), func(n int, text string) error {
				if filter.Match(text) && !selection.IsPicked(n) {
					selection.TogglePicked(n)
				}
				return nil
			})
			if err != nil {
				result.Err = err
				return
			}
		}
		result.Selection = selection
	}

	if req.Copy {
		copied := usecase.CopySelectedFromSource(ctx, req.Clipboard, snapshotLines{j.source, result.Snapshot}, filter, req.Selection, cursor)
		result.Message = copied.Message
	}
	after, e := os.Stat(j.path)
	if e != nil {
		result.Err = e
		return
	}
	if sourceReplaced(info, after) {
		result.Retry = true
	}
	return
}

type snapshotLines struct {
	source   *usecase.LogSource
	snapshot port.LogSnapshot
}

func (s snapshotLines) WalkLines(ctx context.Context, first, last int, visit func(int, string) error) error {
	return s.source.WalkLines(ctx, s.snapshot, first, last, visit)
}
func sourceJobCanceled(err error) bool { return errors.Is(err, context.Canceled) }
