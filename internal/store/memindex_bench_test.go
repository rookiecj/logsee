package store

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/domain"
)

func BenchmarkMemIndex_Append(b *testing.B) {
	idx := NewMemIndex(lineSeq)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Append(domain.Line{Seq: int64(i + 1), Text: "x"})
	}
}

func BenchmarkMemIndex_Range1MSlice10K(b *testing.B) {
	idx := NewMemIndex(lineSeq)
	for i := 1; i <= 1_000_000; i++ {
		if err := idx.Append(domain.Line{Seq: int64(i), Text: "x"}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// window in the middle of the index, width 10k
		_ = idx.Range(495_000, 505_000)
	}
}

func BenchmarkMemIndex_Get(b *testing.B) {
	idx := NewMemIndex(lineSeq)
	for i := 1; i <= 1_000_000; i++ {
		if err := idx.Append(domain.Line{Seq: int64(i), Text: "x"}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = idx.Get(domain.Seq(i%1_000_000) + 1)
	}
}
