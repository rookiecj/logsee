package app

import "unicode/utf8"

func runeCount(text string) int {
	return utf8.RuneCountInString(text)
}

func clampTextCursor(text string, pos int) int {
	count := runeCount(text)
	if pos < 0 {
		return 0
	}
	if pos > count {
		return count
	}
	return pos
}

func moveTextCursorLeft(text string, pos int) int {
	pos = clampTextCursor(text, pos)
	if pos == 0 {
		return 0
	}
	return pos - 1
}

func moveTextCursorRight(text string, pos int) int {
	pos = clampTextCursor(text, pos)
	if pos >= runeCount(text) {
		return runeCount(text)
	}
	return pos + 1
}

func insertTextAtCursor(text string, pos int, insert string) (string, int) {
	if insert == "" {
		return text, clampTextCursor(text, pos)
	}
	pos = clampTextCursor(text, pos)
	prefix := string([]rune(text)[:pos])
	suffix := string([]rune(text)[pos:])
	merged := prefix + insert + suffix
	return merged, pos + runeCount(insert)
}

func deleteRuneBeforeCursor(text string, pos int) (string, int) {
	pos = clampTextCursor(text, pos)
	if pos == 0 {
		return text, 0
	}
	prefix := string([]rune(text)[:pos-1])
	suffix := string([]rune(text)[pos:])
	return prefix + suffix, pos - 1
}
