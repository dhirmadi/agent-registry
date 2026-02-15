package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const maxBodySize = 1 << 20 // 1 MB

// Transport implements the MCP Streamable HTTP transport.
// It handles JSON-RPC 2.0 over HTTP POST with optional session management.
type Transport struct {
	handler  MethodHandler
	sessions *SessionStore
}

// NewTransport creates a Transport without session management.
func NewTransport(handler MethodHandler) *Transport {
	return &Transport{handler: handler}
}

// NewTransportWithSessions creates a Transport with session management.
func NewTransportWithSessions(handler MethodHandler, sessions *SessionStore) *Transport {
	return &Transport{handler: handler, sessions: sessions}
}

// ServeHTTP implements http.Handler.
func (t *Transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		t.handlePost(w, r)
	case http.MethodDelete:
		t.handleDelete(w, r)
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (t *Transport) handleDelete(w http.ResponseWriter, r *http.Request) {
	if t.sessions != nil {
		if sid := r.Header.Get("Mcp-Session-Id"); sid != "" {
			t.sessions.DeleteSession(sid)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (t *Transport) handlePost(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
	if err != nil {
		writeJSONRPCError(w, nil, NewParseError("failed to read request body"))
		return
	}
	if len(body) > maxBodySize {
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		return
	}

	// Determine if batch or single request.
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) == 0 {
		writeJSONRPCError(w, nil, NewParseError("empty request body"))
		return
	}

	if trimmed[0] == '[' {
		t.handleBatch(w, r, body)
		return
	}

	t.handleSingle(w, r, body)
}

func (t *Transport) handleSingle(w http.ResponseWriter, r *http.Request, body []byte) {
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, NewParseError("invalid JSON: "+err.Error()))
		return
	}

	if rpcErr := validateRequest(&req); rpcErr != nil {
		writeJSONRPCError(w, req.ID, rpcErr)
		return
	}

	// Handle initialize specially for session creation.
	if req.Method == "initialize" && t.sessions != nil {
		t.handleInitialize(w, r, &req)
		return
	}

	// Dispatch to handler.
	result, rpcErr := t.handler.HandleMethod(r.Context(), req.Method, req.Params)

	// Notifications get no response body.
	if req.IsNotification() {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if rpcErr != nil {
		writeJSONRPCError(w, req.ID, rpcErr)
		return
	}

	writeJSONRPCResult(w, req.ID, result)
}

func (t *Transport) handleInitialize(w http.ResponseWriter, _ *http.Request, req *JSONRPCRequest) {
	var params InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeJSONRPCError(w, req.ID, NewInvalidParams("invalid initialize params: "+err.Error()))
			return
		}
	}

	session, err := t.sessions.NewSession(&params.Capabilities)
	if err != nil {
		writeJSONRPCError(w, req.ID, NewInternalError("failed to create session"))
		return
	}

	result := InitializeResult{
		ProtocolVersion: "2025-03-26",
		Capabilities:    t.handler.Capabilities(),
		ServerInfo:      t.handler.ServerInfo(),
	}

	w.Header().Set("Mcp-Session-Id", session.ID)
	writeJSONRPCResult(w, req.ID, result)
}

func (t *Transport) handleBatch(w http.ResponseWriter, r *http.Request, body []byte) {
	var reqs []JSONRPCRequest
	if err := json.Unmarshal(body, &reqs); err != nil {
		writeJSONRPCError(w, nil, NewParseError("invalid JSON batch: "+err.Error()))
		return
	}

	if len(reqs) == 0 {
		writeJSONRPCError(w, nil, NewInvalidRequest("empty batch"))
		return
	}

	responses := make([]JSONRPCResponse, 0, len(reqs))
	for _, req := range reqs {
		if rpcErr := validateRequest(&req); rpcErr != nil {
			responses = append(responses, JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   rpcErr,
			})
			continue
		}

		result, rpcErr := t.handler.HandleMethod(r.Context(), req.Method, req.Params)

		// Skip notifications in batch response.
		if req.IsNotification() {
			continue
		}

		if rpcErr != nil {
			responses = append(responses, JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   rpcErr,
			})
			continue
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			responses = append(responses, JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   NewInternalError("failed to marshal result"),
			})
			continue
		}

		responses = append(responses, JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultJSON,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

func validateRequest(req *JSONRPCRequest) *JSONRPCError {
	if req.JSONRPC != "2.0" {
		return NewInvalidRequest("jsonrpc must be \"2.0\"")
	}
	if req.Method == "" {
		return NewInvalidRequest("method is required")
	}
	return nil
}

func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, rpcErr *JSONRPCError) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcErr,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result interface{}) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		writeJSONRPCError(w, id, NewInternalError("failed to marshal result"))
		return
	}
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultJSON,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
