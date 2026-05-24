package usecase

import (
	"context"
	"fmt"
	"sort"

	"logsee/internal/port"
)

const DefaultRawPageSize = 64 * 1024

type RawPageOptions struct {
	PageSize int
}

type PageMetadata struct {
	PageIndex          int
	ByteStart          int64
	ByteEnd            int64
	FirstRawLineNumber int
	RawLineCount       int
}

type RawPage struct {
	Metadata PageMetadata
	Data     []byte
}

type RawPagePipeline struct {
	source   port.RawPageSource
	pageSize int

	fileSizeKnown bool
	fileSize      int64
	totalPages    int

	pages    map[int]RawPage
	metadata map[int]PageMetadata
	scanned  pageScanState
}

type pageScanState struct {
	nextPageIndex  int
	nextLine       int
	pageStartsLine bool
}

func NewRawPagePipeline(source port.RawPageSource, options RawPageOptions) (*RawPagePipeline, error) {
	if source == nil {
		return nil, fmt.Errorf("raw page source is required")
	}
	pageSize := options.PageSize
	if pageSize == 0 {
		pageSize = DefaultRawPageSize
	}
	if pageSize < 1 {
		return nil, fmt.Errorf("raw page size must be 1 or greater")
	}

	return &RawPagePipeline{
		source:   source,
		pageSize: pageSize,
		pages:    map[int]RawPage{},
		metadata: map[int]PageMetadata{},
		scanned: pageScanState{
			nextLine:       1,
			pageStartsLine: true,
		},
	}, nil
}

func (p *RawPagePipeline) LoadPageWindow(ctx context.Context, currentPage int) error {
	if currentPage < 0 {
		return fmt.Errorf("current page must be 0 or greater")
	}
	if err := p.ensureFileSize(ctx); err != nil {
		return err
	}
	if p.totalPages == 0 {
		p.pages = map[int]RawPage{}
		return nil
	}
	if currentPage >= p.totalPages {
		return fmt.Errorf("current page %d outside page range 0-%d", currentPage, p.totalPages-1)
	}

	window := p.windowForPage(currentPage)
	if err := p.ensureMetadataThrough(ctx, window[len(window)-1]); err != nil {
		return err
	}
	return p.loadOnlyWindow(ctx, window)
}

func (p *RawPagePipeline) LoadPageWindowForLine(ctx context.Context, rawLineNumber int) error {
	if rawLineNumber < 1 {
		return fmt.Errorf("raw line number must be 1 or greater")
	}
	if err := p.ensureFileSize(ctx); err != nil {
		return err
	}
	for pageIndex := 0; pageIndex < p.totalPages; pageIndex++ {
		if err := p.ensureMetadataThrough(ctx, pageIndex); err != nil {
			return err
		}
		meta := p.metadata[pageIndex]
		if meta.RawLineCount == 0 {
			continue
		}
		lastLine := meta.FirstRawLineNumber + meta.RawLineCount - 1
		if rawLineNumber >= meta.FirstRawLineNumber && rawLineNumber <= lastLine {
			return p.LoadPageWindow(ctx, pageIndex)
		}
	}
	return fmt.Errorf("raw line %d not found in source %q", rawLineNumber, p.source.Path())
}

