# logsee — Agent 정의

**logsee**는 Go로 만든 **stdio·파일 로그 TUI**다. Bubble Tea 기반 UI, 링 버퍼·파일 부분 로딩·필터·하이라이트 등은 [`docs/plans/stdio-log-viewer-prd.md`](docs/plans/stdio-log-viewer-prd.md)와 [`docs/architecture/`](docs/architecture/) 문서가 단일 기준이다.

에이전트는 이 저장소에서 **터미널 로그 뷰어의 동작·성능·호환성을 깨지 않는 변경**을 우선하고, 모호할 때는 PRD와 아키텍처 노트를 먼저 확인한다.

---

## 세션 시작 시

1. **`docs/`를 훑는다** — 최소한 PRD(`docs/plans/stdio-log-viewer-prd.md`), `docs/architecture/*.md`, 진행 중이면 해당 `docs/plans/*`, `docs/stories/*`를 읽고 맥락을 맞춘다.
2. **코드 위치 힌트**: 엔트리 [`cmd/logsee/main.go`](cmd/logsee/main.go), 도메인·버퍼 [`internal/buffer`](internal/buffer), [`internal/domain`](internal/domain), 파일/줄 입력 [`internal/loginput`](internal/loginput), TUI [`internal/ui`](internal/ui), 설정 [`internal/config`](internal/config).

---

## 빌드·검증·실행

- **명령은 Makefile 기준**으로 제안·실행한다: `make dep`, `make build`, `make test`, `make vet`, `make fmt`, `make run ARGS="..."`.
- 변경 후 **`make test`가 통과**해야 한다. 린트/포맷 수정만 할 때도 **빌드 실패·동작 변경·테스트 실패**가 없어야 한다.

---

## 코드 구조 규칙

- **단일 소스 파일은 500줄을 넘기지 않는다.** 넘으면 역할 단위로 파일을 나눈다.
- **`internal/ui`의 Bubble Tea `Model`**
  - 타입·생성자·핵심 상태는 [`internal/ui/model.go`](internal/ui/model.go)에 둔다.
  - 스크롤·선택·`Update`/입력·뷰·크롬·로그 렌더 등은 `model_*.go`로 나눈다.
  - 터미널 **셀 폭** 유틸은 [`internal/ui/display_cell.go`](internal/ui/display_cell.go)에 둔다.
- **실용적 클린 아키텍처**: 프로젝트 규모에 맞게 레이어를 과도하게 쪼개지 않고, `cmd` → `internal` 경계와 도메인·UI·입력 책임을 명확히 유지한다.

---

## 동작·품질 원칙

- **PRD와 불일치하는 “개선”**은 하지 않는다. 키맵·stdin vs 파일 모드·줄 번호 의미 등은 PRD를 따른다.
- **파일 모드**(부분 로딩·오프셋 테이블·슬라이딩 윈도우)와 **stdin 모드**의 차이를 바꿀 때는 [`docs/architecture/log-read-pipeline.md`](docs/architecture/log-read-pipeline.md)를 반드시 고려한다.
- **설정·CLI 우선순위**(`CLI > config > 기본값`)는 [`internal/config`](internal/config)와 README의 설명과 맞춘다.

---

## 테스트

- **테이블 드리븐 테스트가 아닌 경우**, 비즈니스 로직 테스트는 **Given / When / Then** 패턴으로 작성한다.
- 기능 완료 판단은 **유닛 테스트 통과**를 포함한다(프로젝트 정책과 story acceptance criteria가 있으면 그에 따른다).

---

## 기획·문서 워크플로 (요청 시)

큰 기능·에픽 단위 작업 시 저장소 관례에 맞춘다:

- 계획 초안: `docs/plans/`
- 아키텍처 결정: `docs/architecture/`
- 에픽·스토리: `docs/epics/`, `docs/stories/` (파일 네이밍 규칙: `epic-{no}.{name}.md`, `story-{epic}.{feature}-{name}.md`), 서로 링크로 연결

