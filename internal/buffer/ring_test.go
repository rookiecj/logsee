package buffer

import "testing"

func TestRing_DropsOldestAtCapacity(t *testing.T) {
	// Given: a ring of capacity 2
	// When: more than two lines are pushed
	// Then: only the newest two remain in order
	r := NewRing(2)
	r.Push("a")
	r.Push("b")
	r.Push("c")
	if r.Len() != 2 {
		t.Fatalf("len got %d", r.Len())
	}
	if got := r.At(0).Text; got != "b" {
		t.Fatalf("oldest text: %q", got)
	}
	if got := r.At(1).Text; got != "c" {
		t.Fatalf("newest text: %q", got)
	}
	if r.At(1).Seq <= r.At(0).Seq {
		t.Fatalf("seq not increasing: %d %d", r.At(0).Seq, r.At(1).Seq)
	}
}
