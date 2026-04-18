package ui

import (
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/buffer"
)

func TestApplyIncomingLines_batch(t *testing.T) {
	// Given: a ring and a batch of lines
	// When: applyIncomingLines runs
	// Then: all lines are stored in order
	r := buffer.NewRing(100)
	m := NewModel(r, nil, false, false, "", "stdin", "", nil, nil, nil)
	m.applyIncomingLines([]string{"a", "b", "c"})
	if r.Len() != 3 {
		t.Fatalf("Len got %d", r.Len())
	}
	if r.At(0).Text != "a" || r.At(2).Text != "c" {
		t.Fatalf("order wrong: %#v %#v", r.At(0).Text, r.At(2).Text)
	}
}

func TestTruncateStatusPath(t *testing.T) {
	if got := truncateStatusPath("short", 20); got != "short" {
		t.Fatalf("%q", got)
	}
	long := strings.Repeat("x", 50)
	got := truncateStatusPath(long, 20)
	if len([]rune(got)) > 20 {
		t.Fatalf("too long: %d", len([]rune(got)))
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("expected ellipsis in %q", got)
	}
}
