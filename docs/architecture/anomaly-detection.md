# Anomaly Detection · 아키텍처 설계

## Why

logsee는 지금까지 "라인 스트림"을 사람이 보는 **TUI 뷰어**였다. AI 보조 분석(특히 Android adb system log의 이상탐지) 요구가 더해지면서 기존 데이터 모델과 축이 다른 부하가 생긴다.

| 축 | 뷰어 (현재) | 이상탐지 (신규) |
|---|---|---|
| 단위 | 라인 | 블록/이벤트/기간(span) |
| 상태 | stateless 술어(`filter.Program`) | stateful (PID 세션, 윈도우 통계, template 집합) |
| 시간성 | synchronous render | async, 배치 허용 |
| 확장 차원 | key binding, 렌더 옵션 | detector, rule, 모델 |
| 소비자 | 화면 하나 | TUI + JSON + MCP + CLI + webhook |

`filter.Program`/`Model`에 기능을 욱여넣으면 god object(Model: 101 edges, applyIncomingLines: 93 edges)가 더 커진다. 이질성을 **레이어와 포트로 격리**해 TUI·분석·출력이 독립 진화하도록 한다.

## 목표

- **이질성 격리**: 이상탐지 상태/로직이 `internal/ui/*`를 오염시키지 않는다.
- **확장 지점 단일화**: 새 규칙·새 블록 파서·새 출력·새 입력은 **Analyzer** 또는 **Adapter** 중 정확히 한 인터페이스로 추가된다.
- **TUI 비의존 코어**: 분석 파이프라인이 Bubble Tea 없이도 동작한다(headless 질의, MCP).
- **Seq 중심 좌표계**: 기존 `Seq`(1-based 파일/세션 절대 줄번호)를 모든 상위 레이어의 유일한 cross-layer key로 사용한다.
- **점진 이행**: 현 코드 대대적 이사 없이 한 슬라이스씩 추출한다.

## 비목표

- 필터 DSL 의미 변경 없음(PRD §8.0 layer 1 유지).
- `Ring` / `WindowProvider` 외부 계약 변경 없음.
- ML(DeepLog/LogBERT)은 v1 범위 밖. Analyzer 인터페이스가 수용하기만 하면 됨.
- 실시간 알림/웹훅 v1 미포함. 포트만 열어둔다.

## 목표 아키텍처

```
┌─────────────────────────────────────────────────────────┐
│ Outbound adapters   TUI │ JSON │ MCP │ CLI │ webhook    │
├─────────────────────────────────────────────────────────┤
│ Query layer         filter.Program │ EventQuery          │
├─────────────────────────────────────────────────────────┤
│ Derived indexes (all keyed by Seq ranges, additive)     │
│   L3 anomalies      Finding{kind, span, confidence}     │
│   L2 events         Span{start_seq..end_seq, kind}      │
│   L1 records        Record{ts, level, pid, tag, msg}    │
│   L0 lines          (existing Ring + WindowProvider)    │
├─────────────────────────────────────────────────────────┤
│ Analysis pipeline   Analyzer chain (async, bounded)     │
├─────────────────────────────────────────────────────────┤
│ Inbound adapters    stdin │ file │ adb │ journalctl(후) │
└─────────────────────────────────────────────────────────┘
```

### 레이어 규칙

1. **상위 레이어는 하위 레이어에만 의존**. 역방향 참조 금지.
2. **모든 상위 레이어는 Seq 범위로만 L0를 참조**. 포인터/인덱스 복사 금지(ring 교체 안전).
3. **인덱스는 derive 가능**. 손실 시 재생성할 수 있어야 한다(checksum/버전만 맞으면).
4. **L0은 변경 불가(append-only)**. 상위 레이어의 재분석은 L1부터 다시 돈다.

## 도메인 타입 (`internal/domain`)

순수 데이터 타입. 다른 internal 패키지에 의존하지 않는다.

```go
type Seq int64

type Line struct {
    Seq  Seq
    Raw  string
    Time time.Time // 수신 시각(포맷 파싱 이전)
}

type Level int
const (
    LevelUnknown Level = iota
    LevelVerbose; LevelDebug; LevelInfo; LevelWarn; LevelError; LevelFatal
)

type Record struct {
    Seq       Seq
    Time      time.Time   // 파싱된 로그 시각
    Level     Level
    PID, TID  int32
    Tag       string
    Component string // Android component(package/process)
    Message   string
    Format    filter.LogFormat
    SchemaVer uint16
}

type SpanKind uint8
const (
    SpanNativeCrash SpanKind = iota + 1
    SpanJavaFatal
    SpanANR
    SpanWatchdog
    SpanGCStorm
)

type Span struct {
    Kind     SpanKind
    StartSeq Seq
    EndSeq   Seq
    PID      int32
    Summary  string // one-line
}

type FindingKind uint16 // 규칙 ID space
type Finding struct {
    Kind       FindingKind
    Seq        Seq    // 대표 라인(블록이면 start)
    SpanID     int64  // 0이면 단일라인 발견
    Severity   Level
    Confidence float32 // 0..1
    Fields     map[string]string
}
```

