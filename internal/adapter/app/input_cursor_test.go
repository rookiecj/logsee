package app

import "testing"

func TestInsertTextAtCursor(t *testing.T) {
	t.Parallel()

	// Given
	text := "error"
	pos := 3

	// When
	gotText, gotPos := insertTextAtCursor(text, pos, "z")

	// Then
	if gotText != "errzor" {
		t.Fatalf("text = %q, want errzor", gotText)
	}
	if gotPos != 4 {
		t.Fatalf("cursor = %d, want 4", gotPos)
	}
}

func TestDeleteRuneBeforeCursor(t *testing.T) {
	t.Parallel()

	// Given
	text := "error"
	pos := 3

	// When
	gotText, gotPos := deleteRuneBeforeCursor(text, pos)

	// Then
	if gotText != "eror" {
		t.Fatalf("text = %q, want eror", gotText)
	}
	if gotPos != 2 {
		t.Fatalf("cursor = %d, want 2", gotPos)
	}
}

func TestMoveTextCursorLeftRight(t *testing.T) {
	t.Parallel()

	text := "ab"
	pos := 1

	if got := moveTextCursorLeft(text, pos); got != 0 {
		t.Fatalf("left from 1 = %d, want 0", got)
	}
	if got := moveTextCursorRight(text, pos); got != 2 {
		t.Fatalf("right from 1 = %d, want 2", got)
	}
}
