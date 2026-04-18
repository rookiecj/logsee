# Seq-coord · Pull-driven window loading

## Why

파일 부분 로딩 모드에서 **연속 스크롤 중 커서가 화면 중간으로 튀는** 회귀가 반복 관찰됨. 근본 원인:

- **뷰 상태가 ring-local idx(`scrollTop`, `cursorIdx`)에 묶여 있음**. `Ring.ReplaceRecords`가 일어나면 같은 정수값이 다른 의미의 슬롯을 가리키게 됨.
- **`syncScrollToCursor`의 minimal-change 정책**이 버퍼 교체 후에도 stale `scrollTop`을 유지. 새 cursorIdx(≈vh, 창 중앙)가 우연히 visible 범위면 scrollTop이 갱신되지 않아 커서가 viewport 내 임의 row에 착지.

`stickyFileScrollPin`(file_partial.go:131-133)은 위 구조 위에 덮는 **보정 장치**일 뿐이며, 체이닝 로드(nav→expand, nav→filter top-up)·async race마다 새 분기가 필요해 완전 차단이 어렵다.

## 목표

**뷰 상태를 파일 절대 좌표(Seq)로 격상**시켜 ring 교체와 무관하게 커서 화면 위치를 결정론적으로 유지한다.

- `viewTopSeq`, `cursorSeq`가 뷰의 primary 상태.
- `scrollTop`, `cursorIdx`는 **렌더 시점에 seq로부터 유도**되는 파생 값.
- Ring은 "Seq 범위 캐시" 역할. `ReplaceRecords`는 뷰 상태에 영향을 주지 않는다.
- 최종적으로 `WindowProvider.Fetch(seqRange)` 인터페이스 뒤로 stdin/file 양 입력을 통일.

## 비목표

- 필터 의미 변경 없음(§8.0 layer 1 그대로).
- PRD §4.1 외 규약(100 MiB 한계, 부트스트랩 42줄, 2×vh 윈도우) 변경 없음.
- stdin follow·wrap·bookmark·history 외부 동작 변경 없음.

## 목표 아키텍처

### 뷰 상태 (primary)

| 필드 | 의미 |
|------|------|
| `cursorSeq int64` | 커서가 가리키는 파일 내 1-based 줄 번호. 필터가 있어도 "현재 목록에 속한" 줄. |
| `viewTopSeq int64` | 뷰포트 최상단(또는 첫 가시 필터 매치)의 파일 내 줄 번호. |
| `viewCursorRow int` | (선택) wrap off에서 `cursorSeq - viewTopSeq`를 fidx 공간으로 해석한 screen row(0..vh-1). 렌더 불변식 검증용. |

### 파생 값 (렌더 시점에만 계산)

- `cursorIdx = indexOfSeqInFidx(cursorSeq, fidx)` (없으면 clamp).
- `scrollTop = indexOfSeqInFidx(viewTopSeq, fidx)` (없으면 `cursorIdx - desiredRow`).
- wrap on: `scrollSegTop`은 seg 배열에서 `cursorSeq`에 해당하는 첫 seg를 찾아 계산.

### WindowProvider(파이프라인)

```
Render loop:
  want := [viewTopSeq .. viewTopSeq + vh - 1] (fidx 공간, 필요 시 앞뒤로 여유분)
  records, status := window.Fetch(want)
  if status == loading: show "… loading" placeholder for missing rows
  else: render records

Navigation (j/k/PgUp/PgDn/Home/End/Search/Bookmark):
  cursorSeq = nextSeq(cursorSeq, ...)
  if cursorSeq > viewTopSeq + vh - 1: viewTopSeq = cursorSeq - vh + 1
  if cursorSeq < viewTopSeq: viewTopSeq = cursorSeq
```

비동기 도착:
- `Fetch`는 캐시 히트 시 즉시 records, 미스 시 `loading` + 백그라운드 로드 트리거.
- `FileWindowLoadedMsg` 도착 시 ring을 갱신할 뿐 뷰 상태는 불변.

