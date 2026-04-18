package version

import (
	"strings"
	"testing"
)

func TestLine_nonEmpty(t *testing.T) {
	// Given: module linked in tests
	// When:
	s := Line()
	// Then:
	if s == "" {
		t.Fatal("expected non-empty version line")
	}
}

func TestLine_includesAppVersionWhenSet(t *testing.T) {
	// Given: AppVersion set as it would be at link time from VERSION + Makefile
	prev := AppVersion
	t.Cleanup(func() { AppVersion = prev })
	AppVersion = "9.9.9-test"

	// When:
	s := Line()

	// Then:
	if !strings.Contains(s, "9.9.9-test") {
		t.Fatalf("expected AppVersion in line, got %q", s)
	}
	if !strings.HasPrefix(s, "logsee ") {
		t.Fatalf("expected logsee prefix when AppVersion set, got %q", s)
	}
}
