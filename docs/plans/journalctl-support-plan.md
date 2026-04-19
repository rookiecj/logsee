# journalctl (systemd journal) 지원 · 구현 계획

`logsee`의 어댑터/파이프라인을 Android adb 전용 가정에서 벗어나 **Linux systemd journal** 입력을 1급 시민으로 다루게 한다. 설계 기반은 `docs/architecture/anomaly-detection.md` 의 3-layer 구조(Source·Record·Analyzer)에 그대로 맞춘다.

## Why

- 서버/임베디드 리눅스 이상 분석은 대부분 `journalctl` 가 1차 증거원. Android adb와 같은 엔지니어링 수요(ANR·crash 탐지)와 대칭.
- 현재 `pipeline.BuildRecord`·`classify.Rules()`·`filter.DetectLogFormat` 모두 adb threadtime만 인식. 플러그형 설계는 갖춰져 있으므로 포맷·규칙·소스 3개 축만 더 얹으면 됨.
- `--export-anomalies` 출력 스키마가 포맷 중립이라 journal도 같은 소비자(LLM, jq 파이프라인)에 바로 붙음.

## 목표

- `logsee <journal.log>` 과 `journalctl -f -o short-iso-precise | logsee` 둘 다 **자동 감지** 로 동작.
- 명시적 오버라이드: `--log-type=journal` (CLI > config > 기본).
- systemd journal 고유 이상(systemd unit failed, OOM-kill, kernel BUG/panic, coredump, apparmor/selinux DENIED) Tier A 규칙 최소 8종.
- kernel panic / systemd-coredump 를 **블록 단위 Span** 으로 묶음 (Android native tombstone과 동급 품질).
- 기존 adb 경로 무결: threadtime 라인이 journal 규칙을 오발하지 않아야 하고 반대도 마찬가지.

## 비목표

- systemd D-Bus / libjournal 바인딩. subprocess + 텍스트/JSON 출력만 다룸 (cgo 회피, 크로스빌드 유지).
- 바이너리 `.journal` 파일 직접 파싱. 항상 `journalctl` 명령을 경유.
- Windows Event Log. 별도 계획.
- 실시간 알림/웹훅. 이상 이벤트는 기존 `--export-anomalies` JSONL 로만 내보냄.

## 선행 조건

- Android 경로(Phase 0–10 + 9b)가 main 기준으로 green 인 상태 유지. 본 계획은 **순수 additive**.
- Go 1.22 유지. subprocess 실행은 `os/exec` 로 충분.

## 입력 포맷 조사

| 형식 | 한 줄 예시 | 장단점 |
|---|---|---|
| `-o short` (기본) | `Apr 19 14:24:10 host systemd[1]: Starting foo...` | 연/타임존 모호. 파싱 까다로움. |
| `-o short-iso` | `2024-04-19T14:24:10+0900 host systemd[1]: Starting foo...` | ISO 타임. **1순위 텍스트 파서**. |
| `-o short-iso-precise` | `2024-04-19T14:24:10.123456+0900 host …` | µs 정밀도. 시간 정렬/상관에 유리. |
| `-o json` / `json-pretty` | `{"MESSAGE":"...","PRIORITY":"6","_SYSTEMD_UNIT":"foo.service","_PID":"1234",...}` | 완전 구조. **2순위 파서**(선택 경로로 지원). |
| `-o export` | 바이너리 프레임. | 스킵. |

**1차 목표**: short-iso / short-iso-precise 텍스트 파싱. 규모·필드가 충분하고 툴체인 의존이 없음.
**2차 목표(후속)**: `-o json` 파이프. 파서가 trivial 하므로 원하는 사용자에게는 `--log-type=journal-json` 로 오픈.

### 공통 필드 매핑

