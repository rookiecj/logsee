# Anomaly Detection · 구현 계획

`docs/architecture/anomaly-detection.md`의 설계를 단계별로 구현한다. 각 Phase는 **독립 릴리스 가능**해야 하며, 이전 Phase 없이 되돌릴 수 있어야 한다.

## Why

설계 문서에 요약. 핵심 요구:
1. AI 보조 분석(Android adb 이상탐지)이 가능해야 한다 — headless JSONL.
2. 신규 로직이 `Model`(101 edges)에 붙지 않도록 분석 레이어를 분리한다.
3. 새 규칙/파서/출력 추가 시 **한 인터페이스만** 만지도록 확장 지점을 단일화한다.

## 범위

in-scope:
- 도메인 타입·포트·파이프라인 골격
- Tier A 규칙(ANR/FATAL/tombstone/LMK/Watchdog 등) 분류기
- 블록 파서(native crash, Java FATAL, ANR)
- headless JSON export

out-of-scope (v2 이후):
- Drain 템플릿 마이닝
- 정상 baseline diff
- adb 직접 소스(현재는 파이프·파일만)
- bbolt 영속화 드라이버
- ML 기반 탐지

## 비목표

- 기존 TUI 키바인딩·필터 의미 변경 없음
- `Ring`/`WindowProvider` 외부 계약 변경 없음
- `--out` 디스크 포맷 변경 없음

## 선행 조건

- 현재 브랜치 main, `make publish-verify` green
- Go 1.22+ (iter.Seq 사용)

## 공통 작업 규칙

- **PRD·feedback 우선**: 각 Phase 시작 시 `docs/plans/stdio-log-viewer-prd.md` 관련 섹션 재확인
- **TDD**: 새 패키지는 테이블 테스트 우선, 구현 후 `go test ./... -cover` 80%+ 확인
- **커밋 단위**: 한 PR = 한 Phase 소작업. 각 커밋은 독립 빌드 가능
- **문서 동기화**: Phase 종료 시 `docs/architecture/anomaly-detection.md`의 오픈 이슈 갱신

---

## Phase 0 · 준비

**목표**: 신규 구조를 위한 빈 공간을 만든다. 기존 동작 변경 없음.

### 작업

1. `internal/domain/` 패키지 생성 (빈 doc.go만)
2. `internal/analysis/` 패키지 생성 (빈 doc.go만)
3. `internal/source/`, `internal/pipeline/`, `internal/store/` 마찬가지
4. `testdata/android/` 디렉터리 + 대표 샘플 3개 수집:
   - `anr_input_dispatch.log` (ActivityManager ANR)
   - `native_tombstone.log` (SIGSEGV backtrace)
   - `java_fatal_system_server.log` (FATAL EXCEPTION IN SYSTEM PROCESS)
5. `make publish-verify` 통과 확인

### Deliverable

빈 패키지 5개 + 샘플 3개. `go build`·`go test ./...` 그대로 green.

### Acceptance

- `go list ./internal/domain ./internal/analysis ./internal/source ./internal/pipeline ./internal/store` 5개 모두 출력
- `testdata/android/*.log` 3개 존재, 각 100줄 이상

---

## Phase 1 · 도메인 타입 확정

**목표**: 설계의 타입을 코드로 고정. 아무 로직도 의존하지 않으므로 가장 먼저 안정화.

### 작업

1. `internal/domain/seq.go` — `Seq int64`, 기본 연산
2. `internal/domain/line.go` — `Line`
3. `internal/domain/record.go` — `Record` + `Level` enum + `String()`
4. `internal/domain/span.go` — `SpanKind`, `Span`
5. `internal/domain/finding.go` — `FindingKind`, `Finding`
6. 각 타입 JSON 마샬링 테스트 (`*_test.go`)
7. 스키마 버전 상수 `SchemaVersion = 1`

### Deliverable

순수 타입 + JSON round-trip 테스트.

### Acceptance

- `go test ./internal/domain/... -cover` ≥ 90%
- 다른 `internal/*` 패키지 의존 0개 (`go list -deps`로 확인)
- 기존 UI 빌드 영향 없음

---

## Phase 2 · Record Builder (L0 → L1)

**목표**: 기존 `filter.ExtractRawLevel`과 `FormatAndroid` 정규식을 재사용해 `Line → Record` 변환기를 만든다. 아직 파이프라인은 없다 — 순수 함수.

### 작업

