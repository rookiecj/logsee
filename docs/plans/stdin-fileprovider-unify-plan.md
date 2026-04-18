# Stdin/File 입력 경로 통일 — tee 모델 (Ring + WindowProvider 단일 스토어)

## 배경

logsee 의 입력 경로가 비대칭이라 UI 에 분기가 많다.

- **파일 경로** (`filePartial=true`): `internal/fileindex` 가 byte offset 테이블을 build → `FileIndexReadyMsg`/`FilePartialBootstrapMsg` → `FileSliceProvider` (`internal/ui/window_provider.go`, `WindowProvider` 구현) → UI 는 `cursorSeq`/`viewTopSeq` 기반 seq-primary 모델로 sliding window 로드.
- **stdin 경로** (`filePartial=false`): `loginput.ScanLines` → `LineBatchMsg`/`LineMsg` → `applyIncomingLines` (`internal/ui/model_update.go:331`) → `LineAppender.WriteLine+Flush` (per-line flush 이미 적용) + `ring.Push`. 스크롤은 ring idx 기반. `follow` 태일링.

기존 자산: `WindowProvider` 인터페이스 와 `Ring.ReplaceRecords` 는 이미 존재. seq-primary 전환은 file 경로에 한정 적용.

## 원안의 대안 — 기각 사유

사용자 원안은 "stdin → 파일 저장 → 파일 경로로 재독" 파이프라인. 파일을 중간 큐로 쓰면 세 가지가 발생:

1. per-line flush + read 왕복 → 실시간성 하락.
2. `-out` 회전 (`outMaxBytes` 기본 10 MiB) 이 reader 와 충돌. inode/rotate 추적 필요.
3. `BuildLineStartOffsets` 는 1 회 빌드. tail-style 증분 인덱서 부재.

→ **파일을 중간 큐로 쓰지 않는 "tee 모델"** 로 우회: 두 경로가 같은 추상(Ring + WindowProvider)을 채우게 한다.

## 목표

- stdin 전용 `RingStreamProvider` 도입. `m.windowProvider` 를 stdin 에서도 항상 세팅.
- `LineAppender` 는 durability 용도로 유지 (통일과 별개).
- UI 분기를 가능한 곳에서 `filePartial` → `windowProvider != nil` 로 점진 교체.
- 렌더/입력 체감 불변.

**스코프 밖** (후속 과제):

- 링 용량 초과 과거 라인을 `-out` 파일에서 다시 읽는 디스크 fallback.
- 증분 인덱서, 회전 추적.
- stdin 경로의 lazy search (Ctrl+n/p) 를 ring 밖으로 확장.

## 가정 & 제약

- 기존 UI 테스트는 ring 을 직접 채우고 `windowProvider` 미설정 — `windowProviderOrFallback` 의 `fileOffsets` fallback 경로는 유지.
- `tea.Cmd` 클로저가 goroutine 에서 provider 를 호출 — `RingStreamProvider.Fetch` 는 `sync.Mutex` snapshot 으로 race 방지.
- `stdinClosed` 는 `maybeResolveAutoFormat` 의 유일한 트리거. 세팅 경로는 건드리지 않는다.

## 아키텍처 변경

- `internal/ui/window_provider.go` — `RingStreamProvider` 신규.
- `internal/ui/model.go` — `SetWindowProvider` setter. 필드 `streamTotalLines` (누적 수신 카운터).
- `internal/ui/model_update.go` — `applyIncomingLines` 에서 provider counter 증가, phase 3 에서 seq 앵커 동기화.
- `cmd/logsee/main.go` — stdin 분기 provider 주입.
- `internal/ui/file_partial.go`, `model_view_chrome.go` — `statusLineTotal` 일반화 (Phase 2).

## Phases

### Phase 1 — `RingStreamProvider` 도입 (UI 동작 불변)

- `internal/ui/window_provider.go`
  - `RingStreamProvider` 타입: `buf *buffer.Ring`, `mu sync.Mutex`, `totalRecv int64`.
  - `Fetch(first, last)`: ring 스캔, seq 범위 포함 record 복사 반환.
  - `TotalLines()`: `atomic.LoadInt64(&totalRecv)`.
  - `FileSize()`: 0.
  - `EstimateBytes(...)`: 0.
  - `NoteReceived(n int)`: stdin 측에서 누적 카운터 증가 훅.