### 스키마 진화 규칙

- `SchemaVer`는 모든 영속 타입의 필수 필드.
- deprecation: 필드 제거 금지. 새 필드 추가, 이전 버전은 zero value로 읽는다.
- 디스크 포맷은 JSONL(append-only) 또는 bbolt(key=Seq). v1은 JSONL 채택.

## 포트 (인터페이스 사양)

### LogSource (inbound)

```go
type LogSource interface {
    Lines(ctx context.Context) <-chan domain.Line
    Close() error
}
```

기존 stdin/file 경로를 이 포트로 감싸 파이프라인 입력을 단일화. `cmd/logsee/main.go`의 `setupModelWithDiskFallback()`가 `LogSource` 인스턴스를 조립한다.

### Analyzer (core)

```go
type Analyzer interface {
    Name() string
    OnRecord(r domain.Record) []domain.Finding
    Flush() []domain.Finding
}

type StatefulAnalyzer interface {
    Analyzer
    Snapshot() ([]byte, error)
    Restore([]byte) error
}

type BlockAnalyzer interface {
    Analyzer
    OnBlockStart(r domain.Record) bool
    OnBlockLine(r domain.Record) (done bool)
    Emit() domain.Span
}
```

**규칙**: Analyzer는 I/O 없음. 모든 외부 접근은 생성 시 주입된 인터페이스를 통해서만. 테스트는 in-memory record 배열로 완결된다.

### Index / Store

```go
type Index[T any] interface {
    Append(v T) error
    Range(from, to domain.Seq) iter.Seq[T]
    Get(id int64) (T, bool)
    Len() int
}

type Store interface {
    Lines()     Index[domain.Line]
    Records()   Index[domain.Record]
    Spans()     Index[domain.Span]
    Findings()  Index[domain.Finding]
}
```

v1: 메모리 구현 + 선택적 JSONL flush. v2에서 bbolt 드라이버 추가.

### Sink (outbound)

```go
type Sink interface {
    Emit(ctx context.Context, f domain.Finding) error
    Close() error
}
```

TUI bus, JSONL writer, MCP notifier 모두 동일 포트로 꽂힌다.

## 데이터 플로우

```
LogSource
  └─► [append] Lines (L0)
        └─► RecordBuilder  ── Format 추출 + Parse
              └─► [append] Records (L1)
                    ├─► fanout ──┐
                    │            ├─► ClassifyAnalyzer  ─► Findings (L3)
                    │            ├─► BlockAnalyzer ×N  ─► Spans (L2)
                    │            └─► TemplateMiner     ─► Findings (L3, rare)
                    └─► TUI Model (rendering only)
```

- **렌더 경로는 L0/L1만 구독**. Findings/Spans는 비동기로 도착 → gutter/badge 갱신.
- **백프레셔**: analyzer 채널은 capacity=N(기본 4096). 포화 시 drop-oldest + lag counter 증가(status bar 노출).
- **순서 보장**: Record는 Seq 오름차순. Analyzer는 단조 Seq 입력을 기대한다.

## 인덱스 모델

| 인덱스 | 키 | 저장 | 재생성 비용 |
|---|---|---|---|
| Lines | Seq | Ring (hot) + `--out` 파일 (cold) | 재생성 불가 (원천) |
| Records | Seq | 메모리 + 선택 JSONL | L0 재파싱 O(N) |
| Spans | `span_id` + (StartSeq,EndSeq) | 메모리 + JSONL | L1 재분석 O(N) |
| Findings | (Seq, Kind) | 메모리 + JSONL | L1/L2 재분석 O(N) |

cross-layer 조회 패턴:
- "이 Span에 속한 라인": Spans.Get(id).{Start,End}Seq → Ring/File로 읽음
- "이 PID의 Findings": Findings.Range(all) filter PID (보조 인덱스 v2)
- "시간 t±dt 이상": Records.RangeByTime → Findings lookup

## 패키지 구조 (목표)