| logsee.domain.Record | journal 텍스트 그룹 | journal JSON 키 |
|---|---|---|
| Time | `2024-04-19T14:24:10(.123456)?(+0900)` | `__REALTIME_TIMESTAMP` (µs since epoch) |
| PID | `systemd[1]` 의 `1` | `_PID` |
| TID | (없음) | (없음) |
| Tag | `systemd` (process/comm) | `_COMM` 또는 `SYSLOG_IDENTIFIER` |
| Component | `foo.service` (unit, message에서 유추) | `_SYSTEMD_UNIT` |
| Message | `:` 이후 전체 | `MESSAGE` |
| Level | PRIORITY 없음 → message 내 `error`/`warn` 추론 or Unknown | `PRIORITY` 0–7 → domain.Level 매핑 |

PRIORITY → Level 매핑 표:

| PRIORITY | syslog name | domain.Level |
|---|---|---|
| 0 | emerg | Fatal |
| 1 | alert | Fatal |
| 2 | crit | Fatal |
| 3 | err | Error |
| 4 | warning | Warn |
| 5 | notice | Info |
| 6 | info | Info |
| 7 | debug | Debug |

텍스트(short-iso) 경로는 PRIORITY 없으므로 기본 Info, 규칙이 level-independent 하게 설계됨을 가정.

## 목표 아키텍처 변화

3-layer 설계 그대로. 새 슬롯만 채운다.

```
Source     : source.JournalctlSource  (신규, subprocess 또는 stdin-pipe)
                │
Record     : pipeline.BuildRecord + JournalParser  (format=LineFormatJournal)
                │
Analyzer   : classify.Rules + systemd Tier A 규칙 테이블 추가
             analysis/block/kernel_panic.go        (신규 block analyzer)
             analysis/block/systemd_coredump.go    (신규 block analyzer)
```

어떤 기존 파일도 **의미를 바꾸지 않는다**. 규칙 테이블은 format 가드(FormatAndroid / FormatJournal)로 분리해 충돌을 막는다.

## 스키마 확장

- `internal/domain/format.go`: `LineFormatJournal uint8 = 4` 추가, `String()`·`UnmarshalText` 업데이트.
- `internal/filter/format.go`: 기존 `LogFormat` enum 에도 `FormatJournal` 별도 상수 추가(텍스트 프로브용). 이름 충돌 방지 위해 `FormatSystemdJournal` 명시.
- `domain.FindingKind` 신규 값: `FindingSystemdUnitFailed`, `FindingSystemdCoredump`, `FindingKernelPanic`, `FindingKernelBUG`, `FindingSegfault`, `FindingOOMKilledLinux`, `FindingAppArmorDenied`, `FindingAuditSELinuxDenied`, `FindingSSHAuthFailure`.

기존 FindingKind enum 뒷자리에 **추가만**(이미 unknown-name tolerant 디코딩을 채택해서 wire 호환).

## 포맷 감지

`filter.DetectLogFormatN` 에 journal 패턴 추가. 첫 N 비어있지 않은 라인 중:

- `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?([+-]\d{4}|Z)\s+\S+\s+\S+(\[\d+\])?:` → journal 점수 +1
- 기존 Android / bracket 점수와 경쟁, 최다 득표 채택.

JSON 경로는 감지 생략 (명시적 `--log-type=journal-json`).

## 단계별 구현

### Phase J0 · testdata

- `testdata/journalctl/systemd_unit_failed.log` (short-iso, unit start→fail→restart, 120+ lines)
- `testdata/journalctl/kernel_panic.log` (kernel BUG / Oops / panic 블록, 120+ lines)
- `testdata/journalctl/oom_killer.log` (`kernel: Out of memory: Killed process`, 100+ lines)
- `testdata/journalctl/coredump.log` (systemd-coredump 생성부터 dump until 종료, 100+ lines)
- `testdata/journalctl/auth_failures.log` (sshd Failed password 반복 + fail2ban 개입, 100+ lines)

각 샘플은 현실적 노이즈(정상 init, network, cron) 섞어 Tier A 규칙의 오발화 없음을 회귀 검증 가능하게.

### Phase J1 · 포맷 감지 + Record 빌더