## 단계별 구현

### Phase 1: 뷰 상태의 Seq primary화 (현재 이 단계)

**목표**: 버퍼 교체가 커서 화면 row를 흔들지 못하도록 seq를 source of truth로 전환. 외부 API·렌더는 그대로.

변경:
1. `Model`에 `cursorSeq`, `viewTopSeq` 필드 추가. 초기값 0 = "미설정".
2. 헬퍼 추가 (`model_scroll.go`):
   - `captureViewAnchors(fidx)`: 현재 `cursorIdx`/`scrollTop` → `cursorSeq`/`viewTopSeq` 기록.
   - `restoreViewAnchors(fidx)`: `cursorSeq`/`viewTopSeq` → `cursorIdx`/`scrollTop` 재구성. 앵커 seq가 fidx에 없으면 (a) cursorSeq는 `pendingFocusSeq` fallback, (b) viewTopSeq는 `cursorIdx - desiredRow`로 유도.
3. 뷰 상태를 바꾸는 **모든 경로**에서 다음 중 하나를 호출:
   - `syncSeqFromIdx(fidx)`: idx 기반 로직이 cursorIdx/scrollTop을 쓴 뒤 seq에 반영.
   - `syncIdxFromSeq(fidx)`: seq를 먼저 바꾼 뒤 idx에 반영 (주로 버퍼 교체 후).
4. `applyFileWindowLoaded`:
   - 진입 시 `pendingFocusSeq`가 있으면 → `cursorSeq = pendingFocusSeq`.
   - 아니면 기존 `cursorSeq` 유지.
   - `restoreViewAnchors(fidx)`로 `cursorIdx`/`scrollTop` 재구성.
   - 기존 `syncScrollToCursor` + sticky pin + `pendingFocusPreferBottom` 블록 **제거**.
5. `maybeFileLoadAfterNavDown/Up`: 로드 전에 의도한 앵커를 seq로 직접 설정.
   - Down 경계: `cursorSeq = G + 1`, `viewTopSeq = G + 1 - (vh - 1)` → 로드 후 자연스럽게 cursor bottom row.
   - Up 경계: `cursorSeq = G - 1`, `viewTopSeq = G - 1` → cursor top row.
6. `stickyFileScrollPin` 관련 필드·함수·호출 **제거**.

**불변식**: 버퍼 교체 전후로 `(cursorSeq, viewTopSeq)`가 불변이면 화면 내 커서 row도 불변.

**호환성**: 기존 테스트의 `cursorIdx`/`scrollTop` 검증은 그대로 통과해야 함(derivation이 동일 결과를 냄). sticky 관련 테스트는 seq 기반 검증으로 교체.

**체크 포인트**:
- [ ] 모든 `m.cursorIdx = …` 호출 지점에 seq 동기화 있음.
- [ ] 모든 `m.scrollTop = …` 호출 지점에 seq 동기화 있음.
- [ ] `ReplaceRecords` 호출 지점에 `restoreViewAnchors` 있음.
- [ ] sticky pin 관련 코드 흔적 없음.

### Phase 2: WindowProvider 인터페이스 (구현됨)

파일 random-access I/O를 인터페이스 뒤로 분리해 Model이 구체 구현(`fileindex.ReadWindowRecords`)에 직접 의존하지 않게 한다.

```go
// internal/ui/window_provider.go
type WindowProvider interface {
    Fetch(firstSeq, lastSeq int64) ([]domain.Record, error)
    TotalLines() int64
    FileSize() int64
    EstimateBytes(firstSeq, lastSeq int64) int64
}
```

구현:
- **FileSliceProvider**: `fileindex.ReadWindowRecords` 래퍼. 생성 시 offsets를 복사해 in-flight goroutine과 Model 쪽 갱신 간 race 방지.
- Model 필드 `windowProvider WindowProvider`. `applyFileIndexReady`에서 주입.
- `windowProviderOrFallback()`이 테스트용 raw `fileOffsets` seed도 ephemeral provider로 래핑 — 기존 테스트 무수정.

