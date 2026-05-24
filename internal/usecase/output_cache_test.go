package usecase

import (
	"reflect"
	"strconv"
	"testing"
)

func TestOutputLogCacheDefaultTrimNeverExceedsTenThousandLines(t *testing.T) {
	cache, err := NewOutputLogCache(OutputLogCacheOptions{})
	if err != nil {
		t.Fatalf("new output cache: %v", err)
	}

	if err := cache.AppendAndTrim(
		outputRecords(1, "default-capacity-", DefaultOutputLogCacheCapacity+5),
		OutputCacheTrimOptions{CursorOutputIndex: DefaultOutputLogCacheCapacity + 4, ViewportHeight: 1},
	); err != nil {
		t.Fatalf("append and trim output cache: %v", err)
	}

	if got, want := cache.Len(), DefaultOutputLogCacheCapacity; got != want {
		t.Fatalf("cache length = %d, want %d", got, want)
	}
}

func TestOutputLogCacheTrimKeepsCapacity(t *testing.T) {
	cache, err := NewOutputLogCache(OutputLogCacheOptions{Capacity: 3})
	if err != nil {
		t.Fatalf("new output cache: %v", err)
	}

	cache.Append(outputRecords(101, "line-", 5))
	if err := cache.Trim(OutputCacheTrimOptions{CursorOutputIndex: 4, ViewportHeight: 2}); err != nil {
		t.Fatalf("trim output cache: %v", err)
	}

	if got, want := cache.Len(), 3; got != want {
		t.Fatalf("cache length = %d, want %d", got, want)
	}
	if got, want := cache.StartOutputIndex(), 2; got != want {
		t.Fatalf("start output index = %d, want %d", got, want)
	}
}

func TestOutputLogCacheTrimPreservesProtectedVisibleRange(t *testing.T) {
	cache, err := NewOutputLogCache(OutputLogCacheOptions{Capacity: 4})
	if err != nil {
		t.Fatalf("new output cache: %v", err)
	}

	cache.Append(outputRecords(201, "visible-", 6))
	if err := cache.Trim(OutputCacheTrimOptions{CursorOutputIndex: 4, ViewportHeight: 2}); err != nil {
		t.Fatalf("trim output cache: %v", err)
	}

	got := cache.Records()
	want := []OutputLogRecord{
		{RawLineNumber: 203, Text: "visible-3"},
		{RawLineNumber: 204, Text: "visible-4"},
		{RawLineNumber: 205, Text: "visible-5"},
		{RawLineNumber: 206, Text: "visible-6"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records after trim = %#v, want %#v", got, want)
	}
}

func TestOutputLogCacheTrimUsesCursorMinusViewportAsCutoff(t *testing.T) {
	cache, err := NewOutputLogCache(OutputLogCacheOptions{Capacity: 5})
	if err != nil {
		t.Fatalf("new output cache: %v", err)
	}

	cache.Append(outputRecords(301, "cutoff-", 8))
	if err := cache.Trim(OutputCacheTrimOptions{CursorOutputIndex: 6, ViewportHeight: 3}); err != nil {
		t.Fatalf("trim output cache: %v", err)
	}

	if got, want := cache.StartOutputIndex(), 3; got != want {
		t.Fatalf("start output index = %d, want protected cutoff %d", got, want)
	}
}

func TestOutputLogCacheRetainsRawLineNumbersAcrossAppendAndTrim(t *testing.T) {
	cache, err := NewOutputLogCache(OutputLogCacheOptions{Capacity: 3})
	if err != nil {
		t.Fatalf("new output cache: %v", err)
	}

	cache.Append([]OutputLogRecord{
		{RawLineNumber: 9, Text: "drop"},
		{RawLineNumber: 17, Text: "keep one"},
		{RawLineNumber: 42, Text: "keep two"},
		{RawLineNumber: 100, Text: "keep three"},
	})
	if err := cache.Trim(OutputCacheTrimOptions{CursorOutputIndex: 3, ViewportHeight: 2}); err != nil {
		t.Fatalf("trim output cache: %v", err)
	}

	got := cache.Records()
	want := []OutputLogRecord{
		{RawLineNumber: 17, Text: "keep one"},
		{RawLineNumber: 42, Text: "keep two"},
		{RawLineNumber: 100, Text: "keep three"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records after trim = %#v, want %#v", got, want)
	}
}

func outputRecords(firstRawLine int, textPrefix string, count int) []OutputLogRecord {
	records := make([]OutputLogRecord, count)
	for i := range records {
		records[i] = OutputLogRecord{
			RawLineNumber: firstRawLine + i,
			Text:          textPrefix + strconv.Itoa(i+1),
		}
	}
	return records
}
