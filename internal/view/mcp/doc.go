// Package mcp implements a minimal Model Context Protocol (stdio, JSON-RPC
// 2.0) server over the logsee analysis pipeline. It exposes four tools —
// load_session, list_anomalies, get_event, summarize_pid — that let an
// AI agent such as Claude Code query detected anomalies in a log file
// without having to read the raw text line by line.
//
// The server keeps session state in-memory for the lifetime of the stdio
// connection; closing stdin terminates the server and drops all sessions.
package mcp
