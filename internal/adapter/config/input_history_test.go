package config

import (
	"path/filepath"
	"testing"

	"logsee/internal/usecase"
)

func TestLoadSaveInputHistory_RoundTripsSnapshot(t *testing.T) {
	// Given: a temp home dir and a snapshot with filter/search data
	home := t.TempDir()
	path := ResolveInputHistoryPath(home)
	snapshot := usecase.InputHistorySnapshot{
		Filter: usecase.InputChannelHistory{
			Last:    "error",
			History: []string{"error", "timeout"},
		},
		Search: usecase.InputChannelHistory{
			Last:    "db",
			History: []string{"db", "ready"},
		},
	}

	// When: saving then loading
	if err := SaveInputHistory(path, snapshot); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadInputHistory(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Then: values match
	if got.Filter.Last != "error" || len(got.Filter.History) != 2 {
		t.Fatalf("filter = %#v, want last error and two entries", got.Filter)
	}
	if got.Search.Last != "db" || len(got.Search.History) != 2 {
		t.Fatalf("search = %#v, want last db and two entries", got.Search)
	}
	if filepath.Base(path) != "input_history.json" {
		t.Fatalf("path base = %q, want input_history.json", filepath.Base(path))
	}
}

func TestLoadInputHistory_MissingFileReturnsEmpty(t *testing.T) {
	// Given: no history file
	path := filepath.Join(t.TempDir(), "missing.json")

	// When: loading
	got, err := LoadInputHistory(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Then: snapshot is empty
	if got.Filter.Last != "" || len(got.Filter.History) != 0 {
		t.Fatalf("filter = %#v, want empty", got.Filter)
	}
}