교체된 call site (모두 `prov.Fetch`/`prov.EstimateBytes` 경유):
- `cmdLoadFileWindowAround` / `cmdLoadFileWindowStartingAt` / `cmdLoadFileWindowAroundTop` / `cmdLoadFileWindowAroundBottom`
- `cmdFindFilterMatchForwardFromWindowEnd` / `cmdFindFilterMatchBackwardFromWindowStart`
- `cmdScanSearchInFile` (100 MiB 한도 계산 포함)

테스트: `window_provider_test.go`가 `fakeWindowProvider`를 주입해 **디스크 파일 없이** cmd 사이클을 검증 — Phase 3의 `SeqMatcher` 단위 테스트 기반이 됨.

향후 확장(Phase 3 연계): Async cache + `Loading` 상태는 pull-driven 뷰 구조로 가는 길목에서 필요해질 때 추가. 현재는 동기 `Fetch + error`로 충분.

### Phase 3: SeqMatcher 헬퍼 (구현됨)

"다음 매치 seq 찾기"가 여러 곳에 inline 되어 있었다:
- `gotoNextSearchHit` / `gotoPrevSearchHit` — ring 내 검색 hit 탐색 (`n`/`p`·`Ctrl+n`/`Ctrl+p`).
- `FilterScanResultMsg` handler의 nav-advance 분기 — filter top-up 후 cursor를 N번째 매치로.
- 향후 `cmdScanSearchInFile`·`cmdFindFilterMatch*`도 같은 패턴을 쓸 것.

Phase 3은 공통 헬퍼로 추출해 "매치 판정 rule"과 "스캔 mechanism"을 직교화한다.

```go
// internal/ui/seq_match.go
type SeqPredicate func(rec domain.Record) bool

func (m *Model) filterPredicate() SeqPredicate
func (m *Model) searchPredicate() SeqPredicate
func (m *Model) nextMatchIdxInFidx(fidx []int, fromSeq int64, dir int, pred SeqPredicate) int
```

규약:
- `nextMatchIdxInFidx`는 **ring-local scan**. 윈도우 내 `fromSeq` 기준 `dir` 방향으로 `pred`를 만족하는 첫 fidx idx를 반환, 없으면 `-1`. on-disk fallback은 호출자가 결정.
- `pred == nil`이면 fidx가 이미 filter 투영된 상태를 가정하고 seq 비교만 한다 (filter nav-advance 용도).
- `filterPredicate()` / `searchPredicate()`는 goroutine-safe closure — 현재 Model 상태를 snapshot해서 비동기 disk scan에서도 안전.

교체된 call site:
- `gotoNextSearchHit` / `gotoPrevSearchHit`: inline loop → `nextMatchIdxInFidx(fidx, curSeq, ±1, searchPredicate())`.
- `FilterScanResultMsg` handler nav-advance: dup된 forward/backward 분기를 하나의 dir 파라미터 루프로 통합 + `nextMatchIdxInFidx(fidx, cur, dir, nil)`.

테스트 (`seq_match_test.go`):
- 정/역방향 strict 비교 (`> fromSeq` / `< fromSeq`).
- Predicate가 nil일 때 fidx 투영만 사용.
- Predicate가 있을 때 filter/search 판정이 정확한지 (case-sensitive 포함).
- 빈 fidx / 경계값 처리.

향후 Phase 4 연계: pull-driven 뷰의 async "disk 위 다음 매치" API를 이 헬퍼의 동기 형태와 같은 시그니처로 만들면 ring vs disk 스캔이 한 인터페이스 뒤에 숨는다.

### Phase 4: sticky pin·pendingFocus 제거 (완료)

Phase 1~3 동안 호환성을 위해 유지했던 레거시 보정 장치를 완전히 제거하고, **뷰 상태는 오직 `cursorSeq` / `viewTopSeq`만**이 표현하도록 정리.