1. `domain.LineFormatJournal` 상수 + JSON 이름 `journal`.
2. `filter/format.go` : `FormatSystemdJournal`, `journalHead` 정규식, `DetectLogFormatN` 에 점수 포함.
3. `pipeline/journal_parser.go`:
   - text(short-iso/precise) 한 줄 → `domain.Record` (Seq, Time, Level=Info 기본, PID, Tag=_COMM, Component=유추, Message).
   - 정규식: `^(?P<ts>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d{4}|Z))\s+(?P<host>\S+)\s+(?P<tag>[^\s\[]+)(?:\[(?P<pid>\d+)\])?:\s*(?P<msg>.*)$`
   - Component: message 앞쪽의 `<unit>.service:` 프리픽스가 있으면 거기서, 없으면 빈값.
4. `pipeline.BuildRecord` 확장: `format == LineFormatJournal` 분기에서 위 파서 호출.
5. 골든 테스트: 각 샘플 첫 50줄 → `testdata/golden/records_journal_*.jsonl`.

### Phase J2 · JournalctlSource

1. `internal/source/journal.go`:
   - `type JournalctlSource struct { args []string; cmd *exec.Cmd }`.
   - `NewJournalctl(args ...string) *JournalctlSource` — 기본 `journalctl -f -o short-iso-precise --no-pager`.
   - `Lines(ctx)` : subprocess 스폰 → stdout 라인 스트림. ctx 취소 시 process kill.
   - `Close()`: 프로세스 종료 + wait.
   - 크로스플랫폼: 비-linux 빌드 시 `journalctl_unsupported.go` (build tag) 로 friendly error.
2. 테스트: subprocess 대신 `script` fixture(샘플 파일 cat) 로 대체 가능하도록 `execCmd` 주입 hook.

### Phase J3 · Classify Tier A

`internal/analysis/classify/rules.go` 에 journal 전용 규칙 추가. 각 규칙은 `MsgContains` / `MsgRegex` + (선택) `TagEq`. Android 규칙과 분리된 슬라이스로 두고 `Rules(format domain.LineFormat) []Rule` 형태로 가드 — 혹은 single table 에 `Formats []LineFormat` 필드 추가. **제안**: Rule에 `Formats` 필드 추가, 빈 값이면 모든 포맷 허용, 지정하면 해당 포맷에서만 평가.

신설 규칙:

| FindingKind | 시그니처 |
|---|---|
| SystemdUnitFailed | `TagEq=systemd`, `MsgContains=["Failed with result", "Main process exited, code=dumped"]` |
| SystemdCoredump | `TagEq=systemd-coredump`, `MsgContains=["Process "," of user "]` |
| KernelPanic | `TagEq=kernel`, `MsgPrefix="Kernel panic"` |
| KernelBUG | `TagEq=kernel`, `MsgRegex=^BUG:` |
| Segfault | `TagEq=kernel`, `MsgContains=["segfault at"]` |
| OOMKilledLinux | `TagEq=kernel`, `MsgContains=["Out of memory: Killed process"]` |
| AppArmorDenied | `MsgContains=["apparmor=\"DENIED\""]` |
| AuditSELinuxDenied | `TagEq=audit`, `MsgContains=["avc: denied"]` |
| SSHAuthFailure | `TagEq=sshd`, `MsgContains=["Failed password for"]` |

`Rules()` 반환 테이블 확장 + unit test: 각 journal 샘플에서 기대 FindingKind 정확히 1+회 검출.

### Phase J4 · Block analyzers

1. `internal/analysis/block/kernel_panic.go` — `BUG:`/`Oops:`/`Kernel panic:` 시작, 다음 비-kernel 태그 또는 `---[ end trace` 엔드마커까지 Span.
2. `internal/analysis/block/systemd_coredump.go` — `systemd-coredump[…]: Process X` 시작, 같은 PID 의 연속된 coredump 태그 라인까지.
3. 각각 `NewKernelPanic()` / `NewSystemdCoredump()` 생성자, 기존 NewNativeCrash 패턴 답습.
4. 골든 Span: 샘플에서 정확히 1건, 라인 범위 ±1 허용.

