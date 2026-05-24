package usecase

import (
	"context"
	"reflect"
	"testing"
)

func TestRawPageCacheKeepsOnlyPreviousCurrentNextPages(t *testing.T) {
	pipeline := newTestRawPagePipeline(t, "aa\nbb\ncc\ndd\nee\nff\n", 3)

	if err := pipeline.LoadPageWindow(context.Background(), 3); err != nil {
		t.Fatalf("load page window: %v", err)
	}

	if got, want := pipeline.LoadedPageIndexes(), []int{2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded pages = %#v, want %#v", got, want)
	}
}

func TestRawPageMetadataAssignsBoundarySpanningLineToEarlierPage(t *testing.T) {
	pipeline := newTestRawPagePipeline(t, "abcd\nef\n", 3)

	if err := pipeline.LoadPageWindow(context.Background(), 1); err != nil {
		t.Fatalf("load page window: %v", err)
	}

	page0, ok := pipeline.PageMetadata(0)
	if !ok {
		t.Fatal("page 0 metadata was not retained")
	}
	if page0.FirstRawLineNumber != 1 || page0.RawLineCount != 1 {
		t.Fatalf("page 0 metadata = %#v, want first line 1 count 1", page0)
	}

	page1, ok := pipeline.PageMetadata(1)
	if !ok {
		t.Fatal("page 1 metadata was not retained")
	}
	if page1.FirstRawLineNumber != 2 || page1.RawLineCount != 1 {
		t.Fatalf("page 1 metadata = %#v, want first line 2 count 1", page1)
	}
}

func TestRawPageMetadataSurvivesPageEviction(t *testing.T) {
	pipeline := newTestRawPagePipeline(t, "aa\nbb\ncc\ndd\nee\nff\n", 3)

	if err := pipeline.LoadPageWindow(context.Background(), 0); err != nil {
		t.Fatalf("load first window: %v", err)
	}
	if err := pipeline.LoadPageWindow(context.Background(), 4); err != nil {
		t.Fatalf("load later window: %v", err)
	}

	if containsInt(pipeline.LoadedPageIndexes(), 0) {
		t.Fatalf("page 0 still loaded after eviction: %#v", pipeline.LoadedPageIndexes())
	}
	page0, ok := pipeline.PageMetadata(0)
	if !ok {
		t.Fatal("page 0 metadata was not retained after eviction")
	}
	if page0.FirstRawLineNumber != 1 || page0.RawLineCount != 1 {
		t.Fatalf("page 0 metadata = %#v, want first line 1 count 1", page0)
	}
}

func TestRawPageMetadataUsesZeroSentinelForPagesWithoutOwnedLines(t *testing.T) {
	pipeline := newTestRawPagePipeline(t, "abcdef\n", 3)

	if err := pipeline.LoadPageWindow(context.Background(), 1); err != nil {
		t.Fatalf("load page window: %v", err)
	}

	page1, ok := pipeline.PageMetadata(1)
	if !ok {
		t.Fatal("page 1 metadata was not retained")
	}
	if page1.FirstRawLineNumber != 0 || page1.RawLineCount != 0 {
		t.Fatalf("page 1 metadata = %#v, want zero sentinel and count 0", page1)
	}
}

func TestRawPageRandomAccessLoadsWindowAroundTargetRawLine(t *testing.T) {
	pipeline := newTestRawPagePipeline(t, "aa\nbb\ncc\ndd\nee\nff\n", 3)

	if err := pipeline.LoadPageWindowForLine(context.Background(), 5); err != nil {
		t.Fatalf("load page window for line: %v", err)
	}

	if got, want := pipeline.LoadedPageIndexes(), []int{3, 4, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded pages = %#v, want target line window %#v", got, want)
	}
	if page, ok := pipeline.PageMetadata(4); !ok || page.FirstRawLineNumber != 5 || page.RawLineCount != 1 {
		t.Fatalf("target page metadata = %#v, %v; want line 5 on page 4", page, ok)
	}
}

func newTestRawPagePipeline(t *testing.T, content string, pageSize int) *RawPagePipeline {
	t.Helper()

	pipeline, err := NewRawPagePipeline(memoryRawPageSource{data: []byte(content)}, RawPageOptions{PageSize: pageSize})
	if err != nil {
		t.Fatalf("new raw page pipeline: %v", err)
	}
	return pipeline
}

type memoryRawPageSource struct {
	data []byte
}

func (s memoryRawPageSource) Path() string {
	return "memory.log"
}

func (s memoryRawPageSource) Size(context.Context) (int64, error) {
	return int64(len(s.data)), nil
}

func (s memoryRawPageSource) ReadAt(_ context.Context, offset int64, size int) ([]byte, error) {
	if offset >= int64(len(s.data)) {
		return []byte{}, nil
	}
	end := offset + int64(size)
	if end > int64(len(s.data)) {
		end = int64(len(s.data))
	}
	return append([]byte(nil), s.data[offset:end]...), nil
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
