package app

import (
	"context"
	"fmt"
	"logsee/internal/adapter/filesystem"
	"logsee/internal/port"
	"logsee/internal/usecase"
	"os"
)

type fileLineIndex struct {
	source   *usecase.LogSource
	snapshot port.LogSnapshot
	size     int64
	total    int
	filtered *usecase.FilteredSource
}
type filteredOutputWindow struct {
	records                                       []usecase.OutputLogRecord
	startOutputIndex, totalMatches, totalRawLines int
	fileSize                                      int64
}

func buildFileLineIndex(ctx context.Context, path string) (fileLineIndex, error) {
	size, err := sourceFileSize(path)
	if err != nil {
		return fileLineIndex{}, err
	}
	return buildFileLineIndexLimit(ctx, path, size)
}
func buildFileLineIndexLimit(ctx context.Context, path string, size int64) (fileLineIndex, error) {
	source, err := usecase.NewLogSource(filesystem.NewFileSource(path))
	if err != nil {
		return fileLineIndex{}, err
	}
	index := fileLineIndex{source: source, snapshot: port.LogSnapshot{Generation: 1, ByteLimit: size}, size: size}
	index.total, err = source.Refresh(ctx, index.snapshot)
	index.filtered = usecase.NewFilteredSource(source)
	return index, err
}
func extendFileLineIndex(ctx context.Context, path string, index fileLineIndex) (fileLineIndex, error) {
	size, err := sourceFileSize(path)
	if err != nil {
		return index, err
	}
	return extendFileLineIndexLimit(ctx, index, size)
}
func extendFileLineIndexLimit(ctx context.Context, index fileLineIndex, size int64) (fileLineIndex, error) {
	if size < index.size {
		index.snapshot.Generation++
	}
	index.snapshot.ByteLimit = size
	index.size = size
	total, err := index.source.Refresh(ctx, index.snapshot)
	index.total = total
	return index, err
}
func (i fileLineIndex) totalLines() int { return i.total }
func sourceFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat rendered SOT %q: %w", path, err)
	}
	return info.Size(), nil
}
func readRawLogWindow(ctx context.Context, _ string, index fileLineIndex, start, count int) ([]usecase.RawLogLine, error) {
	if count <= 0 {
		return nil, nil
	}
	var logs []usecase.RawLogLine
	err := index.source.WalkLines(ctx, index.snapshot, max(0, start)+1, max(0, start)+count, func(n int, text string) error {
		logs = append(logs, usecase.RawLogLine{RawLineNumber: n, Text: text})
		return nil
	})
	return logs, err
}

type selectedFileLines struct {
	path  string
	index fileLineIndex
}

func (s selectedFileLines) WalkLines(ctx context.Context, first, last int, visit func(int, string) error) error {
	return s.index.source.WalkLines(ctx, s.index.snapshot, first, last, visit)
}
func findRawSearchMatch(ctx context.Context, _ string, index fileLineIndex, matcher usecase.SearchMatcher, current int, direction usecase.SearchDirection) (int, bool, error) {
	return index.filtered.Search(ctx, usecase.FilterRequest{Snapshot: index.snapshot}, usecase.CompiledFilter{}, matcher, current, direction)
}
func findFilteredSearchMatch(ctx context.Context, path string, filter usecase.CompiledFilter, matcher usecase.SearchMatcher, current int, direction usecase.SearchDirection) (int, bool, error) {
	index, err := buildFileLineIndex(ctx, path)
	if err != nil {
		return current, false, err
	}
	return index.filtered.Search(ctx, usecase.FilterRequest{Snapshot: index.snapshot}, filter, matcher, current, direction)
}
func scanFilteredOutputWindow(ctx context.Context, path string, filter usecase.CompiledFilter, start, capacity int) (filteredOutputWindow, error) {
	index, err := buildFileLineIndex(ctx, path)
	if err != nil {
		return filteredOutputWindow{}, err
	}
	return scanIndexedFilter(ctx, index, filter, start, capacity, 0)
}
func scanIndexedFilter(ctx context.Context, index fileLineIndex, filter usecase.CompiledFilter, start, capacity int, revision uint64) (filteredOutputWindow, error) {
	w, err := index.filtered.ScanFilter(ctx, usecase.FilterRequest{Snapshot: index.snapshot, FilterRevision: revision, StartOutputIndex: start, Capacity: capacity}, filter)
	return filteredOutputWindow{records: w.Records, startOutputIndex: w.Request.StartOutputIndex, totalMatches: w.TotalMatches, totalRawLines: w.TotalRawLines, fileSize: index.size}, err
}
