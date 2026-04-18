# 하이라이트 매칭 이동: `n` / `p` · `Ctrl+n` / `Ctrl+p` 구현 계획

(이전 문서: `highlight-nav-alt-np-plan.md` — `Alt+n`/`Alt+p`에서 변경.)

## 목적

- **적용 검색어**가 있을 때(하이라이트 모드) 다음/이전 매칭 줄로 커서를 옮기는 키는 **로그 목록**에서 **`n` / `p`**(vim 스타일 remap → `KeyCtrlN`/`KeyCtrlP`)와, **모든 포커스**에서 **`Ctrl+n` / `Ctrl+p`**(`tryBrowseKey`)이다. **필터·검색 초안** 편집 중에는 평문 **`n`/`p`는 초안 문자**이므로 매칭 이동은 **`Ctrl` 조합**만 쓴다(PRD §6.3·§8.4).

## 키 바인딩 (확정)

| 동작 | 키 | 비고 |
|------|-----|------|
| 다음 매칭 줄 | **`n`** (목록) / **`Ctrl+n`** | 목록: `remapVimNavKeysForLogList`; 공통: `tryBrowseKey` → `ctrl+n`; 커서보다 **아래**만 스캔, **순환 없음** |
| 이전 매칭 줄 | **`p`** (목록) / **`Ctrl+p`** | 목록: remap; 공통: `ctrl+p`; 커서보다 **위**만 스캔, **순환 없음** |

## 터미널·입력 주의

- **`Ctrl+n`/`Ctrl+p`**는 제어 문자로, 일부 터미널·SSH·로컬 line editor가 가로채면 bubbletea까지 오지 않을 수 있다. PRD 확정 표에 한 줄 주의사항으로 명시한다.

## 코드 변경 요약

| 위치 | 내용 |
|------|------|
| [`internal/ui/focus.go`](../../internal/ui/focus.go) `remapVimNavKeysForLogList` | **`n`→`KeyCtrlN`**, **`p`→`KeyCtrlP`** (로그 목록만; 필터·compose에서는 평문 유지). |
| [`internal/ui/model.go`](../../internal/ui/model.go) `tryBrowseKey` | `case "ctrl+n"`, `case "ctrl+p"`에서 `searchBuf != ""`일 때 `gotoNextSearchHit` / `gotoPrevSearchHit`(한 방향 스캔·**wrap 없음**). `searchBuf`가 비어도 **`true` 반환**해 **매칭 이동용 바인딩**이 검색·필터 초안에 문자로 섞이지 않도록 소비(무동작). |
| `handleKey` | compose·필터 초안 경로와의 우선순위는 PRD §6·§8. |
| `buildHighlightLine` (하이라이트 크롬 줄) | `HIGHLIGHT  │  > …` / `∅` 표시, `Ctrl+n/Ctrl+p` 등 힌트 문구. |
| [`cmd/logsee/main.go`](../../cmd/logsee/main.go) | `-help`에 `Ctrl+n`/`Ctrl+p` 안내. |
| PRD [`stdio-log-viewer-prd.md`](stdio-log-viewer-prd.md) | 확정 표·§2·§6·§8·§11·CLI 표. 목록 검색은 **`/`→compose→`Enter` 확정**·compose 중 **Esc** 취소. |

## 테스트 (given-when-then)

| 테스트 | 내용 |
|--------|------|
| `model_search_enter_test.go` | 확정 후 매칭 이동: `KeyCtrlN` 등으로 `ctrl+n` 시뮬레이션. |
| `model_ctrl_np_nav_test.go` | `searchBuf` 있을 때 목록에서 **`n`** remap으로 다음 매칭 이동; `ctrl+n` 경로 등. |

## 완료 조건

- PRD와 구현·둘째 줄 힌트가 일치한다.
- `go test ./...` 통과.
- 필터 입력 포커스에서도 `Ctrl+n`/`Ctrl+p`가 **매칭 이동**으로만 처리되고(하이라이트 모드일 때), **필터 초안에 문자로 들어가지 않는다**.
- 마지막/첫 매칭에서 **다음/이전 키가 목록 반대 끝으로 순환(wrap)하지 않는다**; 이동이 없을 때는 **`follow`를 불필요하게 끊지 않는다**.
