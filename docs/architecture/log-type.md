# Log type (`--log-type`)

## Why

Reserved filter tag `level:` needs a consistent way to extract severity from a line. Log shapes differ (plain text, Android logcat, etc.). The user picks a **log type** at startup or uses **`auto`** to probe the oldest non-empty lines in the ring.

## Mapping

| CLI | `filter.LogFormat` | Level extraction |
|-----|--------------------|------------------|
| `plain` | `FormatPlain` | None (structured patterns disabled). |
| `adb` | `FormatAndroid` | Single-letter V/D/I/W/E/F (see PRD §7.1). |
| `auto` | Resolved once to Android or Plain | Probe scores android vs bracket lines; Android wins only when android score is strictly higher, otherwise **Plain** (`EffectiveFormatFromDetect`). |

`FormatUnknown` and `FormatBracket` remain for legacy parser compatibility in `ExtractRawLevel`; **`auto`** resolution does not leave the session on these values — it becomes **`FormatAndroid`** or **`FormatPlain`**.

## UI

`Model` caches `effectiveLogFmt` and `logFormatResolved`. Status bar shows `type:…` (see PRD §5).

## Config

- Path: `$HOME/.local/logsee/config.toml` (or `--config` override); format is TOML. Example: repo root `config.example.toml` or `logsee --print-default-config`.
- Keys:
  - `log_type.default` (`auto|plain|adb`)
  - `log_type.probe_lines`
  - `log_type.patterns` (`adb_head_time`, `adb_head_threadtime`, `bracket_head`)
- Precedence: **CLI > config > defaults**
- `state.json` remains history-only.
