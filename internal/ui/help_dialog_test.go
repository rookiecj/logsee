package ui

import (
	"strings"
	"testing"
)

func TestHelpDialogSections_modeLabels(t *testing.T) {
	// Given: section definitions
	sections := helpDialogSections()
	// Then: five mode groups per PRD §5
	if len(sections) != 5 {
		t.Fatalf("Then: want 5 sections, got %d", len(sections))
	}
	want := []string{"이 대화창", "공통", "로그 목록 화면", "필터 입력", "검색(highlight) 입력"}
	for i, w := range want {
		if sections[i].title != w {
			t.Fatalf("section %d title: want %q got %q", i, w, sections[i].title)
		}
	}
}

func TestRenderHelpDialog_includesVersionAndSections(t *testing.T) {
	box := RenderHelpDialog("1.2.3-test", 80)
	if !strings.Contains(box, "1.2.3-test") {
		t.Fatalf("expected version in box, got:\n%s", box)
	}
	if !strings.Contains(box, "— 로그 목록 화면 —") {
		t.Fatalf("expected list section header, got:\n%s", box)
	}
}
