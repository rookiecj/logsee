package app

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"logsee/internal/usecase"
)

func TestTeaKeyToLoopInputMapsCtrlFAndCtrlBToPageNavigation(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyType
		want loopEvent
	}{
		{name: "ctrl-f", key: tea.KeyCtrlF, want: loopEventPageDown},
		{name: "ctrl-b", key: tea.KeyCtrlB, want: loopEventPageUp},
		{name: "space", key: tea.KeySpace, want: loopEventSpacePick},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := teaKeyToLoopInput(tea.KeyMsg(tea.Key{Type: tt.key}))
			if got.event != tt.want {
				t.Fatalf("teaKeyToLoopInput(%s) event = %v, want %v", tt.name, got.event, tt.want)
			}
		})
	}
}

func TestTeaKeyToLoopInputTreatsQuestionRuneAsText(t *testing.T) {
	got := teaKeyToLoopInput(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'?'}}))

	if got.event != loopEventText {
		t.Fatalf("question rune event = %v, want %v", got.event, loopEventText)
	}
	if got.text != "?" {
		t.Fatalf("question rune text = %q, want ?", got.text)
	}
}

func TestTeaKeyToLoopInputMapsShiftArrowsToRangeSelection(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyType
		want loopEvent
	}{
		{name: "shift-up", key: tea.KeyShiftUp, want: loopEventShiftUp},
		{name: "shift-down", key: tea.KeyShiftDown, want: loopEventShiftDown},
		{name: "left", key: tea.KeyLeft, want: loopEventHorizontalLeft},
		{name: "right", key: tea.KeyRight, want: loopEventHorizontalRight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := teaKeyToLoopInput(tea.KeyMsg(tea.Key{Type: tt.key}))
			if got.event != tt.want {
				t.Fatalf("teaKeyToLoopInput(%s) event = %v, want %v", tt.name, got.event, tt.want)
			}
		})
	}
}

func TestTeaLoopModelShowsFilterChromeWhileEditing(t *testing.T) {
	logPath := writeLoopLog(t, "error one\ninfo two\n")
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}
	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 120, 5, unboundedRecordLimit, "")
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}
	model := teaLoopModel{ctx: context.Background(), state: state}

	updated, _ := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{':'}}))
	model = updated.(teaLoopModel)
	if got := model.View(); !strings.Contains(got, "FILTER INPUT(':')  │  > _") {
		t.Fatalf("view after ':' = %q, want visible empty filter editing chrome", got)
	}

	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'e'}}))
	model = updated.(teaLoopModel)
	if got := model.View(); !strings.Contains(got, "FILTER INPUT(':')  │  > e_") {
		t.Fatalf("view after filter text = %q, want visible draft filter chrome", got)
	}

	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'r'}}))
	model = updated.(teaLoopModel)
	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'r'}}))
	model = updated.(teaLoopModel)
	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'o'}}))
	model = updated.(teaLoopModel)
	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'r'}}))
	model = updated.(teaLoopModel)
	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	model = updated.(teaLoopModel)
	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyLeft}))
	model = updated.(teaLoopModel)
	updated, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'z'}}))
	model = updated.(teaLoopModel)
	if got := model.View(); !strings.Contains(got, "FILTER INPUT(':')  │  > errz_or") {
		t.Fatalf("view after cursor move and insert = %q, want errz_or editing chrome", got)
	}
}

func TestTeaLoopModelClearsCopyMessageAfterExpiry(t *testing.T) {
	logPath := writeLoopLog(t, "copy me\n")
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}
	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 200, 5, unboundedRecordLimit, "")
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}
	state.clipboard = &fakeLoopClipboard{}
	model := teaLoopModel{ctx: context.Background(), state: state}

	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'c'}}))
	model = updated.(teaLoopModel)
	if cmd == nil {
		t.Fatal("copy input should schedule message expiry")
	}
	if got := model.View(); !strings.Contains(got, "1 line copied") {
		t.Fatalf("view after copy = %q, want copy message", got)
	}

	updated, _ = model.Update(runtimeMessageExpiredMsg{id: model.state.runtimeMessageID})
	model = updated.(teaLoopModel)
	if got := model.View(); strings.Contains(got, "1 line copied") {
		t.Fatalf("view after message expiry = %q, want copy message cleared", got)
	}
}

func TestTeaLoopModelClearsNoMatchMessageAfterExpiry(t *testing.T) {
	logPath := writeLoopLog(t, "timeout first\nidle\n")
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}
	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 200, 5, unboundedRecordLimit, "")
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}
	state.searchText = "timeout"
	model := teaLoopModel{ctx: context.Background(), state: state}

	updated, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'n'}}))
	model = updated.(teaLoopModel)
	if cmd == nil {
		t.Fatal("search boundary should schedule message expiry")
	}
	if got := model.View(); !strings.Contains(got, "no match") {
		t.Fatalf("view after search boundary = %q, want no match message", got)
	}

	updated, _ = model.Update(runtimeMessageExpiredMsg{id: model.state.runtimeMessageID})
	model = updated.(teaLoopModel)
	if got := model.View(); strings.Contains(got, "no match") {
		t.Fatalf("view after message expiry = %q, want no match message cleared", got)
	}
}

func TestTeaLoopModelFullHeightViewDoesNotEndWithNewline(t *testing.T) {
	logPath := writeLoopLog(t, "one\ntwo\n")
	session := usecase.InputSession{
		Mode:    usecase.InputModeFile,
		SOTPath: logPath,
	}
	state, err := newLoopState(context.Background(), session, logPath, usecase.LogTypePlain, 80, 5, unboundedRecordLimit, "")
	if err != nil {
		t.Fatalf("new loop state: %v", err)
	}
	model := teaLoopModel{ctx: context.Background(), state: state}

	got := model.View()
	for _, want := range []string{
		"FILTER INPUT(':')  │  ∅",
		"SEARCH INPUT('/')  │  ∅",
		"1",
		"2",
		"lines:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("full-height view %q does not contain %q", got, want)
		}
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("full-height view must not end with newline that can scroll the filter row: %q", got)
	}
}