func (p *RawPagePipeline) LoadedPageIndexes() []int {
	indexes := make([]int, 0, len(p.pages))
	for index := range p.pages {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

func (p *RawPagePipeline) PageMetadata(pageIndex int) (PageMetadata, bool) {
	metadata, ok := p.metadata[pageIndex]
	return metadata, ok
}

func (p *RawPagePipeline) LoadedPage(pageIndex int) (RawPage, bool) {
	page, ok := p.pages[pageIndex]
	return page, ok
}

func (p *RawPagePipeline) ensureFileSize(ctx context.Context) error {
	if p.fileSizeKnown {
		return nil
	}
	size, err := p.source.Size(ctx)
	if err != nil {
		return fmt.Errorf("read raw page source size %q: %w", p.source.Path(), err)
	}
	if size < 0 {
		return fmt.Errorf("raw page source size must be 0 or greater")
	}
	p.fileSize = size
	p.totalPages = int((size + int64(p.pageSize) - 1) / int64(p.pageSize))
	p.fileSizeKnown = true
	return nil
}

func (p *RawPagePipeline) ensureMetadataThrough(ctx context.Context, pageIndex int) error {
	if pageIndex < 0 {
		return nil
	}
	if err := p.ensureFileSize(ctx); err != nil {
		return err
	}
	if p.totalPages == 0 {
		return nil
	}
	if pageIndex >= p.totalPages {
		return fmt.Errorf("page %d outside page range 0-%d", pageIndex, p.totalPages-1)
	}

	for p.scanned.nextPageIndex <= pageIndex {
		data, meta, err := p.scanNextPage(ctx)
		if err != nil {
			return err
		}
		p.metadata[meta.PageIndex] = meta
		if _, loaded := p.pages[meta.PageIndex]; loaded {
			p.pages[meta.PageIndex] = RawPage{Metadata: meta, Data: data}
		}
	}
	return nil
}

func (p *RawPagePipeline) scanNextPage(ctx context.Context) ([]byte, PageMetadata, error) {
	pageIndex := p.scanned.nextPageIndex
	start := int64(pageIndex * p.pageSize)
	end := start + int64(p.pageSize)
	if end > p.fileSize {
		end = p.fileSize
	}
	data, err := p.source.ReadAt(ctx, start, int(end-start))
	if err != nil {
		return nil, PageMetadata{}, fmt.Errorf("read raw page %d from %q: %w", pageIndex, p.source.Path(), err)
	}

	meta := PageMetadata{
		PageIndex: pageIndex,
		ByteStart: start,
		ByteEnd:   end,
	}
	firstLine := 0
	lineCount := 0
	if p.scanned.pageStartsLine && start < p.fileSize {
		firstLine = p.scanned.nextLine
		lineCount++
		p.scanned.nextLine++
	}
	for i, b := range data {
		globalOffset := start + int64(i)
		if b == '\n' && globalOffset+1 < p.fileSize {
			nextLineStart := globalOffset + 1
			if nextLineStart >= start && nextLineStart < end {
				if lineCount == 0 {
					firstLine = p.scanned.nextLine
				}
				lineCount++
				p.scanned.nextLine++
			}
		}
	}
	meta.FirstRawLineNumber = firstLine
	meta.RawLineCount = lineCount

	p.scanned.nextPageIndex++
	p.scanned.pageStartsLine = len(data) > 0 && data[len(data)-1] == '\n' && end < p.fileSize
	return data, meta, nil
}

func (p *RawPagePipeline) windowForPage(currentPage int) []int {
	if p.totalPages == 1 {
		return []int{0}
	}
	start := currentPage - 1
	if start < 0 {
		start = 0
	}
	end := currentPage + 1
	if end >= p.totalPages {
		end = p.totalPages - 1
	}
	if end-start+1 < 3 && start > 0 {
		start--
	}
	if end-start+1 < 3 && end+1 < p.totalPages {
		end++
	}

	window := make([]int, 0, end-start+1)
	for index := start; index <= end; index++ {
		window = append(window, index)
	}
	return window
}

func (p *RawPagePipeline) loadOnlyWindow(ctx context.Context, window []int) error {
	keep := map[int]struct{}{}
	for _, pageIndex := range window {
		keep[pageIndex] = struct{}{}
	}
	for pageIndex := range p.pages {
		if _, ok := keep[pageIndex]; !ok {
			delete(p.pages, pageIndex)
		}
	}
	for _, pageIndex := range window {
		if _, ok := p.pages[pageIndex]; ok {
			continue
		}
		page, err := p.loadPage(ctx, pageIndex)
		if err != nil {
			return err
		}
		p.pages[pageIndex] = page
	}
	return nil
}

func (p *RawPagePipeline) loadPage(ctx context.Context, pageIndex int) (RawPage, error) {
	if err := p.ensureMetadataThrough(ctx, pageIndex); err != nil {
		return RawPage{}, err
	}
	meta := p.metadata[pageIndex]
	data, err := p.source.ReadAt(ctx, meta.ByteStart, int(meta.ByteEnd-meta.ByteStart))
	if err != nil {
		return RawPage{}, fmt.Errorf("read raw page %d from %q: %w", pageIndex, p.source.Path(), err)
	}
	return RawPage{Metadata: meta, Data: data}, nil
}
