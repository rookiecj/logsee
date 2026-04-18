# stdin scrollback 디스크 fallback (후속 과제)

## 배경

[`stdin-fileprovider-unify-plan.md`](./stdin-fileprovider-unify-plan.md) 로 stdin 과 file 경로가 단일 `WindowProvider` 추상 뒤에서 동작하게 되었다. 하지만 stdin 용 `RingStreamProvider` 는 live ring 만 조회 가능 — ring 용량을 벗어난 과거 라인은 접근 불가. 이 제약은 현 구조가 갖는 유일한 비대칭이다.

logsee 는 이미 stdin 을 `-out` 파일로 append 중(per-line flush). 이 파일을 역으로 읽어 링 밖 scrollback 을 제공하는 것이 자연스러운 다음 단계.

## 목표

stdin 모드에서도 Home / Ctrl+n,p / PageUp / 북마크 점프 등이 **ring 용량 이상의 과거** 를 커버. 체감은 파일 모드와 동일.

## 기술 과제

### 1. 증분 인덱서

현재 `fileindex.BuildLineStartOffsets` 는 파일 전체를 한 번 스캔. stdin 은 append-only 스트림이므로 **증분 성장** 이 필요:

- `IncrementalOffsetIndex`: 마지막으로 인덱스한 byte offset 을 기억, 새 바이트만 추가 스캔.
- `applyIncomingLines` 가 write+flush 후 인덱서에게 "파일 크기가 Δ 늘어남" 알림.
- 주기적 (e.g. 100ms) 또는 PageUp 처럼 과거 조회 트리거 시 pull.

### 2. 회전 추적

`-out` 은 `outMaxBytes` 초과 시 `path` → `path.1` → `path.2` 로 회전 ([`storage/append.go`](../../internal/storage/append.go)). 리더가:

- rename/inode 변화 감지 (`tail -F` 방식: `os.Stat` 주기 폴링, `dev/ino` 비교).
- 회전 이후 과거는 `path.1..path.N` 에서 읽어야 함 — multi-file 인덱스 또는 회전 시 bailout ("older than last rotation not available").
- 단순화 옵션: 회전 발생하면 과거 scrollback 을 그 시점까지만 제공, 그 이상은 "회전으로 손실" 메시지.

### 3. Provider 합성

두 provider 를 묶는 `ChainedProvider` 또는 `RingStreamProvider` 내부에 on-demand disk fallback:

- `Fetch(first, last)` → live ring 범위는 ring 에서, 그 이전은 증분 인덱스 + `fileindex.ReadWindowRecords` 로 `-out` 파일에서.
- seq 1-based 일관성 유지 — stdin 경로의 ring `Seq` 는 `nextSeq` 에서 1 부터 증가하므로, `-out` 줄 번호와 일치.

### 4. EstimateBytes 복원

현재 `RingStreamProvider.EstimateBytes` 는 0. 디스크 fallback 도입 후엔 `-out` 부분은 offsets 차분으로 계산, ring 부분은 평균 추정 또는 0. 기존 disk-scan 100 MiB 가드 유지를 위해 필요.

### 5. lazy search 확장

[`model_input.go`](../../internal/ui/model_input.go) 의 Ctrl+n/p 는 현재 `filePartial` 가드로 막혀 stdin 에서 ring 내 검색만 지원. fallback 완료 후 가드를 `windowProvider != nil` 로 완화.

## 비목표 (별도 과제)

- `-out` 파일 자체의 변경 감지 (외부 프로세스가 수정) — logsee 가 단독 writer 인 현재 가정 유지.
- stdin 과 file 입력의 혼합 (파이프 + 인자 동시).

## 위험 및 미해결 질문

- **회전 간극**: `path.1` 생성 → `path` 재오픈 사이에 evict 된 seq 는 물리적으로 추적 불가. "회전 경계 이후만 scrollback" 을 수용할지 결정 필요.
- **동시성**: writer(UI goroutine) 와 reader(`tea.Cmd` goroutine) 가 같은 `-out` 파일을 건드림. OS 레벨 append write 는 원자적이지만 offset 인덱스 mutation 은 명시 mutex 필요.
- **테스트 인프라**: `tail -F` 류 증분 동작은 time-driven 이라 테스트가 flaky 해지기 쉬움. 파일을 직접 manual 확장하는 in-process 헬퍼로 검증.

## Success Criteria (제안)

- stdin 모드에서 `--out-max-bytes=0` (회전 비활성) + ring 용량 작음 조합으로 실행 시, PageUp / Home / Ctrl+p 가 ring 용량 이상의 과거 라인을 정상 표시.
- 회전 발생 시 status strip 에 "scrollback: rotated at seq N" 류 힌트 표시.
- 기존 file/stdin 경로 회귀 없음.

## 참고

- [`stdin-fileprovider-unify-plan.md`](./stdin-fileprovider-unify-plan.md) — Phase 1-5 이 본 과제의 기반.
- [`docs/architecture/log-read-pipeline.md`](../architecture/log-read-pipeline.md) — 현 파이프라인 다이어그램.
- [`seq-coord-pull-window-plan.md`](./seq-coord-pull-window-plan.md) — file 경로의 seq-primary 전환 (reader 가 참고할 원형).
