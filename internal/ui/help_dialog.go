package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpSection struct {
	title string
	rows  [][2]string // key label, description
}

func filterSyntaxHelpSections() []helpSection {
	return []helpSection{
		{
			title: "이 대화창",
			rows: [][2]string{
				{"F1, Esc", "닫기 (필터 입력 유지)"},
				{"q, Ctrl+C", "종료"},
			},
		},
		{
			title: "결합 규칙 (§7)",
			rows: [][2]string{
				{"공백 구분", "plain·서로 다른 tag 키 → 모두 AND"},
				{"같은 tag 키", "+값 여러 개 → 그 축에서 OR"},
				{"토큰 | 토큰", "가지 나눔: (가지1) OR (가지2); 공백으로 | 분리"},
				{"\"…\"", "공백·특수 포함 한 토큰; tag 값 공백 시 따옴표"},
				{"tag: + 다음", "값 병합은 다음 토큰이 | 이면 하지 않음(OR 유지)"},
			},
		},
		{
			title: "입력 예 (요약)",
			rows: [][2]string{
				{"a b", "줄에 a AND b 부분 문자열"},
				{"a | b", "a 포함 이거나 b 포함"},
				{"level:E level:W", "레벨 ERROR 또는 WARN"},
				{"level:E timeout", "레벨 E AND 줄에 timeout"},
				{"svc:p x | svc:p y", "공통 tag는 가지마다 반복"},
			},
		},
		{
			title: "필터 입력 키",
			rows: [][2]string{
				{"Enter", "파싱·적용·목록 화면"},
				{"Esc", "초안 취소·목록 화면 (적용 필터 유지)"},
				{"↓", "필터 히스토리 (닫힌 상태)"},
				{"← →", "초안 캐럿"},
				{"PgUp 등", "§6.2·6.3 — 목록만 (초안에 안 들어감)"},
			},
		},
	}
}

func helpDialogSections() []helpSection {
	return []helpSection{
		{
			title: "이 대화창",
			rows: [][2]string{
				{"F1, Esc", "닫기"},
				{"q, Ctrl+C", "종료"},
			},
		},
		{
			title: "공통",
			rows: [][2]string{
				{"F1 · ?", "도움말 열기/닫기 (? 는 로그 목록에서만, F1 미수신 시)"},
				{"q, Ctrl+C", "종료"},
				{"Ctrl+W", "줄 바꿈(wrap) on/off"},
				{"Enter, :", "필터 입력 모드"},
				{"/", "하이라이트 검색 입력 시작"},
			},
		},
		{
			title: "로그 목록 화면",
			rows: [][2]string{
				{"↑ ↓", "한 줄 위/아래"},
				{"h j k l", "vim 방향: ← ↓ ↑ → 와 동일 (이 화면에서만; 필터·검색 입력 중에는 글자로 입력)"},
				{"PgUp / PgDn · Ctrl+B / Ctrl+F", "페이지 이동"},
				{"Home / End · G", "맨 위·맨 아래 (G = End, 로그 목록만)"},
				{"← →", "가로 스크롤 (wrap off)"},
				{"Shift+↑↓ · Shift+Home/End", "연속 구간(앵커)"},
				{"Space", "pick 토글 · 구간 있으면 구간→pick"},
				{"c", "구간∪pick(정렬·중복 없음) · 없으면 커서 한 줄"},
				{"m", "북마크 토글 (9칸 가득 시 1→…→9 순환 덮어쓰기, §6.7)"},
				{"1–9", "해당 슬롯 북마크 줄로 이동 (목록에 없으면 무동작)"},
				{"n / p", "다음·이전 매칭 (하이라이트 ON, 비순환; 로그 목록 화면)"},
				{"Ctrl+n / Ctrl+p", "동일 동작. 필터·검색 입력 중에는 평문 n·p는 초안 → 매칭 이동은 이 조합"},
				{"Esc", "구간+pick 해제 → compose → 검색 클리어 (§6.5·6.6)"},
			},
		},
		{
			title: "필터 입력",
			rows: [][2]string{
				{"F1", "필터 문법·결합 규칙 요약 (필터 입력 중만)"},
				{"Enter", "필터 적용·로그 목록 화면으로"},
				{"Esc", "로그 목록 화면 (초안 취소, 적용 필터 유지)"},
				{"↓", "필터 히스토리 목록 (Enter=초안에 반영, Esc=닫기)"},
				{"← →", "초안 캐럿 이동 (목록 가로 스크롤과 구분)"},
				{"토큰 | 토큰", "OR: 예) caching | current (공백 양쪽 |)"},
				{"그 외", "필터 식 편집; §6.2·6.3 키는 목록만 조작"},
			},
		},
		{
			title: "검색(highlight) 입력",
			rows: [][2]string{
				{"Enter", "적용 검색어로 확정·입력 종료"},
				{"/", "필터 입력 모드로 전환 (초안은 적용 검색어로 복귀); ':'는 검색 입력 중 일반 문자"},
				{"Esc", "목록 선택 있으면 해제만; 없으면 초안 취소"},
				{"↓", "하이라이트 히스토리 목록 (Enter=초안에 반영, Esc=닫기)"},
				{"← →", "초안 캐럿 이동 (목록 가로 스크롤과 구분)"},
				{"문자 · Space · Backspace", "검색 초안 편집"},
			},
		},
	}
}

