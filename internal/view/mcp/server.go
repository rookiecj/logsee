package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Server is a single-connection stdio JSON-RPC 2.0 server. It holds
// sessions in memory for the lifetime of the connection.
type Server struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewServer returns a Server with no sessions. Pass the result's Serve
// method the stdio pair to handle one connection.
func NewServer() *Server {
	return &Server{sessions: map[string]*Session{}}
}

// Serve reads JSON-RPC messages (one per line) from r and writes
// responses to w. It returns when r reaches EOF or scanner error; ctx
// cancels an in-flight tool call.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(w)

	for scanner.Scan() {
		var req rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(errorResponse(nil, codeParseError, "parse error"))
			continue
		}
		resp, ok := s.dispatch(ctx, &req)
		if ok {
			_ = enc.Encode(resp)
		}
	}
	return scanner.Err()
}

// --- JSON-RPC wire types ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

func okResponse(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcErr{Code: code, Message: msg}}
}

// dispatch returns (response, shouldSend). Notifications never get a
// response even on error, so shouldSend=false.
func (s *Server) dispatch(ctx context.Context, req *rpcRequest) (rpcResponse, bool) {
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return okResponse(req.ID, initializeResult()), !isNotification
	case "notifications/initialized", "initialized":
		return rpcResponse{}, false
	case "ping":
		return okResponse(req.ID, map[string]any{}), !isNotification
	case "tools/list":
		return okResponse(req.ID, toolsListResult()), !isNotification
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		if isNotification {
			return rpcResponse{}, false
		}
		return errorResponse(req.ID, codeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method)), true
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolCall(ctx context.Context, req *rpcRequest) (rpcResponse, bool) {
	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, codeInvalidParams, "invalid tools/call params"), true
	}
	fn, ok := s.lookupTool(p.Name)
	if !ok {
		return errorResponse(req.ID, codeInvalidParams, "unknown tool: "+p.Name), true
	}
	result, err := fn(ctx, p.Arguments)
	if err != nil {
		return okResponse(req.ID, toolCallErrorPayload(err.Error())), true
	}
	return okResponse(req.ID, toolCallPayload(result)), true
}

func toolCallPayload(v any) map[string]any {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		b = []byte(fmt.Sprintf(`{"encode_error":%q}`, err.Error()))
	}
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": string(b)}},
	}
}

func toolCallErrorPayload(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": msg}},
		"isError": true,
	}
}
