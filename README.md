# logsee

stdio(파이프/리다이렉션)로 들어오는 텍스트 로그를 **실시간으로 TUI로 탐색**하면서, **stdin 입력**일 때만 받은 원문을 파일로 append 저장하는 도구입니다(**`input-file`로 읽을 때는 원본이 이미 파일이므로 저장하지 않음**).

- **입력**: stdin(기본) 또는 `input-file` 1개
- **출력 파일(stdin만)**: `--out`로 지정하거나, 비워두면 `./logsee-YYYYMMDD-HHMMSS.log` 자동 생성
- **플랫폼**: macOS/Linux, UTF-8 (Go 1.22+)

자세한 요구사항/동작 기준은 [`docs/plans/stdio-log-viewer-prd.md`](docs/plans/stdio-log-viewer-prd.md)를 참고하세요.

## Quick start

```bash
make dep
make build
make test
```

### Run (stdin)

```bash
some-cmd 2>&1 | ./bin/logsee
```

### Run (file)

```bash
./bin/logsee path/to/file.log
```

### Run with Makefile

```bash
make run ARGS="--out session.log"
```

## Usage

```text
logsee [flags] [input-file]
```

- **`input-file` 생략**: stdin에서 읽습니다.
- **`input-file` 지정**: 해당 파일을 처음부터 EOF까지 스트리밍합니다.
- **`-`**: stdin을 명시적으로 사용합니다.

## Flags (요약)

아래는 실제 구현 기준 요약입니다(정확한 설명/기본값은 `logsee -h` 출력과 [`cmd/logsee/main.go`](cmd/logsee/main.go)를 우선합니다).

- **`--out`**: **stdin 입력일 때만** 받은 원문 줄을 해당 파일에 append. 비어 있으면 현재 디렉터리에 `logsee-YYYYMMDD-HHMMSS.log` 생성. **`input-file` 지정 시에는 무시**(저장 안 함)
- **`--out-max-bytes`**: 출력 파일 로테이션(바이트). 기본값 `0` (로테이션 비활성, 단일 파일)
- **`--max-lines`**: 메모리에 유지할 최대 줄 수(링 버퍼)
- **`--ignore-case`**: **필터 매칭만** 대소문자 무시(하이라이트 검색은 항상 대소문자 구분)
- **`--no-line-numbers`**: 시퀀스(줄번호) 컬럼 숨김
- **`--sync-interval`**: `>0`이면 해당 주기로 출력 파일 `fsync` (예: `1s`)
- **`--stdin-batch-ms`**: 입력 줄을 UI 업데이트 단위로 묶기(0이면 줄마다 즉시 반영)
- **`--config`**: 로그 타입/패턴 설정 파일 경로(기본: `$HOME/.local/logsee/config.toml`). **`--print-default-config`**: 내장 기본값과 동일한 주석 포함 TOML을 stdout에 출력
- 필터/하이라이트 MRU 저장 디렉터리는 `config.toml`의 **`[history] dir`** 로 지정합니다(비워두면 `$HOME/.local/logsee`, 파일명 `state.json`).
- **`--log-type`**: `level:` 태그용 줄 형태 — `auto`(기본), `plain`, `adb` (상태바 `type:`에 표시)
- **`--log-type-probe-lines`**: `auto`일 때 샘플로 볼 비어 있지 않은 줄 수(기본 32)

`--log-type` / `--log-type-probe-lines` 는 `config.toml` 값보다 우선합니다(CLI > config > 기본값). 예제는 저장소 루트 `config.example.toml` 또는 `logsee --print-default-config` 참고.

## Keymap (핵심만)

- **종료**: **로그 목록 화면에서 `q` 또는 `Ctrl+Q`**, 또는 어디서나 `Ctrl+C` (필터·하이라이트 입력 중에는 `q`가 문자로 들어가고, `Ctrl+Q`·도움말에서는 소비됨)
- **도움말**: `F1` (인앱 도움말 + 버전). **로그 목록**에서만 `?`도 동일하게 열림 (IDE/터미널이 `F1`을 가로챌 때)
- **목록 이동**: 방향키 · **로그 목록 화면에서만** `h` `j` `k` `l` (`←` `↓` `↑` `→` 와 동일) · **`G`** (마지막 줄, `End` 와 동일; 필터·검색 입력 중에는 해당 글자로 입력)
- **필터 입력**: `Enter` 또는 `:`
- **하이라이트 입력(검색)**: `/` → 입력 → `Enter`로 확정
- **다음/이전 매칭 줄 이동**: `n` / `p` (로그 목록, 하이라이트 확정 시) · `Ctrl+n` / `Ctrl+p` (동일 동작·필터 입력 중 매칭 이동에 사용)
- **북마크**: `m` (현재 줄 토글/할당), `1`–`9` (슬롯 점프)
- **줄 줄바꿈(wrap) 토글**: `Ctrl+W`
- **줄 번호 컬럼 토글**: `Ctrl+I` (= `Tab`) — 로그 목록 화면에서만; CLI `--no-line-numbers`의 런타임 대응