### Phase J5 · CLI / config 통합

1. `cmd/logsee/main.go` : 기존 `--log-type` 플래그에 `journal` 값 추가. `parseLogTypeKind` 확장.
2. `config.LogType` 기본값: auto (그대로). 감지 결과 → journal 로 귀결 시 `effectiveLogFmt = FormatSystemdJournal`.
3. `cmd/logsee --export-anomalies` 플로우에 `parseFormat("journal")` 케이스 추가.
4. 새 CLI 플래그 `--journalctl` : 있을 때 `source.NewJournalctl(...)` 를 source 로 씀(파일 인자 대신).
   - 예: `logsee --journalctl -u nginx.service --since "1h ago"` → subprocess 인자로 전달.
5. UI 상태바 `type:` 표기에 `journal` 추가.

### Phase J6 · TUI·필터 호환성

- `filter.DetectLogFormat` 감지 시 상태바 반영, 다른 것 없음.
- 기존 `anomaly:*` 필터 태그가 journal 기반 FindingKind 에도 자연히 작동함을 확인하는 회귀 테스트 1개.
- `A` 토글 동작 동일 (FindingKind 무관).

### Phase J7 · 문서

- `docs/architecture/log-type.md` : journal 포맷 섹션 추가(감지·추출 필드·Level 매핑 표).
- `README.md` : "AI-assisted analysis" 섹션에 journalctl 예:
  ```bash
  journalctl -u nginx.service -f -o short-iso-precise | logsee --log-type=journal --export-anomalies -
  logsee --journalctl -u nginx.service --since "2h ago" --export-anomalies
  ```
- `docs/plans/anomaly-detection-plan.md` 에 "journal 지원: 별도 plan 참조" 한 줄.

## 릴리스 계획

| 버전 | 포함 | 비고 |
|---|---|---|
| v1.10.0 | J0–J1 | 포맷 감지 + Record 빌더 + 골든. TUI 체감 없음 |
| v1.11.0 | J2–J3 | JournalctlSource + Tier A 규칙 |
| v1.12.0 | J4 | kernel panic + coredump block |
| v1.13.0 | J5–J7 | CLI/config/문서 정리 |

Phase 9b.3(gutter)와 독립이라 순서는 제품 우선순위로 재배치 가능.

## Acceptance (릴리스별 공통)

- `make publish-verify` green (fmt/vet/test/build).
- `go test -race ./internal/pipeline/... ./internal/analysis/... ./internal/source/...` green.
- 기존 Android 골든 JSONL diff zero — journal 규칙이 adb 샘플을 오발화 하지 않음.
- 신규 journal 샘플 × 기대 Finding·Span 골든 작성.

## 리스크 / 오픈 이슈

- **distro 별 journalctl 포맷 편차**: Ubuntu/Arch/RHEL 가 공백·호스트 길이·micro-초 표기에서 약간 다름. 샘플은 3개 distro 에서 채집 권장. 정규식은 ±micro-초 optional 로 유연하게.
- **timezone 보존**: `+0900`/`Z` 그대로 `time.Time`에 보존. 내부 비교는 UTC 환산 위치에서.
- **대용량 `-b` 부팅 덤프**: 수백만 줄 가능. `Ring`·`Store` 기본 max-lines 로 이미 방어되지만 메모리 상한 문서화 필요.
- **CGO 없이 binary journal 읽기 불가** — `journalctl` 의존은 수용, 배포물에 이 의존성 명시.
- **`-o json`**: JSON 스트림 파서는 구현 trivial 이나 이슈는 key 차이(distro별 존재 여부). 후속 단계에서 별도로 도입.

## 관련 문서

- `docs/architecture/anomaly-detection.md` — 3-layer 설계 근거
- `docs/architecture/log-type.md` — 기존 adb/plain/bracket 포맷 추출
- `docs/plans/anomaly-detection-plan.md` — Android 경로 구현 단계
