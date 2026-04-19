# Using `logsee mcp` with Claude Code

This walkthrough shows how to wire logsee's MCP server into Claude Code and use the four anomaly tools from a natural-language prompt.

## 1. Build

```bash
make build
# binary at ./bin/logsee
```

## 2. Register with Claude Code

Edit `~/.claude/settings.json` (or your project's `.claude/settings.json`):

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

Restart Claude Code. The four tools appear under `mcp__logsee__*`:

- `mcp__logsee__load_session`
- `mcp__logsee__list_anomalies`
- `mcp__logsee__get_event`
- `mcp__logsee__summarize_pid`

## 3. Verify from the shell

The server speaks newline-delimited JSON-RPC 2.0 over stdio, so you can smoke-test it without Claude Code:

```bash
$ printf '%s\n' \
    '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
    '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
    | ./bin/logsee mcp \
    | jq -c '.result | (.serverInfo // (.tools | map(.name)))'
{"name":"logsee-mcp","version":"1"}
["load_session","list_anomalies","get_event","summarize_pid"]
```

## 4. Example Claude Code prompt

With the server registered, ask Claude:

> 여기 `/tmp/adb_capture.log` 열어서 어떤 이상이 있었는지 요약해줘. ANR 있으면 해당 블록 라인도 보여줘.

Claude Code will:

1. Call `load_session({"path": "/tmp/adb_capture.log"})`.
   Server replies with counts and a `session_id`:
   ```json
   {"session_id":"s-1713571234-1","path":"/tmp/adb_capture.log","lines":106,"findings":1,"spans":1}
   ```
2. Call `list_anomalies({"session_id": "s-...1"})`.
   Receives findings + spans arrays; sees `kind: anr` span.
3. Call `get_event({"session_id": "s-...1", "span_id": 1})`.
   Receives the full line range (start..end) of the ANR block plus a summary
   like `"ANR in com.example.app (com.example.app/.MainActivity)"`.
4. Summarizes the root cause in natural language, citing line numbers.

## 5. Current tool coverage

| signature | detected as |
|---|---|
| `AndroidRuntime: FATAL EXCEPTION` | finding `fatal_java`, span `java_fatal` |
| `*** FATAL EXCEPTION IN SYSTEM PROCESS` | finding `fatal_java`, span `java_fatal` |
| `ActivityManager: ANR in …` | finding `anr`, span `anr` |
| `DEBUG: *** *** ***` (tombstone) | finding `native_crash_header`, span `native_crash` |
| `Watchdog: WATCHDOG KILLING …` | finding `watchdog` |
| `lowmemorykiller: Killing …` | finding `lmk_kill` |
| `FAILED BINDER TRANSACTION`, `TransactionTooLargeException` | finding `binder_fail` |
| `avc: denied` | finding `selinux_denied` |
| `Log.wtf`, `wtf_` | finding `wtf` |
| `OutOfMemoryError` | finding `oom` |

Adding a new Tier A rule = one entry in `internal/analysis/classify/rules.go`; adding a new block kind = one file in `internal/analysis/block/`. Both land without touching the MCP server.

## 6. Limitations (v1)

- Single session model holds state in memory for the lifetime of the stdio connection. Closing stdin drops all sessions.
- `load_session` is synchronous — for multi-hundred-MB adb captures it may take a few seconds. No progress events are emitted yet.
- adb threadtime is the only parsed format. Lines in other shapes yield `LevelUnknown` records and never match Tier A rules.
- Template mining (Drain) and baseline diff are out of scope for v1; planned follow-ups live in `docs/plans/anomaly-detection-plan.md` under "오픈 이슈".
