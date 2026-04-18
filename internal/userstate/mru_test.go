package userstate

import (
	"reflect"
	"testing"
)

func TestPushMRU_givenEmptyEntry_whenPush_thenUnchanged(t *testing.T) {
	// Given
	list := []string{"a", "b"}
	// When
	got := PushMRU(list, "", 10)
	// Then
	if !reflect.DeepEqual(got, list) {
		t.Fatalf("expected unchanged, got %#v", got)
	}
}

func TestPushMRU_givenNewEntry_whenPush_thenPrepends(t *testing.T) {
	// Given
	list := []string{"old"}
	// When
	got := PushMRU(list, "new", 10)
	// Then
	want := []string{"new", "old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestPushMRU_givenDuplicate_whenPush_thenMovesToFront(t *testing.T) {
	// Given
	list := []string{"a", "b", "c"}
	// When
	got := PushMRU(list, "b", 10)
	// Then
	want := []string{"b", "a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestPushMRU_givenMax_whenPush_thenTrimsTail(t *testing.T) {
	// Given
	list := []string{"1", "2", "3"}
	// When
	got := PushMRU(list, "0", 3)
	// Then
	if len(got) != 3 || got[0] != "0" {
		t.Fatalf("got %#v", got)
	}
}