1. `internal/pipeline/record_builder.go` — `BuildRecord(line domain.Line, fmt filter.LogFormat) domain.Record`
2. Android threadtime / time 포맷에서 PID/TID/Tag/Component 추가 추출 (현재 level만 추출하므로 정규식 그룹 확장)
   - `format.go`의 `AndroidHeadThreadtime` 패턴에 그룹 추가 (기존 코드에 영향 가지 않도록 새 패턴 정의)
3. 시간 파싱: `MM-DD HH:MM:SS.sss` / `YYYY-MM-DD …` → `time.Time` (타임존은 로컬)
4. 테이블 테스트: 입력 라인 → 기대 Record

### 영향 범위

- `internal/filter/format.go`: 신규 정규식 상수 추가만. 기존 `AndroidHeadThreadtime` 불변.
- 그 외 기존 코드 변경 없음.

### Deliverable

`BuildRecord` 함수 + Android 샘플에 대한 골든 테스트.

### Acceptance

- `testdata/android/*.log`의 첫 50줄을 Record 배열로 변환, JSON 덤프가 `testdata/golden/records_*.jsonl`과 일치
- coverage ≥ 85%

---

## Phase 3 · Store (in-memory 인덱스)

**목표**: `Index[T]` 제네릭 구현 + `Store` 조립. 디스크 영속화는 Phase 6으로 미룸.

### 작업

1. `internal/store/memindex.go` — `MemIndex[T]` (append-only 슬라이스, `RangeBySeq`)
2. `internal/store/store.go` — `Store` 구조체, getters
3. 동시성: 단일 writer / 다수 reader, `sync.RWMutex`
4. 벤치: 100만 건 append + range 조회 benchmark

### Deliverable

메모리 Store. 단일 writer 보장.

### Acceptance

- `BenchmarkMemIndex_Append` ≤ 200ns/op
- `BenchmarkMemIndex_Range` 100만 건 중 1만 건 slice ≤ 1ms
- race 테스트 통과 (`go test -race`)

---

## Phase 4 · Analyzer 인터페이스 + Classify 규칙

**목표**: 가장 ROI 높은 Tier A 분류기를 먼저 붙인다. 파이프라인 없이 `[]Record → []Finding` 순수 함수로도 동작.

### 작업

1. `internal/analysis/interface.go` — `Analyzer`, `StatefulAnalyzer`, `BlockAnalyzer`
2. `internal/analysis/classify/rules.go` — 내장 규칙 테이블(Go 코드):
   - `FATAL_JAVA` (tag=AndroidRuntime, msg prefix "FATAL EXCEPTION")
   - `ANR` (tag=ActivityManager, msg prefix "ANR in")
   - `NATIVE_CRASH_HEADER` (tag=DEBUG, msg `^\*\*\* \*\*\* \*\*\*`)
   - `WATCHDOG` (tag=Watchdog)
   - `LMK_KILL` (tag=lowmemorykiller)
   - `BINDER_FAIL` (tag=Binder, msg contains "FAILED BINDER TRANSACTION")
   - `SELINUX_DENIED` (msg contains "avc: denied")
   - `WTF` (msg contains "Log.wtf" or level=F)
   - `OOM` (msg contains "OutOfMemoryError")
3. `internal/analysis/classify/classify.go` — `Classifier` (Analyzer 구현)
4. `internal/analysis/classify/rules.toml` (선택, v1은 Go 코드 우선)
5. 테스트: `testdata/android/*.log` → 각 파일 최소 1건 해당 kind 발견

### Deliverable

Classifier Analyzer 구동. 3개 샘플에서 기대 Finding 생성.

### Acceptance

- 샘플 파일별 Finding 골든 테스트 통과
- Classifier는 stateless (`Snapshot()`·`Restore()` 불필요)
- coverage ≥ 85%

---

## Phase 5 · Block Analyzer (다중라인 파싱)

**목표**: native crash, Java FATAL, ANR을 **Span 하나**로 묶는다. AI가 먹는 "사건" 단위의 시작.

### 작업

1. `internal/analysis/block/builder.go` — 공통 state machine (`Idle`, `InBlock`, `Closed`)
2. `internal/analysis/block/native.go` — tombstone 파서
   - 시작: `tag=DEBUG` + `^\*\*\* \*\*\* \*\*\*`
   - 종료: signal 라인 이후 backtrace 블록 종료(빈 tag 전환 + N라인 whitespace-less)
   - span summary: signal, fault addr, top frame
3. `internal/analysis/block/java.go`
   - 시작: `AndroidRuntime: FATAL EXCEPTION` or `*** FATAL EXCEPTION IN SYSTEM PROCESS`
   - 포함: `\tat `, `Caused by:` 체인
   - 종료: `\tat ` 없는 라인 N개 연속
   - summary: 최상위 예외 class + message
