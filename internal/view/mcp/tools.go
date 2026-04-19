package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// toolFn is the shape the server's dispatcher calls. Each tool receives
// its raw JSON arguments and returns a value to embed in the MCP
// tools/call result (or an error to wrap as an isError content block).
type toolFn func(ctx context.Context, args json.RawMessage) (any, error)

// lookupTool returns the handler registered for name.
func (s *Server) lookupTool(name string) (toolFn, bool) {
	switch name {
	case "load_session":
		return s.toolLoadSession, true
	case "list_anomalies":
		return s.toolListAnomalies, true
	case "get_event":
		return s.toolGetEvent, true
	case "summarize_pid":
		return s.toolSummarizePID, true
	}
	return nil, false
}

// initializeResult is what the server returns for `initialize`. MCP
// clients negotiate protocol version + capabilities here.
func initializeResult() any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "logsee-mcp",
			"version": "1",
		},
	}
}

// toolDef is one entry in tools/list.
type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolsListResult() any {
	return map[string]any{"tools": []toolDef{
		{
			Name:        "load_session",
			Description: "Run the anomaly pipeline over a log file and return a session_id that subsequent tools reference.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "absolute or relative path to the adb log file",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "list_anomalies",
			Description: "List detected Findings and Spans in a session. Optional kinds filter (e.g. [\"anr\",\"fatal_java\"]).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{"type": "string"},
					"kinds": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "optional list of Finding kinds to include",
					},
				},
				"required": []string{"session_id"},
			},
		},
		{
			Name:        "get_event",
			Description: "Return the full Line range of a Span plus the span's summary. Use for inspecting a complete anomaly block.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{"type": "string"},
					"span_id":    map[string]any{"type": "integer"},
				},
				"required": []string{"session_id", "span_id"},
			},
		},
		{
			Name:        "summarize_pid",
			Description: "Return Findings and Spans associated with a specific PID, in Seq order.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{"type": "string"},
					"pid":        map[string]any{"type": "integer"},
				},
				"required": []string{"session_id", "pid"},
			},
		},
	}}
}

// --- tool: load_session ---

type loadSessionArgs struct {
	Path string `json:"path"`
}

type loadSessionResult struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Lines     int64  `json:"lines"`
	Findings  int    `json:"findings"`
	Spans     int    `json:"spans"`
}

func (s *Server) toolLoadSession(ctx context.Context, raw json.RawMessage) (any, error) {
	var a loadSessionArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if a.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	sess, m, err := loadSession(ctx, a.Path)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return loadSessionResult{
		SessionID: sess.ID,
		Path:      sess.Path,
		Lines:     m.Lines,
		Findings:  len(sess.Findings),
		Spans:     len(sess.Spans),
	}, nil
}

// --- tool: list_anomalies ---

type listAnomaliesArgs struct {
	SessionID string   `json:"session_id"`
	Kinds     []string `json:"kinds,omitempty"`
}

func (s *Server) toolListAnomalies(_ context.Context, raw json.RawMessage) (any, error) {
	var a listAnomaliesArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	sess, err := s.session(a.SessionID)
	if err != nil {
		return nil, err
	}
	findings := sess.Findings
	if len(a.Kinds) > 0 {
		want := make(map[string]bool, len(a.Kinds))
		for _, k := range a.Kinds {
			want[k] = true
		}
		filtered := make([]domain.Finding, 0, len(sess.Findings))
		for _, f := range sess.Findings {
			if want[f.Kind.String()] {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}
	return map[string]any{
		"findings": findings,
		"spans":    sess.Spans,
	}, nil
}

// --- tool: get_event ---

type getEventArgs struct {
	SessionID string `json:"session_id"`
	SpanID    int64  `json:"span_id"`
}

func (s *Server) toolGetEvent(_ context.Context, raw json.RawMessage) (any, error) {
	var a getEventArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	sess, err := s.session(a.SessionID)
	if err != nil {
		return nil, err
	}
	sp, ok := sess.spanByID[a.SpanID]
	if !ok {
		return nil, fmt.Errorf("span_id %d not found in session %s", a.SpanID, a.SessionID)
	}
	lines := sess.store.Lines.Range(sp.StartSeq, sp.EndSeq)
	return map[string]any{
		"span":  sp,
		"lines": lines,
	}, nil
}

// --- tool: summarize_pid ---

type summarizePIDArgs struct {
	SessionID string `json:"session_id"`
	PID       int32  `json:"pid"`
}

func (s *Server) toolSummarizePID(_ context.Context, raw json.RawMessage) (any, error) {
	var a summarizePIDArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	sess, err := s.session(a.SessionID)
	if err != nil {
		return nil, err
	}

	var findings []domain.Finding
	pidStr := strconv.FormatInt(int64(a.PID), 10)
	for _, f := range sess.Findings {
		if f.Fields["pid"] == pidStr {
			findings = append(findings, f)
		}
	}

	var spans []domain.Span
	for _, sp := range sess.Spans {
		if sp.PID == a.PID {
			spans = append(spans, sp)
		}
	}
	return map[string]any{
		"pid":      a.PID,
		"findings": findings,
		"spans":    spans,
	}, nil
}
