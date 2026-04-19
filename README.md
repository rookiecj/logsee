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

정확한 규칙은 PRD의 **§7(필터 문법)** 를 참고하세요.

## Dev

```bash
make help
make fmt
make vet
make tidy
```