4. `internal/analysis/block/anr.go`
   - 시작: `ActivityManager: ANR in`
   - 포함: "CPU usage from", process 블록
   - 종료: 같은 PID가 다른 tag로 정상 로그 복귀
   - summary: ANR component + reason
5. 각 파서 테이블 테스트 (golden Span JSON)

### Deliverable

3종 BlockAnalyzer. Span + Classify Finding이 같은 사건을 가리킴(Finding.SpanID 채워짐).

### Acceptance

- 3개 샘플 각각 Span 정확히 1건 생성
- Span 범위가 실제 사건 라인을 **포함**하고 **1라인 이상 초과하지 않음** (±1 라인 허용)
- coverage ≥ 80%

---

## Phase 6 · 파이프라인 조립

**목표**: `LogSource → RecordBuilder → fanout(Classifier, BlockAnalyzers) → Store` 을 하나의 runnable로 엮는다. TUI는 아직 안 건드린다.

### 작업

1. `internal/source/interface.go` — `LogSource` 포트
2. `internal/source/stdin.go` — 기존 `loginput.ScanLines`를 포트로 감쌈
3. `internal/source/file.go` — 기존 `fileindex.IncrementalOffsetIndex`를 포트로 감쌈
4. `internal/pipeline/pipeline.go`
   - `Pipeline.Run(ctx)` — goroutine 3개(source reader, record builder, analyzer fanout)
   - 채널 capacity: Line=8192, Record=8192, Finding=4096
   - drop-oldest + `Metrics` 구조 (dropped count, lag)
5. `cmd/logsee/main.go` 옆에 `cmd/logsee-ana/main.go` (실험용 binary) — `logsee-ana <file>` 로 Findings를 stdout JSONL 출력
6. e2e 테스트: 샘플 파일 입력 → 기대 Findings/Spans

### Deliverable

파이프라인 독립 binary. TUI와 무관하게 돌아감.

### Acceptance

- `./logsee-ana testdata/android/anr_input_dispatch.log` 출력 JSONL이 골든과 일치
- `go test -race ./internal/pipeline/...` 통과
- CPU ≤ 10k lines/sec 환경에서 lag=0

---

## Phase 7 · JSON Export (headless)

**목표**: 기존 `logsee` binary에 `--export-anomalies` 플래그 추가. 파일/파이프 처리 후 JSONL stdout.

### 작업

1. `cmd/logsee/main.go`에 플래그 `--export-anomalies=jsonl`
2. 플래그 설정 시 TUI 띄우지 않고 파이프라인만 실행 → stdout 출력 후 종료
3. 출력 스키마: `{type: "finding"|"span", ...domain fields}`
4. README 섹션 "AI analysis" 추가

### Deliverable

`cat app.log | logsee --export-anomalies=jsonl | jq 'select(.kind=="ANR")'` 시나리오 동작.

### Acceptance

- exit code 0, stderr clean
- 샘플 3개 × golden JSONL diff zero
- TUI 동작 변경 없음 (플래그 없으면 기존 동작)

---

## Phase 8 · MCP stdio 서버 (취소)

당초 `logsee mcp` 서브커맨드로 JSON-RPC 2.0 stdio MCP 서버를 제공했으나,
`--export-anomalies` JSONL 경로가 LLM 파이프라인 연결에 충분하다는 판단으로
v1.7.x 시점에 코드/문서 모두 제거함. 필요 시 과거 커밋(`7210444`, revert in
later chore commit)에서 복구 가능.

---

## Phase 9 · TUI 통합 (Finding 시각화)

**목표**: 기존 `Model`을 최소 침습으로 수정. 분석 결과를 gutter에 표시.

### 작업

1. `internal/ui/` 에 `anomalyBus` 필드 (채널) 추가 — 파이프라인 Sink
2. Gutter에 Finding 마커(`!` for ERROR, `✖` for FATAL, etc. — 기존 bookmark 스타일 재사용)
3. 키 `A` (대문자) — "anomalies only" 필터 토글 (filter.Program에 `anomaly:any` 추가 필요 → Phase 9a)
4. Phase 9a: `internal/filter/parser.go`에 `anomaly:<kind>` 예약 태그 추가
5. 상태바에 "anomalies: N" 카운트

### 영향 범위

- `model.go`, `model_view_chrome.go`, `model_render_log.go` 일부 수정
- 기존 키 바인딩·필터 의미 변경 **없음**
- 분석 비활성 시(기본) 렌더 경로는 기존과 동일

### Acceptance

- 분석 off일 때 TUI 성능/동작 회귀 없음
- `A` 토글 시 Finding 존재하는 라인만 표시
- 기존 테스트 전부 green