- `internal/ui/model.go` — `SetWindowProvider(p WindowProvider)` setter.
- `internal/ui/model_update.go:applyIncomingLines` — ring.Push 직후 provider 에 수신 알림 (타입 단언으로 `RingStreamProvider` 만 호출).
- `cmd/logsee/main.go` — stdin 분기 provider 생성/주입.
- 단위 테스트 4 건:
  - `TestRingStreamProvider_FetchWithinRing`
  - `TestRingStreamProvider_FetchAfterEviction`
  - `TestRingStreamProvider_TotalLinesMonotonic`
  - `TestRingStreamProvider_ConcurrentPushFetch` (-race)

예상 diff ~270L, 리스크 **Low**.

### Phase 2 — `statusLineTotal` 일반화

- provider 우선, nil 이면 `buf.Len()` fallback. `fileTotalLines` 직접 참조 제거.
- status 문자열 테스트 전수 sweep.
- 예상 diff ~30L, 리스크 **Low-Med**.

### Phase 3 — stdin 경로 seq 앵커 동기화 (렌더 불변)

**결과**: 프로덕션 코드 변경 불필요. `syncScrollToCursor` (model_scroll.go:283) 가 이미 모든 분기 끝에서 `syncSeqFromIdx` 를 호출하고, `applyIncomingLines` 가 그것을 경유하므로 불변성이 이미 성립해 있었다. Phase 3 는 **암묵적 계약을 회귀 테스트로 고정** 하는 것으로 마무리.

- `internal/ui/model_stdin_seq_anchor_test.go` 신규: 첫 배치 / 반복 배치 / evict / empty-fidx 4 케이스의 `cursorSeq == buf.At(fidx[cursorIdx]).Seq`, `viewTopSeq == buf.At(fidx[scrollTop]).Seq` 불변성 잠금.
- 실제 diff: 프로덕션 0L, 테스트 ~100L.

### Phase 4 — 분기 점진 교체

- `lazy search` / `Home/End disk expand` 는 file 전용 유지 (의도적).
- 잔존 `filePartial` 분기에 "왜 남는가" 주석.
- 예상 diff ~10L, 리스크 **Low**.

### Phase 5 — 문서/후속 과제

- `docs/architecture/log-read-pipeline.md` 업데이트.
- `docs/plans/scrollback-disk-fallback.md` 후속 과제 seed.

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Fetch(goroutine) vs Push(UI) race | provider 내 `sync.Mutex` snapshot |
| evict 후 `lines:N/M` 기대값 변화 | status 매칭 테스트 sweep; 기존 테스트는 provider 미주입 → fallback 유지 |
| `EstimateBytes=0` guardrail 무의미 | stdin lazy search 는 `filePartial` 가드로 이미 막힘 (현행 유지) |
| `stdinClosed` → `maybeResolveAutoFormat` 연결 | 세팅 경로 변경 금지; 명시 주석 |
| Phase 3 seq 앵커 정합 미묘 | idx-primary 유지, 값만 세팅. 단위 테스트로 불변성 고정 |

## Success Criteria

- [x] Phase 1 후: stdin 에서 `m.windowProvider != nil`. 기존 테스트 전부 pass. `-race` clean.
- [x] Phase 2 후: `statusLineTotal` 단일 경로. status 테스트 pass.
- [x] Phase 3 후: stdin Push 이후 `cursorSeq == buf.At(fidx[cursorIdx]).Seq` 불변 (기존 `syncScrollToCursor → syncSeqFromIdx` 체인으로 이미 성립 — 회귀 테스트로 잠금).
- [x] Phase 4 후: 잔존 `filePartial` 분기에 주석 (Ctrl+n/p lazy search, Home/End disk load, `applyIncomingLines` 가드).
- [x] Phase 5 후: `docs/architecture/log-read-pipeline.md` 에 tee 모델 반영, `docs/plans/scrollback-disk-fallback.md` seed.
- [ ] 수동: `cat big | logsee`, `logsee big`, `tail -f live | logsee` 세 케이스 follow/status 정상 (사용자 수동 확인 필요).

## Complexity

| Phase | Files | Diff | Risk |
|-------|-------|------|------|
| 1 | 4 | ~270 L | Low |
| 2 | 2 | ~30 L | Low-Med |
| 3 | 1 | ~130 L | Med |
| 4 | 2 | ~10 L | Low |
| 5 | 3 | ~200 L | None |
