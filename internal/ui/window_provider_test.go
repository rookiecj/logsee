package ui

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
)

// fakeWindowProvider implements WindowProvider in-memory so command-level tests can exercise the
// nav/filter flow without touching disk (Phase 2: the interface is the seam).
type fakeWindowProvider struct {
	records    []domain.Record // seq i stored at records[i-1]
	sizeBytes  int64
	fetchCalls []fetchCall
}

type fetchCall struct {
	first, last int64
}

func newFakeWindowProvider(seqs []int64) *fakeWindowProvider {
	recs := make([]domain.Record, len(seqs))
	for i, s := range seqs {
		recs[i] = domain.Record{Seq: s, Text: "line-fake"}
	}
	return &fakeWindowProvider{records: recs, sizeBytes: int64(len(recs) * 32)}
}

func (f *fakeWindowProvider) Fetch(first, last int64) ([]domain.Record, error) {
	f.fetchCalls = append(f.fetchCalls, fetchCall{first: first, last: last})
	total := int64(len(f.records))
	if first < 1 || first > total {
		return nil, nil
	}
	if last > total {
		last = total
	}
	return append([]domain.Record(nil), f.records[first-1:last]...), nil
}

func (f *fakeWindowProvider) TotalLines() int64 {
	return int64(len(f.records))
}

func (f *fakeWindowProvider) FileSize() int64 {
	return f.sizeBytes
}

func (f *fakeWindowProvider) EstimateBytes(first, last int64) int64 {
	if first < 1 || last < first {
		return 0
	}
	if last > int64(len(f.records)) {
		last = int64(len(f.records))
	}
	return (last - first + 1) * 32
}

func TestWindowProvider_cmdLoadFileWindowAround_routesThroughProvider(t *testing.T) {
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.width, m.height = 80, 25
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	m.fileTotalLines = 1000
	vh := m.viewportH()

	seqs := make([]int64, 1000)
	for i := range seqs {
		seqs[i] = int64(i + 1)
	}
	fake := newFakeWindowProvider(seqs)
	m.windowProvider = fake

	cmd := m.cmdLoadFileWindowAround(500)
	if cmd == nil {
		t.Fatal("expected cmd from cmdLoadFileWindowAround")
	}
	msg := cmd()
	loaded, ok := msg.(FileWindowLoadedMsg)
	if !ok {
		t.Fatalf("expected FileWindowLoadedMsg, got %T", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected err: %v", loaded.Err)
	}
	if len(fake.fetchCalls) != 1 {
		t.Fatalf("want 1 Fetch call, got %d", len(fake.fetchCalls))
	}
	want := int64(500 - vh)
	if want < 1 {
		want = 1
	}
	if fake.fetchCalls[0].first != want {
		t.Fatalf("Fetch first: want %d, got %d", want, fake.fetchCalls[0].first)
	}
	if loaded.FirstLine != want {
		t.Fatalf("FirstLine: want %d, got %d", want, loaded.FirstLine)
	}
	if got := len(loaded.Records); got != 2*vh {
		t.Fatalf("record count: want %d (2*vh), got %d", 2*vh, got)
	}
	if m.cursorSeq != 500 {
		t.Fatalf("cursorSeq: want 500, got %d", m.cursorSeq)
	}
}

func TestWindowProvider_FileSliceProvider_EstimateBytes(t *testing.T) {
	// Offsets: line 1 starts at 0, line 2 at 100, line 3 at 250, line 4 at 400; fileSize=500.
	// EstimateBytes(2, 3) = offsets[3] - offsets[2-1] = 400 - 100 = 300.
	// EstimateBytes(3, 4) = fileSize - offsets[2] = 500 - 250 = 250 (last line extends to EOF).
	p := NewFileSliceProvider("/tmp/anything", []int64{0, 100, 250, 400}, 500)
	cases := []struct {
		first, last int64
		want        int64
	}{
		{1, 1, 100},
		{1, 4, 500},
		{2, 3, 300},
		{3, 4, 250},
		{4, 4, 100},
		// Degenerate.
		{0, 1, 0},
		{2, 1, 0},
	}
	for _, c := range cases {
		if got := p.EstimateBytes(c.first, c.last); got != c.want {
			t.Errorf("EstimateBytes(%d,%d) = %d, want %d", c.first, c.last, got, c.want)
		}
	}
}

func TestWindowProvider_nilGuard_preventsPanic(t *testing.T) {
	var p *FileSliceProvider
	if got := p.TotalLines(); got != 0 {
		t.Errorf("nil TotalLines: want 0, got %d", got)
	}
	if got := p.FileSize(); got != 0 {
		t.Errorf("nil FileSize: want 0, got %d", got)
	}
	if got := p.EstimateBytes(1, 10); got != 0 {
		t.Errorf("nil EstimateBytes: want 0, got %d", got)
	}
	if recs, err := p.Fetch(1, 10); err != nil || recs != nil {
		t.Errorf("nil Fetch: want (nil, nil), got (%v, %v)", recs, err)
	}
}

func TestWindowProvider_cmdFindFilterMatchForward_usesProvider(t *testing.T) {
	// Verifies filter-scan path also routes through the provider.
	r := buffer.NewRing(1000)
	m := NewModel(r, nil, false, false, "", "/tmp/x.log", "", nil, nil, nil)
	m.width, m.height = 80, 25
	m.filePartial = true
	m.filePath = "/tmp/x.log"
	// Prime a small "loaded window" in the ring.
	m.buf.ReplaceRecords([]domain.Record{
		{Seq: 100, Text: "match"},
		{Seq: 101, Text: "match"},
	})
	m.fileWinFirst = 100
	m.fileTotalLines = 1000

	// Filter must be non-empty for cmdFindFilterMatch* to proceed.
	p, err := filter.Parse("match")
	if err != nil {
		t.Fatalf("parse filter: %v", err)
	}
	m.prog = p
	m.appliedFilter = "match"

	fake := newFakeWindowProvider(make([]int64, 1000))
	// Populate fake with seqs 1..1000.
	for i := range fake.records {
		fake.records[i] = domain.Record{Seq: int64(i + 1), Text: "fake-match"}
	}
	m.windowProvider = fake

	cmd := m.cmdFindFilterMatchForwardFromWindowEnd()
	if cmd == nil {
		t.Fatal("expected cmd from cmdFindFilterMatchForwardFromWindowEnd")
	}
	msg := cmd()
	if _, ok := msg.(FilterScanResultMsg); !ok {
		t.Fatalf("expected FilterScanResultMsg, got %T", msg)
	}
	if len(fake.fetchCalls) != 1 {
		t.Fatalf("want 1 Fetch call, got %d", len(fake.fetchCalls))
	}
	// windowEnd forward: start = fileWinFirst + buf.Len() = 100 + 2 = 102.
	if fake.fetchCalls[0].first != 102 {
		t.Fatalf("Fetch first: want 102 (windowEnd+1), got %d", fake.fetchCalls[0].first)
	}
}