---

## Phase 10 · 문서·예제

**목표**: 사용자/기여자 가이드.

### 작업

1. `README.md` — AI analysis 섹션 (`--export-anomalies` 예)
2. `docs/architecture/anomaly-detection.md` — 오픈 이슈 업데이트
3. `docs/plans/anomaly-detection-plan.md` — 본 문서 완료 체크

---

## 릴리스 계획

| 버전 | 포함 | 비고 |
|---|---|---|
| v1.3.0 | Phase 0–4 | 내부 코어. 외부 변화 없음 |
| v1.4.0 | Phase 5–6 | `logsee-ana` 실험 binary |
| v1.5.0 | Phase 7 | `--export-anomalies` 정식 |
| v1.6.0 | (건너뜀) | Phase 8(MCP)은 제거 |
| v1.7.0 | Phase 9(plumbing)–10 | TUI 훅 + README/예제 |
| v1.8.0 | Phase 9b.1–9b.2 | `anomaly:*` 필터 DSL + `A` 토글 |
| v1.9.0 (예정) | Phase 9b.3 | gutter 마커 + 상태바 카운트 (렌더 레이어 변경 + 골든 갱신) |

## 진행 상태 (2026-04-19)

| Phase | 상태 | 주요 산출물 |
|---|---|---|
| 0 | ✅ | `internal/{domain,analysis,source,pipeline,store}` 스켈레톤 + `testdata/android/*.log` 3종 |
| 1 | ✅ | Seq(alias), Level, LineFormat, Record, Span, Finding + SchemaVersion |
| 2 | ✅ | `pipeline.RecordBuilder` + threadtime 정규식 + 골든 JSONL 3개 |
| 3 | ✅ | `store.MemIndex[T]` + `Store`. Append 46ns, Range 24µs |
| 4 | ✅ | `analysis.Analyzer`/`Output` + `classify.Classifier` + 10 Tier A 규칙 |
| 5 | ✅ | `block.NewNativeCrash / NewJavaFatal / NewANR` |
| 6 | ✅ | `source.FileSource`/`ReaderSource` + `pipeline.Pipeline` + `cmd/logsee-ana` |
| 7 | ✅ | `logsee --export-anomalies` JSONL |
| 8 | ❌ | 제거(`--export-anomalies` 로 충분) |
| 9 (plumbing) | ✅ | Model.classifier + `FindingAt`/`FindingCount` — 렌더 미변경 |
| 9b.1 | ✅ | `anomaly:any` / `anomaly:<kind>` 필터 DSL (`MatchContext`) |
| 9b.2 | ✅ | `A` 토글(anomaly-only view). 상태바 카운트는 별도 슬라이스 |
| 9b.3 | ⏳ | 줄 gutter의 severity 마커 + 상태바 카운트 — 렌더 레이어 변경으로 골든 테스트 대규모 업데이트 필요. 독립 슬라이스 (9b.3) 으로 분리 |
| 10 | ✅ | README "AI-assisted analysis" 섹션 (`--export-anomalies` 예) |

## 리스크

| 리스크 | 완화 |
|---|---|
| Android 포맷 다양성(OEM 패치)으로 regex 깨짐 | Phase 4 규칙 TOML 외부화 예비. `testdata/android/` 샘플 누적 |
| Analyzer 백프레셔로 lag 누적 | Metrics + status bar 노출. Phase 6 벤치에서 drop 발생률 측정 |
| `Model` 책임 분리 실패 시 Phase 9 통합 폭발 | Phase 9 시작 전 `Model` 순수 상태 추출 spike(1일) 선행 |
| Seq 단조성 깨짐(멀티 소스) | v1은 단일 소스만. 멀티 소스는 v2 별도 설계 |

## 오픈 이슈 → 결정 필요

- **Drain 포팅 타이밍**: 본 계획은 out-of-scope로 두었음. v1 끝난 뒤 별도 plan 문서로.
- **adb 직접 소스**: 현재는 파일/파이프만. adb 재접속 로직이 필요하므로 별도 Phase.
- **영속화 포맷**: JSONL 우선. bbolt 필요성은 실사용 후 판단.
- **journalctl (systemd journal) 지원**: 별도 계획 → [`docs/plans/journalctl-support-plan.md`](journalctl-support-plan.md).

## 관련 문서

- `docs/architecture/anomaly-detection.md` — 아키텍처
- `docs/plans/stdio-log-viewer-prd.md` — 전체 PRD
- `docs/architecture/log-read-pipeline.md` — L0 파이프라인
- `docs/architecture/log-type.md` — 기존 포맷 추출
