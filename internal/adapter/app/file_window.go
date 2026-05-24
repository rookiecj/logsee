package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"logsee/internal/usecase"
)

type fileLineIndex struct {
	offsets []int64
	size    int64
}

type filteredOutputWindow struct {
	records          []usecase.OutputLogRecord
	startOutputIndex int
	totalMatches     int
	totalRawLines    int
	fileSize         int64
}

func buildFileLineIndex(ctx context.Context, path string) (fileLineIndex, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileLineIndex{}, fmt.Errorf("open rendered SOT %q: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var offsets []int64
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return fileLineIndex{}, ctx.Err()
		default:
		}

		lineStart := offset
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			offsets = append(offsets, lineStart)
			offset += int64(len(line))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fileLineIndex{}, fmt.Errorf("index rendered SOT %q: %w", path, err)
		}
	}

	return fileLineIndex{offsets: offsets, size: offset}, nil
}

func (i fileLineIndex) totalLines() int {
	return len(i.offsets)
}

func sourceFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat rendered SOT %q: %w", path, err)
	}
	return info.Size(), nil
}

func readRawLogWindow(ctx context.Context, path string, index fileLineIndex, startOutputIndex int, count int) ([]usecase.RawLogLine, error) {
	if count <= 0 || index.totalLines() == 0 {
		return nil, nil
	}
	if startOutputIndex < 0 {
		startOutputIndex = 0
	}
	if startOutputIndex >= index.totalLines() {
		return nil, nil
	}

	endOutputIndex := startOutputIndex + count
	if endOutputIndex > index.totalLines() {
		endOutputIndex = index.totalLines()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open windowed SOT %q: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Seek(index.offsets[startOutputIndex], io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek windowed SOT %q: %w", path, err)
	}

	reader := bufio.NewReader(file)
	lines := make([]usecase.RawLogLine, 0, endOutputIndex-startOutputIndex)
	for outputIndex := startOutputIndex; outputIndex < endOutputIndex; outputIndex++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		text, err := reader.ReadString('\n')
		if len(text) > 0 {
			text = strings.TrimSuffix(text, "\n")
			lines = append(lines, usecase.RawLogLine{
				RawLineNumber: outputIndex + 1,
				Text:          text,
			})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read windowed SOT %q: %w", path, err)
		}
	}
	return lines, nil
}

func findRawSearchMatch(ctx context.Context, path string, index fileLineIndex, matcher usecase.SearchMatcher, currentOutputIndex int, direction usecase.SearchDirection) (int, bool, error) {
	totalLines := index.totalLines()
	if totalLines == 0 {
		return currentOutputIndex, false, nil
	}
	if currentOutputIndex < 0 {
		currentOutputIndex = 0
	}
	if currentOutputIndex >= totalLines {
		currentOutputIndex = totalLines - 1
	}
	if direction == usecase.SearchDirectionPrevious {
		return findPreviousRawSearchMatch(ctx, path, index, matcher, currentOutputIndex)
	}
	return findNextRawSearchMatch(ctx, path, index, matcher, currentOutputIndex)
}

