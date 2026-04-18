package userstate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_givenMissingFile_whenLoad_thenEmptySnapshot(t *testing.T) {
	// Given
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	// When
	s, err := Load(path)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if s.LastFilter != "" || len(s.FilterHistory) != 0 {
		t.Fatalf("expected empty, got %+v", s)
	}
}

func TestSaveRoundTrip_givenSnapshot_whenSaveLoad_thenEqual(t *testing.T) {
	// Given
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	want := Snapshot{
		Version:          SnapshotVersion,
		LastFilter:       `level:ERROR foo`,
		LastHighlight:    `bar "x y"`,
		FilterHistory:    []string{"a", "b"},
		HighlightHistory: []string{"h1"},
	}
	// When
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != SnapshotVersion {
		t.Fatalf("version got %d", got.Version)
	}
	if !reflect.DeepEqual(got.FilterHistory, want.FilterHistory) {
		t.Fatalf("FilterHistory got %#v", got.FilterHistory)
	}
	if !reflect.DeepEqual(got.HighlightHistory, want.HighlightHistory) {
		t.Fatalf("HighlightHistory got %#v", got.HighlightHistory)
	}
	if got.LastFilter != want.LastFilter || got.LastHighlight != want.LastHighlight {
		t.Fatalf("last fields got filter=%q highlight=%q", got.LastFilter, got.LastHighlight)
	}
}

func TestLoad_givenInvalidJSON_whenLoad_thenError(t *testing.T) {
	// Given
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	// When
	_, err := Load(path)
	// Then
	if err == nil {
		t.Fatal("expected error")
	}
}
