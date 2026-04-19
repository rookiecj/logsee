package store

import (
	"errors"
	"sync"
	"testing"

	"git.inpt.fr/42dottools/log/internal/domain"
)

func lineSeq(l domain.Line) domain.Seq { return l.Seq }

func TestMemIndex_AppendInOrder(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	for i := 1; i <= 5; i++ {
		if err := idx.Append(domain.Line{Seq: int64(i), Text: "x"}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if idx.Len() != 5 {
		t.Errorf("Len = %d, want 5", idx.Len())
	}
	if idx.FirstSeq() != 1 {
		t.Errorf("FirstSeq = %d, want 1", idx.FirstSeq())
	}
	if idx.LastSeq() != 5 {
		t.Errorf("LastSeq = %d, want 5", idx.LastSeq())
	}
}

func TestMemIndex_AppendOutOfOrderErrors(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	if err := idx.Append(domain.Line{Seq: 5}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := idx.Append(domain.Line{Seq: 5}); !errors.Is(err, ErrOutOfOrder) {
		t.Errorf("duplicate Seq should be ErrOutOfOrder, got %v", err)
	}
	if err := idx.Append(domain.Line{Seq: 3}); !errors.Is(err, ErrOutOfOrder) {
		t.Errorf("lower Seq should be ErrOutOfOrder, got %v", err)
	}
	if err := idx.Append(domain.Line{Seq: 6}); err != nil {
		t.Errorf("higher Seq should succeed, got %v", err)
	}
}

func TestMemIndex_EmptyQueries(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	if idx.Len() != 0 {
		t.Errorf("Len on empty = %d", idx.Len())
	}
	if _, ok := idx.Get(1); ok {
		t.Error("Get on empty should return !ok")
	}
	if got := idx.Range(1, 10); got != nil {
		t.Errorf("Range on empty = %v, want nil", got)
	}
	if idx.FirstSeq() != 0 || idx.LastSeq() != 0 {
		t.Errorf("empty First/Last should both be 0")
	}
}

func TestMemIndex_GetFinds(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	for _, s := range []int64{2, 4, 6, 8, 10} {
		if err := idx.Append(domain.Line{Seq: s, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok := idx.Get(6); !ok || got.Seq != 6 {
		t.Errorf("Get(6) = %v/%v, want seq=6/true", got, ok)
	}
	if _, ok := idx.Get(5); ok {
		t.Error("Get(5) should be missing (only even seqs)")
	}
	if _, ok := idx.Get(11); ok {
		t.Error("Get(11) should be missing (beyond last)")
	}
	if _, ok := idx.Get(0); ok {
		t.Error("Get(0) should be missing (before first)")
	}
}

func TestMemIndex_RangeBounds(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	for _, s := range []int64{2, 4, 6, 8, 10} {
		if err := idx.Append(domain.Line{Seq: s, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		from, to domain.Seq
		wantSeqs []int64
	}{
		{1, 10, []int64{2, 4, 6, 8, 10}},
		{4, 8, []int64{4, 6, 8}},
		{5, 7, []int64{6}},
		{1, 1, nil},
		{11, 20, nil},
		{8, 2, nil}, // inverted
		{2, 2, []int64{2}},
	}
	for _, tc := range cases {
		got := idx.Range(tc.from, tc.to)
		if len(got) != len(tc.wantSeqs) {
			t.Errorf("Range(%d,%d) len = %d, want %d", tc.from, tc.to, len(got), len(tc.wantSeqs))
			continue
		}
		for i, want := range tc.wantSeqs {
			if got[i].Seq != want {
				t.Errorf("Range(%d,%d)[%d].Seq = %d, want %d", tc.from, tc.to, i, got[i].Seq, want)
			}
		}
	}
}

func TestMemIndex_RangeReturnsCopy(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	for i := 1; i <= 3; i++ {
		if err := idx.Append(domain.Line{Seq: int64(i), Text: "orig"}); err != nil {
			t.Fatal(err)
		}
	}
	out := idx.Range(1, 3)
	out[0].Text = "mutated"
	got, _ := idx.Get(1)
	if got.Text != "orig" {
		t.Error("Range result should be a copy; mutation leaked back into index")
	}
}

func TestMemIndex_ConcurrentReads(t *testing.T) {
	idx := NewMemIndex(lineSeq)
	const N = 10_000
	for i := 1; i <= N; i++ {
		if err := idx.Append(domain.Line{Seq: int64(i), Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(off int) {
			defer wg.Done()
			for i := 1; i <= 1000; i++ {
				s := domain.Seq((i*31 + off) % N)
				if s == 0 {
					s = 1
				}
				if _, ok := idx.Get(s); !ok {
					t.Errorf("Get(%d) missed during concurrent read", s)
					return
				}
			}
			_ = idx.Range(domain.Seq(off*10), domain.Seq(off*10+500))
		}(g)
	}
	wg.Wait()
}

func TestStore_NewHasEmptyIndexes(t *testing.T) {
	s := New()
	if s.Lines == nil || s.Records == nil {
		t.Fatal("Store.New should allocate all indexes")
	}
	if s.Lines.Len() != 0 || s.Records.Len() != 0 {
		t.Errorf("new Store should be empty, got Lines=%d Records=%d", s.Lines.Len(), s.Records.Len())
	}
	if err := s.Lines.Append(domain.Line{Seq: 1, Text: "hi"}); err != nil {
		t.Fatalf("Lines append: %v", err)
	}
	if err := s.Records.Append(domain.Record{Seq: 1, Level: domain.LevelInfo}); err != nil {
		t.Fatalf("Records append: %v", err)
	}
	if s.Lines.Len() != 1 || s.Records.Len() != 1 {
		t.Errorf("after 1 append each, got Lines=%d Records=%d", s.Lines.Len(), s.Records.Len())
	}
}