func findNextRawSearchMatch(ctx context.Context, path string, index fileLineIndex, matcher usecase.SearchMatcher, currentOutputIndex int) (int, bool, error) {
	startOutputIndex := currentOutputIndex + 1
	if startOutputIndex >= index.totalLines() {
		return currentOutputIndex, false, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return currentOutputIndex, false, fmt.Errorf("open search SOT %q: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Seek(index.offsets[startOutputIndex], io.SeekStart); err != nil {
		return currentOutputIndex, false, fmt.Errorf("seek search SOT %q: %w", path, err)
	}

	reader := bufio.NewReader(file)
	for outputIndex := startOutputIndex; outputIndex < index.totalLines(); outputIndex++ {
		select {
		case <-ctx.Done():
			return currentOutputIndex, false, ctx.Err()
		default:
		}

		text, err := reader.ReadString('\n')
		if len(text) > 0 {
			text = strings.TrimSuffix(text, "\n")
			if matcher.Match(text) {
				return outputIndex, true, nil
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return currentOutputIndex, false, fmt.Errorf("read search SOT %q: %w", path, err)
		}
	}
	return currentOutputIndex, false, nil
}

func findPreviousRawSearchMatch(ctx context.Context, path string, index fileLineIndex, matcher usecase.SearchMatcher, currentOutputIndex int) (int, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return currentOutputIndex, false, fmt.Errorf("open search SOT %q: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for outputIndex := currentOutputIndex - 1; outputIndex >= 0; outputIndex-- {
		select {
		case <-ctx.Done():
			return currentOutputIndex, false, ctx.Err()
		default:
		}

		if _, err := file.Seek(index.offsets[outputIndex], io.SeekStart); err != nil {
			return currentOutputIndex, false, fmt.Errorf("seek search SOT %q: %w", path, err)
		}
		reader.Reset(file)
		text, err := reader.ReadString('\n')
		if len(text) > 0 {
			text = strings.TrimSuffix(text, "\n")
			if matcher.Match(text) {
				return outputIndex, true, nil
			}
		}
		if err != nil && err != io.EOF {
			return currentOutputIndex, false, fmt.Errorf("read search SOT %q: %w", path, err)
		}
	}
	return currentOutputIndex, false, nil
}

func findFilteredSearchMatch(ctx context.Context, path string, filter usecase.CompiledFilter, matcher usecase.SearchMatcher, currentOutputIndex int, direction usecase.SearchDirection) (int, bool, error) {
	if currentOutputIndex < 0 {
		currentOutputIndex = 0
	}

	file, err := os.Open(path)
	if err != nil {
		return currentOutputIndex, false, fmt.Errorf("open filtered search SOT %q: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	outputIndex := 0
	previousMatch := -1
	for {
		select {
		case <-ctx.Done():
			return currentOutputIndex, false, ctx.Err()
		default:
		}

		text, err := reader.ReadString('\n')
		if len(text) > 0 {
			text = strings.TrimSuffix(text, "\n")
			if filter.Match(text) {
				if direction == usecase.SearchDirectionPrevious {
					if outputIndex >= currentOutputIndex {
						break
					}
					if matcher.Match(text) {
						previousMatch = outputIndex
					}
				} else if outputIndex > currentOutputIndex && matcher.Match(text) {
					return outputIndex, true, nil
				}
				outputIndex++
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return currentOutputIndex, false, fmt.Errorf("read filtered search SOT %q: %w", path, err)
		}
	}

	if previousMatch >= 0 {
		return previousMatch, true, nil
	}
	return currentOutputIndex, false, nil
}

func scanFilteredOutputWindow(ctx context.Context, path string, filter usecase.CompiledFilter, startOutputIndex int, capacity int) (filteredOutputWindow, error) {
	if startOutputIndex < 0 {
		startOutputIndex = 0
	}
	if capacity < 0 {
		capacity = 0
	}

	file, err := os.Open(path)
	if err != nil {
		return filteredOutputWindow{}, fmt.Errorf("open filtered SOT %q: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	window := filteredOutputWindow{
		records:          make([]usecase.OutputLogRecord, 0, capacity),
		startOutputIndex: startOutputIndex,
	}
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return filteredOutputWindow{}, ctx.Err()
		default:
		}

		text, err := reader.ReadString('\n')
		if len(text) > 0 {
			window.totalRawLines++
			offset += int64(len(text))
			text = strings.TrimSuffix(text, "\n")
			if filter.Match(text) {
				outputIndex := window.totalMatches
				if outputIndex >= startOutputIndex && len(window.records) < capacity {
					window.records = append(window.records, usecase.OutputLogRecord{
						RawLineNumber: window.totalRawLines,
						Text:          text,
					})
				}
				window.totalMatches++
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return filteredOutputWindow{}, fmt.Errorf("scan filtered SOT %q: %w", path, err)
		}
	}
	window.fileSize = offset
	return window, nil
}