**Phase 4a — stickyFileScrollPin 제거:**
- `Model.stickyFileScrollPin` 필드, `stickyScrollOff/Top/Bottom` 상수, `applyStickyFileScrollPin` 함수 전부 삭제.
- nav 핸들러(`maybeFileLoadAfterNavDown/Up`, `maybeFileLoadAfterPageUp/Down`, `cmdBookmarkJumpToSeq`)와 `tryBrowseKey`의 sticky set/clear 라인 6+ 개 삭제.
- `FilterScanResultMsg` 핸들러의 sticky 호출 제거 → nav-advance 성공 시 `msg.Direction`으로부터 top/bottom 의도를 추론해 `cursorSeq`/`viewTopSeq`를 갱신한 뒤 `syncIdxFromSeq(fidx, fallbackRow)` 호출.

**Phase 4b — pendingFocusSeq / pendingFocusPreferBottom 제거:**
- cmd 함수들이 seq 앵커를 **직접** 설정:
  - `AroundBottom`: `cursorSeq=seq`, `viewTopSeq=seq-(vh-1)` (bottom pin).
  - `AroundTop` / `StartingAt` / bookmark: `cursorSeq=seq`, `viewTopSeq=seq` (top pin).
  - `Around`: cursorSeq만 갱신, viewTopSeq는 호출자의 설정 유지.
- `applyFileWindowLoaded`에서 pending* 처리 블록 제거 → 순수 seq 앵커 기반. **`preferBottom`은 `viewTopSeq < cursorSeq`로 추론**(bottom pin의 signature).
- 필드·모든 test 참조 정리.

**획득한 것**:
- 뷰 상태 primary 1종류(seq 앵커). 파생 1종류(idx). 호환 중간층 0종류.
- cmd 함수의 의도가 "seq 앵커를 어떻게 설정하는가"로 자기 문서화.
- nav handler·FilterScan·SearchScan·bookmark 경로가 동일한 규약 사용.

**테스트**:
- `model_sticky_scroll_test.go` 전면 재작성 (sticky pin 검증 → seq 앵커 검증).
- `TestApplyFileWindowLoaded_bottomPinFallback_keepsCursorOffTop` 등으로 fallback 추론 규약 고정.
- 기존 seq 앵커 테스트(Phase 1~3) 모두 통과.

## PRD 반영

§4.1에 다음 원칙을 추가(이 작업 중간에 stdio-log-viewer-prd.md 편집):

> 파일 부분 로딩의 뷰 상태는 **파일 절대 좌표(Seq)**를 primary로 한다. `viewTopSeq`/`cursorSeq`는 ring 교체와 무관하게 유지되며, 화면의 `scrollTop`/`cursorIdx`는 렌더 시점에 이로부터 유도된다. Ring은 Seq 범위의 **캐시**로서 교체되어도 커서 화면 row가 바뀌지 않는다.

## 테스트 전략

Phase 1 신규 테스트(`internal/ui/model_seq_anchor_test.go`):
- 버퍼 교체 직후 커서 화면 row 불변 — Down 경계 시나리오.
- 버퍼 교체 직후 커서 화면 row 불변 — Up 경계 시나리오.
- viewTopSeq가 새 fidx에 없을 때 fallback이 "커서 screen row 보존"으로 수렴.
- 연속 스크롤(체이닝 로드 2회) 후에도 커서 row 불변.

기존 sticky pin 테스트(`model_sticky_scroll_test.go`):
- 일부는 seq 앵커 검증으로 교체(같은 의도, 새 표현).
- 일부는 Phase 1 마무리 시 제거.

## 롤아웃

1. Phase 1 PR: 필드 추가 + 헬퍼 + `applyFileWindowLoaded` 전환 + sticky pin 제거 + 기존 테스트 통과.
2. Phase 2 PR: `WindowProvider` 도입 + 호출지 교체.
3. Phase 3 PR: `SeqMatcher` 통합.
4. Phase 4 PR: 정리·문서 갱신.

각 Phase는 독립적으로 머지 가능해야 한다.
