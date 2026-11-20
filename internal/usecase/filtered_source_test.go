package usecase

import (
	"context"
	"logsee/internal/port"
	"strings"
	"testing"
)

func TestAppendFilterScansOnlyDelta(t *testing.T) {
	raw := &measuredSource{data: strings.Repeat("match a\nmiss b\n", 20000)}
	s, _ := NewLogSource(raw)
	f := NewFilteredSource(s)
	filter, _ := CompileFilter("match", FilterOptions{})
	req := FilterRequest{Snapshot: port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}, Capacity: 3}
	w, e := f.ScanFilter(context.Background(), req, filter)
	if e != nil || w.TotalMatches != 20000 {
		t.Fatalf("%+v %v", w, e)
	}
	raw.bytes = 0
	raw.data += "match tail\n"
	req.Snapshot.ByteLimit = int64(len(raw.data))
	req.StartOutputIndex = 19999
	w, e = f.ScanFilter(context.Background(), req, filter)
	if e != nil || w.TotalMatches != 20001 || len(w.Records) != 2 {
		t.Fatalf("%+v %v", w, e)
	}
	if raw.bytes > 6*DefaultRawPageSize {
		t.Fatalf("append rescanned prefix: %d bytes", raw.bytes)
	}
}
func TestBackwardFilteredNavigationUsesMetadata(t *testing.T) {
	raw := &measuredSource{data: strings.Repeat("match a\nmiss b\n", 100000)}
	s, _ := NewLogSource(raw)
	f := NewFilteredSource(s)
	filter, _ := CompileFilter("match", FilterOptions{})
	req := FilterRequest{Snapshot: port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}, Capacity: 3}
	if _, err := f.ScanFilter(context.Background(), req, filter); err != nil {
		t.Fatal(err)
	}
	raw.bytes = 0
	req.StartOutputIndex = 80000
	w, e := f.ScanFilter(context.Background(), req, filter)
	if e != nil || len(w.Records) != 3 || w.Records[0].RawLineNumber != 160001 {
		t.Fatalf("%+v %v", w, e)
	}
	if raw.bytes > 6*DefaultRawPageSize {
		t.Fatalf("navigation rescanned prefix: %d", raw.bytes)
	}
}
func TestAppendReusesUnaffectedOutputCache(t *testing.T) {
	raw := &measuredSource{data: strings.Repeat("match "+strings.Repeat("x", 120)+"\n", 20000)}
	s, _ := NewLogSource(raw)
	f := NewFilteredSource(s)
	filter, _ := CompileFilter("match", FilterOptions{})
	req := FilterRequest{Snapshot: port.LogSnapshot{Generation: 1, ByteLimit: int64(len(raw.data))}, Capacity: 10000}
	if _, err := f.ScanFilter(context.Background(), req, filter); err != nil {
		t.Fatal(err)
	}
	raw.bytes = 0
	raw.data += "match tail\n"
	req.Snapshot.ByteLimit = int64(len(raw.data))
	w, err := f.ScanFilter(context.Background(), req, filter)
	if err != nil || w.TotalMatches != 20001 || len(w.Records) != 10000 {
		t.Fatalf("count=%d rows=%d err=%v", w.TotalMatches, len(w.Records), err)
	}
	if raw.bytes > 6*DefaultRawPageSize {
		t.Fatalf("reread unchanged viewport: %d bytes", raw.bytes)
	}
}