```
internal/
  domain/              새로 생성. 순수 타입만. deps=0.
  source/              LogSource 포트 + 어댑터
    stdin.go           기존 loginput 감싸기
    file.go            기존 fileindex 감싸기
    adb.go             (후속) adb logcat 직접 스폰
  store/
    memstore.go        in-memory 인덱스
    jsonl.go           영속화 드라이버
  pipeline/
    pipeline.go        LogSource → stages → sinks
    record_builder.go  Line → Record 변환
    fanout.go          Analyzer 팬아웃
  analysis/
    interface.go       Analyzer/BlockAnalyzer/StatefulAnalyzer
    classify/
      rules.go         컴파일된 regex 테이블
      registry.go      FindingKind 등록
      rules.toml       데이터(코드 아님) - 태그 기반 매칭 규칙
    block/
      native.go        tombstone 블록 파서
      java.go          FATAL EXCEPTION + Caused by 체인
      anr.go           ANR + CPU usage trace
      watchdog.go
      builder.go       공통 state machine
    template/
      drain.go         Drain3 포팅
    baseline/
      baseline.go      정상 세션 snapshot diff
  query/
    program.go         (이사) 기존 filter/
    event_query.go     Span/Finding 질의
  view/
    tui/               (이사) 기존 ui/
    json/              headless exporter
    mcp/               MCP server
    cli/               `logsee query`, `logsee anomalies`
```

기존 `internal/ui/*`를 한꺼번에 옮기지 않는다. domain → source → pipeline → analysis 순으로 **신규 패키지를 먼저 구축**하고, TUI는 5단계에서 추출한다.

## 확장 시나리오

| 추가하고 싶은 것 | 건드리는 곳 | 건드리지 않는 곳 |
|---|---|---|
| 새 Tier A 규칙 (예: thermal throttle) | `analysis/classify/rules.toml` + (필요 시) 새 FindingKind 등록 | 모든 Go 코드 |
| 새 블록 종류 (예: kernel oops) | `analysis/block/kernel.go` (BlockAnalyzer 구현) | 기존 파서·파이프라인 |
| 새 입력 (journalctl) | `source/journal.go` | analysis / view |
| 새 출력 (Slack webhook) | `view/webhook/` (Sink 구현) | 분석 로직 |
| MCP tool 추가 | `view/mcp/tools.go` | domain/analysis |
| ML 모델 (DeepLog) | `analysis/ml/deeplog.go` (Analyzer 구현) | 기존 Analyzer |
| 다른 UI (웹) | `view/web/` + 같은 Store 질의 | core 전체 |

핵심: **모든 신규 기능이 Analyzer 또는 Adapter라는 한 지점으로 귀속**.

## 동시성·리소스 모델

- **goroutine 구성**: source(1) → line queue → record builder(1) → analyzer fanout(N개). TUI는 Bubble Tea의 `tea.Cmd`로 finding 채널을 subscribe.
- **채널 capacity**: Line 8192, Record 8192, Finding 4096. drop-oldest 정책 + metric.
- **메모리 상한**: L1/L2/L3 합쳐 기본 256MB. 초과 시 cold 영역을 JSONL로 flush, 메모리는 recent window만 유지.
- **ctx 취소**: 모든 Analyzer `OnRecord`는 ≤10ms 목표. 초과 시 WARN 로깅(logsee 자기 로깅은 stderr).

## 안전성·테스트

- **Analyzer 순수성**: 동일 Record 시퀀스 → 동일 Findings 시퀀스. 테스트는 golden JSONL로 고정.
- **Pipeline 프로퍼티 테스트**: Seq 단조, Finding은 반드시 기존 Record Seq를 가리킴, Span.End ≥ Span.Start.
- **Android 코퍼스**: `testdata/android/`에 대표 샘플 (ANR, tombstone, system_server 크래시 3종) 고정. Drain 결과도 snapshot.
- **Headless/TUI 동치성**: 같은 입력으로 headless JSON export와 TUI session snapshot이 동일한 Findings 집합을 생산.

## 오픈 이슈 (상태)

- **영속화 드라이버**: v1은 순수 인메모리. `Store`는 세션 종료 시 사라짐. JSONL/bbolt 드라이버 결정은 MCP 실사용 측정 후 판단. [pending]
- **Drain3 템플릿 마이닝**: Phase 5 이후로 유예(plan out-of-scope). [deferred]
- **adb 소스(직접 스폰)**: 현재 `source.FileSource` / `source.ReaderSource` 만 구현. adb subprocess 어댑터는 별도 플래그/패키지로 추가 예정. [deferred]
- **MCP 전송**: v1은 stdio + JSON-RPC 2.0 (`logsee mcp`). HTTP/SSE 전송은 v2. [stdio shipped]
- **TUI 완전 통합**: Phase 9 plumbing(Model에 classifier 연결)까지 완료. filter DSL 확장, `A` 토글, gutter 마커 렌더는 Phase 9b로 분리. [partial]

## 관련 문서

- `docs/plans/anomaly-detection-plan.md` — 이 설계의 단계별 구현 계획
- `docs/architecture/log-read-pipeline.md` — 기존 L0 파이프라인 (Ring + WindowProvider)
- `docs/architecture/log-type.md` — 기존 FormatAndroid/level 추출
- `docs/plans/seq-coord-pull-window-plan.md` — Seq 좌표계 근거
