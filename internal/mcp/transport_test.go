package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockHandler implements MethodHandler for testing transport.
type mockHandler struct {
	handleFn  func(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError)
	info      ServerInfo
	caps      ServerCapabilities
}

func (m *mockHandler) HandleMethod(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError) {
	if m.handleFn != nil {
		return m.handleFn(ctx, method, params)
	}
	return map[string]string{"ok": "true"}, nil
}

func (m *mockHandler) ServerInfo() ServerInfo {
	return m.info
}

func (m *mockHandler) Capabilities() ServerCapabilities {
	return m.caps
}

func newTestTransport(handler MethodHandler) *Transport {
	return NewTransport(handler)
}

func TestTransportPOSTBasicRequest(t *testing.T) {
	handler := &mockHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			if method != "tools/list" {
				t.Errorf("method: got %q, want %q", method, "tools/list")
			}
			return map[string][]string{"tools": {}}, nil
		},
	}
	transport := newTestTransport(handler)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q", ct)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if rpcResp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc: got %q", rpcResp.JSONRPC)
	}
	if rpcResp.Error != nil {
		t.Errorf("unexpected error: %v", rpcResp.Error)
	}
}

func TestTransportPOSTWithErrorResult(t *testing.T) {
	handler := &mockHandler{
		handleFn: func(_ context.Context, _ string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			return nil, NewMethodNotFound("no such method")
		},
	}
	transport := newTestTransport(handler)

	body := `{"jsonrpc":"2.0","id":1,"method":"foo/bar"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rpcResp.Error == nil {
		t.Fatal("expected error response")
	}
	if rpcResp.Error.Code != MethodNotFound {
		t.Errorf("error code: got %d, want %d", rpcResp.Error.Code, MethodNotFound)
	}
}

func TestTransportPOSTMalformedJSON(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	body := `{not valid json`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rpcResp.Error == nil {
		t.Fatal("expected parse error")
	}
	if rpcResp.Error.Code != ParseError {
		t.Errorf("error code: got %d, want %d", rpcResp.Error.Code, ParseError)
	}
}

func TestTransportPOSTMissingMethod(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	body := `{"jsonrpc":"2.0","id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	respBody, _ := io.ReadAll(w.Result().Body)
	var rpcResp JSONRPCResponse
	json.Unmarshal(respBody, &rpcResp)
	if rpcResp.Error == nil {
		t.Fatal("expected invalid request error")
	}
	if rpcResp.Error.Code != InvalidRequest {
		t.Errorf("error code: got %d, want %d", rpcResp.Error.Code, InvalidRequest)
	}
}

func TestTransportPOSTWrongJSONRPCVersion(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	body := `{"jsonrpc":"1.0","id":1,"method":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	respBody, _ := io.ReadAll(w.Result().Body)
	var rpcResp JSONRPCResponse
	json.Unmarshal(respBody, &rpcResp)
	if rpcResp.Error == nil {
		t.Fatal("expected invalid request error for wrong version")
	}
	if rpcResp.Error.Code != InvalidRequest {
		t.Errorf("error code: got %d, want %d", rpcResp.Error.Code, InvalidRequest)
	}
}

func TestTransportPOSTWrongContentType(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	body := `{"jsonrpc":"2.0","id":1,"method":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusUnsupportedMediaType)
	}
}

func TestTransportGETReturns405(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1", nil)
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestTransportDELETEReturns204(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	req := httptest.NewRequest(http.MethodDelete, "/mcp/v1", nil)
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestTransportBatchRequest(t *testing.T) {
	callCount := 0
	handler := &mockHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			callCount++
			return map[string]string{"method": method}, nil
		},
	}
	transport := newTestTransport(handler)

	body := `[
		{"jsonrpc":"2.0","id":1,"method":"tools/list"},
		{"jsonrpc":"2.0","id":2,"method":"resources/list"}
	]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResps []JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResps); err != nil {
		t.Fatalf("unmarshal batch: %v", err)
	}
	if len(rpcResps) != 2 {
		t.Fatalf("batch count: got %d, want 2", len(rpcResps))
	}
	if callCount != 2 {
		t.Errorf("handler call count: got %d, want 2", callCount)
	}
}

func TestTransportEmptyBatchRequest(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	body := `[]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	respBody, _ := io.ReadAll(w.Result().Body)
	var rpcResp JSONRPCResponse
	json.Unmarshal(respBody, &rpcResp)
	if rpcResp.Error == nil {
		t.Fatal("expected error for empty batch")
	}
	if rpcResp.Error.Code != InvalidRequest {
		t.Errorf("error code: got %d, want %d", rpcResp.Error.Code, InvalidRequest)
	}
}

func TestTransportNotificationNoResponse(t *testing.T) {
	called := false
	handler := &mockHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			called = true
			return nil, nil
		},
	}
	transport := newTestTransport(handler)

	// Notification: no "id" field
	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if !called {
		t.Error("handler should still be called for notifications")
	}
}

func TestTransportSessionIDHeader(t *testing.T) {
	handler := &mockHandler{
		handleFn: func(_ context.Context, _ string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			return map[string]string{"initialized": "true"}, nil
		},
	}
	store := NewSessionStore()
	transport := NewTransportWithSessions(handler, store)

	// Send initialize request (should create session)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("expected Mcp-Session-Id header on initialize response")
	}

	// Subsequent request with session ID should work
	body2 := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req2 := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Mcp-Session-Id", sessionID)
	w2 := httptest.NewRecorder()

	transport.ServeHTTP(w2, req2)

	if w2.Result().StatusCode != http.StatusOK {
		t.Errorf("subsequent request status: got %d", w2.Result().StatusCode)
	}
}

func TestTransportBodySizeLimit(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	// Create a body larger than 1MB
	bigBody := `{"jsonrpc":"2.0","id":1,"method":"test","params":{"data":"` + strings.Repeat("x", 1<<20+1) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	resp := w.Result()
	// Should either reject or return parse error
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", resp.StatusCode)
	}
}

func TestTransportPUTReturns405(t *testing.T) {
	handler := &mockHandler{}
	transport := newTestTransport(handler)

	req := httptest.NewRequest(http.MethodPut, "/mcp/v1", nil)
	w := httptest.NewRecorder()

	transport.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want %d", w.Result().StatusCode, http.StatusMethodNotAllowed)
	}
}
