package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// rpcCall sends one request through server.Serve and returns the single
// response (server.Serve exits at EOF of its reader so we feed it one
// line at a time).
func rpcCall(t *testing.T, srv *Server, body string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString(body)
	buf.WriteByte('\n')
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), &buf, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() == 0 {
		return nil
	}
	var resp map[string]any
	dec := json.NewDecoder(&out)
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode response: %v (raw=%q)", err, out.String())
	}
	return resp
}

func TestInitialize(t *testing.T) {
	srv := NewServer()
	resp := rpcCall(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if resp == nil || resp["result"] == nil {
		t.Fatalf("expected result, got %v", resp)
	}
	res := resp["result"].(map[string]any)
	if res["protocolVersion"] == nil {
		t.Error("protocolVersion missing")
	}
	srvInfo := res["serverInfo"].(map[string]any)
	if srvInfo["name"] != "logsee-mcp" {
		t.Errorf("serverInfo.name = %v, want logsee-mcp", srvInfo["name"])
	}
}

func TestToolsList(t *testing.T) {
	srv := NewServer()
	resp := rpcCall(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res := resp["result"].(map[string]any)
	tools := res["tools"].([]any)
	if len(tools) < 4 {
		t.Fatalf("expected at least 4 tools, got %d", len(tools))
	}
	seen := map[string]bool{}
	for _, tv := range tools {
		seen[tv.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"load_session", "list_anomalies", "get_event", "summarize_pid"} {
		if !seen[want] {
			t.Errorf("tools/list missing %s", want)
		}
	}
}

func TestPing(t *testing.T) {
	srv := NewServer()
	resp := rpcCall(t, srv, `{"jsonrpc":"2.0","id":42,"method":"ping"}`)
	if resp["error"] != nil {
		t.Errorf("ping errored: %v", resp["error"])
	}
}

func TestUnknownMethodErrors(t *testing.T) {
	srv := NewServer()
	resp := rpcCall(t, srv, `{"jsonrpc":"2.0","id":1,"method":"nonesuch"}`)
	if resp["error"] == nil {
		t.Fatal("expected error for unknown method")
	}
	errObj := resp["error"].(map[string]any)
	if int(errObj["code"].(float64)) != codeMethodNotFound {
		t.Errorf("code = %v, want %d", errObj["code"], codeMethodNotFound)
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	srv := NewServer()
	resp := rpcCall(t, srv, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if resp != nil {
		t.Errorf("notification should produce no response, got %v", resp)
	}
}

// --- Tool integration: load sample → list_anomalies → get_event ---

func TestLoadSession_ANRSample(t *testing.T) {
	srv := NewServer()
	sample := filepath.Join("..", "..", "..", "testdata", "android", "anr_input_dispatch.log")

	sid := loadSampleSession(t, srv, sample)
	if sid == "" {
		t.Fatal("load_session returned empty session id")
	}

	// list_anomalies should include an ANR finding and an ANR span.
	resp := rpcCall(t, srv, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_anomalies","arguments":{"session_id":%q}}}`,
		sid,
	))
	payload := extractPayload(t, resp)
	findings := payload["findings"].([]any)
	spans := payload["spans"].([]any)
	if len(findings) == 0 {
		t.Fatal("findings empty")
	}
	sawANRFinding := false
	for _, fv := range findings {
		f := fv.(map[string]any)
		if f["kind"] == "anr" {
			sawANRFinding = true
			break
		}
	}
	if !sawANRFinding {
		t.Error("expected an ANR finding")
	}
	sawANRSpan := false
	for _, sv := range spans {
		s := sv.(map[string]any)
		if s["kind"] == "anr" {
			sawANRSpan = true
			break
		}
	}
	if !sawANRSpan {
		t.Error("expected an ANR span")
	}

	// Pick the first span id and call get_event.
	spanID := int64(spans[0].(map[string]any)["id"].(float64))
	resp = rpcCall(t, srv, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_event","arguments":{"session_id":%q,"span_id":%d}}}`,
		sid, spanID,
	))
	payload = extractPayload(t, resp)
	span := payload["span"].(map[string]any)
	lines := payload["lines"].([]any)
	if int64(span["id"].(float64)) != spanID {
		t.Errorf("span id mismatch")
	}
	if len(lines) == 0 {
		t.Error("event lines empty; get_event should return the full block range")
	}
}

func TestListAnomalies_KindsFilter(t *testing.T) {
	srv := NewServer()
	sample := filepath.Join("..", "..", "..", "testdata", "android", "java_fatal_system_server.log")
	sid := loadSampleSession(t, srv, sample)

	resp := rpcCall(t, srv, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"list_anomalies","arguments":{"session_id":%q,"kinds":["anr"]}}}`,
		sid,
	))
	payload := extractPayload(t, resp)
	findings := payload["findings"].([]any)
	for _, fv := range findings {
		if k := fv.(map[string]any)["kind"]; k != "anr" {
			t.Errorf("kinds filter leak: got %v", k)
		}
	}
}

func TestGetEvent_UnknownSpanReturnsError(t *testing.T) {
	srv := NewServer()
	sample := filepath.Join("..", "..", "..", "testdata", "android", "native_tombstone.log")
	sid := loadSampleSession(t, srv, sample)

	resp := rpcCall(t, srv, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"get_event","arguments":{"session_id":%q,"span_id":9999}}}`,
		sid,
	))
	res := resp["result"].(map[string]any)
	if res["isError"] != true {
		t.Errorf("expected isError=true for unknown span, got %v", res)
	}
}

func TestLoadSession_MissingPathRejected(t *testing.T) {
	srv := NewServer()
	resp := rpcCall(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"load_session","arguments":{}}}`)
	res := resp["result"].(map[string]any)
	if res["isError"] != true {
		t.Errorf("missing path should be an isError result, got %v", res)
	}
}

// --- helpers ---

func loadSampleSession(t *testing.T, srv *Server, path string) string {
	t.Helper()
	resp := rpcCall(t, srv, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"load_session","arguments":{"path":%q}}}`,
		path,
	))
	payload := extractPayload(t, resp)
	sid, _ := payload["session_id"].(string)
	if sid == "" {
		t.Fatalf("load_session returned no session_id (payload=%v)", payload)
	}
	return sid
}

func extractPayload(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	if resp == nil {
		t.Fatal("nil response")
	}
	if errObj, ok := resp["error"]; ok && errObj != nil {
		t.Fatalf("rpc error: %v", errObj)
	}
	res, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", resp)
	}
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode tool text %q: %v", text, err)
	}
	return payload
}

// keep unused imports honest
var _ = strings.Contains
var _ = domain.Finding{}