상세 키맵/예외 규칙은 [`docs/plans/stdio-log-viewer-prd.md`](docs/plans/stdio-log-viewer-prd.md) 기준입니다.

## Filter syntax (아주 짧은 요약)

- 공백/탭으로 토큰을 나눕니다. `"..."`로 공백 포함 토큰을 만들 수 있습니다.
- **태그 토큰**: `tag:value` (부호 없으면 `+`로 해석)
- **AND/OR**:
  - 서로 다른 `tag` 키, 그리고 일반(비태그) 토큰들은 **AND**
  - 같은 `tag` 키에서 여러 `+value`는 **OR**
  - 최상위 OR는 단독 토큰 `|`로 가지를 나눕니다(예: `level:WARN | timeout`)
- **예약 태그**:
  - `level:<V|D|I|W|E|F>` — adb/bracket 포맷에서 추출된 심각도
  - `anomaly:<kind>` — 분류기가 탐지한 이상(예: `anomaly:anr`, `anomaly:fatal_java`, `anomaly:native_crash_header`). 와일드카드 `anomaly:any`는 **어떤 종류든** finding이 있는 줄을 매칭
  - 대문자 `A` 키는 `anomaly:any` 를 입력하지 않고도 같은 효과 (anomaly-only 토글)

정확한 규칙은 PRD의 **§7(필터 문법)** 를 참고하세요.

## AI-assisted analysis

`logsee`는 TUI 뷰어 외에 **헤드리스 이상탐지(anomaly detection)** 경로를 제공합니다.
Android adb system 로그에서 ANR · native tombstone · Java FATAL 같은 이벤트를 Tier A 규칙과 블록 파서로 찾아내고, LLM agent가 바로 먹을 수 있는 JSON / MCP 형식으로 내놓습니다.

### `--export-anomalies` (headless JSONL)

TUI 없이 파일/파이프를 한 번 훑어서 감지된 Finding·Span을 JSONL로 stdout에 씁니다.

```bash
# file 입력
./bin/logsee --export-anomalies adb.log > anomalies.jsonl

# 파이프 입력 (adb logcat 직접)
adb logcat -v threadtime | ./bin/logsee --export-anomalies -

# Claude/CLI LLM과 즉시 합치기
./bin/logsee --export-anomalies adb.log \
  | jq 'select(.type=="span") | .span' \
  | claude -p "이 Android 이벤트의 근본 원인을 1줄로 요약해줘"
```

출력 스키마(한 줄 당 하나의 JSON 객체):

```jsonc
{"type":"finding","finding":{"kind":"anr","seq":41,"severity":"E","confidence":1,
                              "fields":{"pid":"1245","tag":"ActivityManager"},"schema_version":1}}
{"type":"span","span":{"id":0,"kind":"anr","start_seq":41,"end_seq":58,"pid":1245,
                        "summary":"ANR in com.example.app (com.example.app/.MainActivity)","schema_version":1}}
```

### `logsee mcp` (Model Context Protocol, stdio)

Claude Code 같은 MCP 클라이언트가 직접 호출할 수 있는 JSON-RPC 2.0 서버입니다.
제공 도구:

| tool | 용도 |
|---|---|
| `load_session(path)` | 파일을 파이프라인에 태우고 `session_id` 반환 |
| `list_anomalies(session_id, kinds?)` | 감지된 Finding + Span 배열 |
| `get_event(session_id, span_id)` | Span에 해당하는 전체 라인 + 요약 |
| `summarize_pid(session_id, pid)` | 특정 PID 관련 Finding + Span |

`~/.claude/settings.json` 에 등록:

```jsonc
{
  "mcpServers": {
    "logsee": {
      "command": "/absolute/path/to/logsee",
      "args": ["mcp"]
    }
  }
}
```

등록 후 Claude Code에서 자연어로 호출 가능: _"`/path/to/adb.log` 열어서 ANR 리포트 확인해줘"_ 등.

자세한 설계·구현 단계는 [`docs/architecture/anomaly-detection.md`](docs/architecture/anomaly-detection.md) 와 [`docs/plans/anomaly-detection-plan.md`](docs/plans/anomaly-detection-plan.md) 를 참고하세요.
예제 Claude Code 시나리오는 [`examples/mcp-claude.md`](examples/mcp-claude.md).

## Dev

```bash
make help
make fmt
make vet
make tidy
```