func helpKeyColumnWidth(sections []helpSection) int {
	w := 0
	for _, sec := range sections {
		for _, row := range sec.rows {
			if n := len(row[0]); n > w {
				w = n
			}
		}
	}
	return w
}

// RenderHelpDialog builds the F1 modal body (bordered box) per PRD §5 mode-separated keymap.
func RenderHelpDialog(version string, maxOuterWidth int) string {
	maxInner := maxOuterWidth - 4
	if maxInner < 20 {
		maxInner = 20
	}
	ver := strings.TrimSpace(version)
	if ver == "" {
		ver = "unknown"
	}
	title := lipgloss.NewStyle().Bold(true).Render("logsee — help")
	verLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("114")).
		Render("Version · 버전  " + ver)

	sections := helpDialogSections()
	keyCol := helpKeyColumnWidth(sections)

	var body strings.Builder
	body.WriteString(title)
	body.WriteByte('\n')
	body.WriteString(verLine)

	for _, sec := range sections {
		body.WriteString("\n\n")
		body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("— " + sec.title + " —"))
		for _, row := range sec.rows {
			key := row[0]
			pad := strings.Repeat(" ", keyCol-len(key))
			body.WriteString("\n")
			body.WriteString(fmt.Sprintf("%s%s  %s", key, pad, row[1]))
		}
	}

	st := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		MaxWidth(maxInner)
	return st.Render(body.String())
}

// RenderFilterSyntaxHelpDialog is the F1 modal from filter-input focus: §7 summary + filter keys (PRD §5·§6.4).
func RenderFilterSyntaxHelpDialog(version string, maxOuterWidth int) string {
	maxInner := maxOuterWidth - 4
	if maxInner < 20 {
		maxInner = 20
	}
	ver := strings.TrimSpace(version)
	if ver == "" {
		ver = "unknown"
	}
	title := lipgloss.NewStyle().Bold(true).Render("logsee — 필터 문법")
	verLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("114")).
		Render("Version · 버전  " + ver)

	sections := filterSyntaxHelpSections()
	keyCol := helpKeyColumnWidth(sections)

	var body strings.Builder
	body.WriteString(title)
	body.WriteByte('\n')
	body.WriteString(verLine)

	for _, sec := range sections {
		body.WriteString("\n\n")
		body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("— " + sec.title + " —"))
		for _, row := range sec.rows {
			key := row[0]
			pad := strings.Repeat(" ", keyCol-len(key))
			body.WriteString("\n")
			body.WriteString(fmt.Sprintf("%s%s  %s", key, pad, row[1]))
		}
	}

	st := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		MaxWidth(maxInner)
	return st.Render(body.String())
}