에이전트는 **사용자가 문서 작성을 요청하지 않은 한** 불필요한 마크다운 파일을 추가하지 않는다.

---

## Sub agent 역할 (위임·병렬 작업)

작업이 **여러 패키지 경계**를 넘거나, **읽기 전용 탐색**과 **빌드·테스트 실행**을 나눌 때 아래 역할로 쪼갠다. Cursor **Task** 도구를 쓸 때는 `explore`(코드/문서 읽기), `shell`(Make·git), `general-purpose`(구현)와 조합해 **한 번에 하나의 sub agent 범위**만 맡기면 맥락이 덜 섞인다.

| ID | 초점 | 주요 경로·산출물 | 함께 읽을 문서 |
|----|------|------------------|----------------|
| **ui-tui** | Bubble Tea `Model`, 키맵, 포커스, 뷰/크롬, 스크롤·선택·wrap·도움말·히스토리 오버레이, 터미널 셀 폭 | `internal/ui/model.go`, `model_*.go`, `display_cell.go`, `help_dialog.go`, `focus.go`, `bookmark.go`, `history_overlay.go`, `wrap.go`, `model_view_chrome.go`, 관련 `*_test.go` | PRD 키맵·화면, `docs/plans/screen-layout-filter-status.md` 등 UI 계획 |
| **log-pipeline** | 줄 스캔·파일 인덱스·링 버퍼·레코드 도메인, **파일 모드** 부분 로딩과 `Model` 연동 | `internal/loginput/`, `internal/fileindex/`, `internal/buffer/`, `internal/domain/`, `internal/ui/file_partial.go` | [`docs/architecture/log-read-pipeline.md`](docs/architecture/log-read-pipeline.md), PRD §stdin vs 파일 |
| **filter-highlight** | 필터 문법·파싱, 하이라이트/검색과 목록의 교차 | `internal/filter/`, `internal/ui/highlight.go`, `model` 내 필터·검색 분기(보통 `model_input.go`, `model_update.go`, 렌더 쪽) | PRD 필터·검색, 관련 `docs/plans/*highlight*` |
| **config-cli-state** | CLI 플래그, TOML 설정, MRU·상태 저장, stdin 시 append 출력 | `cmd/logsee/main.go`, `internal/config/`, `internal/userstate/`, `internal/storage/`, `internal/version/` | `config.example.toml`, README, PRD 플래그·저장 동작 |
| **docs-prd** | PRD·아키텍처·story와의 **정합성 검토**, 에픽/스토리 링크·수용 기준 문구(요청 시) | `docs/plans/`, `docs/architecture/`, `docs/stories/`, `docs/epics/` | PRD 단일 기준; 새 결정은 `docs/architecture/` 또는 플랜에 반영 |

**경계 메모**

- `internal/ui/file_partial.go`는 **log-pipeline**이 깨지면 파일 모드 전체가 깨지므로, 파일 윈도·인덱스·메시지 흐름 변경은 **log-pipeline** 우선, UI만의 배치·문구는 **ui-tui**와 조율한다.
- 하이라이트 **색·렌더**는 **ui-tui**, 토큰·매칭 규칙은 **filter-highlight**에 가깝다.
- **config-cli-state**가 바꾼 플래그/기본값은 README·`config.example.toml`·PRD 요약과 **한 세트**로 맞춘다.

**도구 매핑 (Task 사용 시)**

- 코드베이스만 빠르게 훑기 → `explore`, 읽기 전용.
- `make test` / `make build` / `go test` 패키지 지정 → `shell`.
- 한 sub agent ID에 해당하는 **구현·리팩터** → `general-purpose` + 위 표의 경로를 프롬프트에 명시.

---

## diff 철학

- 요청 범위 밖의 리팩터·스타일만 바꾸는 대규모 diff는 피한다.
- 한 커밋/한 PR에 **목적이 분명한 최소 변경**을 선호한다.
