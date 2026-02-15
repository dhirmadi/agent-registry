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

// ============================================================================
// QA FUNCTIONAL TEST SUITE — JSON-RPC 2.0 Protocol Compliance
// ============================================================================
// Tests validate the transport layer against JSON-RPC 2.0 specification
// (https://www.jsonrpc.org/specification) and MCP Streamable HTTP transport.

// --- Test helper: mock handler ---

type qaHandler struct {
	handleFn func(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError)
}

func (q *qaHandler) HandleMethod(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError) {
	if q.handleFn != nil {
		return q.handleFn(ctx, method, params)
	}
	return map[string]interface{}{}, nil
}

func (q *qaHandler) ServerInfo() ServerInfo {
	return ServerInfo{Name: "test-server", Version: "1.0.0"}
}

func (q *qaHandler) Capabilities() ServerCapabilities {
	return ServerCapabilities{
		Tools:     &ToolsCapability{ListChanged: false},
		Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
		Prompts:   &PromptsCapability{ListChanged: false},
	}
}

func qaPost(t *testing.T, transport *Transport, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)
	return w
}

func qaParseResponse(t *testing.T, w *httptest.ResponseRecorder) JSONRPCResponse {
	t.Helper()
	body, _ := io.ReadAll(w.Body)
	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to parse response: %v\nbody: %s", err, string(body))
	}
	return resp
}

func qaParseBatchResponse(t *testing.T, w *httptest.ResponseRecorder) []JSONRPCResponse {
	t.Helper()
	body, _ := io.ReadAll(w.Body)
	var resps []JSONRPCResponse
	if err := json.Unmarshal(body, &resps); err != nil {
		t.Fatalf("failed to parse batch response: %v\nbody: %s", err, string(body))
	}
	return resps
}

// ============================================================================
// SECTION 1: JSON-RPC 2.0 Request Validation
// ============================================================================

func TestQA_JSONRPC_VersionMustBe2_0(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid 2.0", "2.0", false},
		{"wrong 1.0", "1.0", true},
		{"wrong 3.0", "3.0", true},
		{"empty", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jsonrpc":"` + tc.version + `","id":1,"method":"ping"}`
			if tc.version == "" {
				body = `{"jsonrpc":"","id":1,"method":"ping"}`
			}
			w := qaPost(t, transport, body)
			resp := qaParseResponse(t, w)

			if tc.wantErr {
				if resp.Error == nil {
					t.Fatal("expected error for invalid jsonrpc version")
				}
				if resp.Error.Code != InvalidRequest {
					t.Errorf("error code = %d, want %d", resp.Error.Code, InvalidRequest)
				}
			} else {
				if resp.Error != nil {
					t.Errorf("unexpected error: %v", resp.Error)
				}
			}
		})
	}
}

func TestQA_JSONRPC_MethodRequired(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"with method", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, false},
		{"empty method", `{"jsonrpc":"2.0","id":1,"method":""}`, true},
		{"missing method field", `{"jsonrpc":"2.0","id":1}`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := qaPost(t, transport, tc.body)
			resp := qaParseResponse(t, w)
			if tc.wantErr && resp.Error == nil {
				t.Fatal("expected error for missing/empty method")
			}
			if !tc.wantErr && resp.Error != nil {
				t.Errorf("unexpected error: %v", resp.Error)
			}
		})
	}
}

func TestQA_JSONRPC_IDTypes(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	tests := []struct {
		name string
		body string
	}{
		{"integer ID", `{"jsonrpc":"2.0","id":1,"method":"ping"}`},
		{"string ID", `{"jsonrpc":"2.0","id":"abc-123","method":"ping"}`},
		{"null ID", `{"jsonrpc":"2.0","id":null,"method":"ping"}`},
		{"large integer ID", `{"jsonrpc":"2.0","id":999999999,"method":"ping"}`},
		{"negative integer ID", `{"jsonrpc":"2.0","id":-1,"method":"ping"}`},
		{"zero ID", `{"jsonrpc":"2.0","id":0,"method":"ping"}`},
		{"empty string ID", `{"jsonrpc":"2.0","id":"","method":"ping"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := qaPost(t, transport, tc.body)
			resp := qaParseResponse(t, w)
			if resp.Error != nil {
				t.Errorf("unexpected error for valid ID type: %v", resp.Error)
			}
			if resp.JSONRPC != "2.0" {
				t.Errorf("response jsonrpc = %q, want 2.0", resp.JSONRPC)
			}
		})
	}
}

func TestQA_JSONRPC_ResponseAlwaysHasJSONRPC2_0(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	// Success response
	w := qaPost(t, transport, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	resp := qaParseResponse(t, w)
	if resp.JSONRPC != "2.0" {
		t.Errorf("success response jsonrpc = %q, want 2.0", resp.JSONRPC)
	}

	// Error response
	w = qaPost(t, transport, `{invalid`)
	resp = qaParseResponse(t, w)
	if resp.JSONRPC != "2.0" {
		t.Errorf("error response jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
}

func TestQA_JSONRPC_ResponseIDMatchesRequest(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	tests := []struct {
		name   string
		body   string
		wantID string
	}{
		{"integer 1", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "1"},
		{"integer 42", `{"jsonrpc":"2.0","id":42,"method":"ping"}`, "42"},
		{"string abc", `{"jsonrpc":"2.0","id":"abc","method":"ping"}`, `"abc"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := qaPost(t, transport, tc.body)
			resp := qaParseResponse(t, w)
			if string(resp.ID) != tc.wantID {
				t.Errorf("response ID = %s, want %s", string(resp.ID), tc.wantID)
			}
		})
	}
}

func TestQA_JSONRPC_ResponseContentType(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	w := qaPost(t, transport, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ============================================================================
// SECTION 2: JSON-RPC 2.0 Error Codes
// ============================================================================

func TestQA_JSONRPC_ErrorCodes(t *testing.T) {
	handler := &qaHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			if method == "unknown" {
				return nil, NewMethodNotFound("unknown method")
			}
			return nil, nil
		},
	}
	transport := NewTransport(handler)

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			"parse error: malformed JSON",
			`{invalid`,
			ParseError,
		},
		{
			"invalid request: wrong version",
			`{"jsonrpc":"1.0","id":1,"method":"test"}`,
			InvalidRequest,
		},
		{
			"invalid request: missing method",
			`{"jsonrpc":"2.0","id":1}`,
			InvalidRequest,
		},
		{
			"method not found",
			`{"jsonrpc":"2.0","id":1,"method":"unknown"}`,
			MethodNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := qaPost(t, transport, tc.body)
			resp := qaParseResponse(t, w)
			if resp.Error == nil {
				t.Fatal("expected error")
			}
			if resp.Error.Code != tc.wantCode {
				t.Errorf("error code = %d, want %d", resp.Error.Code, tc.wantCode)
			}
			if resp.Error.Message == "" {
				t.Error("error message should not be empty")
			}
		})
	}
}

func TestQA_JSONRPC_ErrorResponseHasNoResult(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	w := qaPost(t, transport, `{invalid`)
	body, _ := io.ReadAll(w.Body)

	var raw map[string]interface{}
	json.Unmarshal(body, &raw)

	if _, ok := raw["result"]; ok {
		t.Error("error response must not have 'result' field")
	}
	if _, ok := raw["error"]; !ok {
		t.Error("error response must have 'error' field")
	}
}

func TestQA_JSONRPC_SuccessResponseHasNoError(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	w := qaPost(t, transport, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	body, _ := io.ReadAll(w.Body)

	var raw map[string]interface{}
	json.Unmarshal(body, &raw)

	if _, ok := raw["result"]; !ok {
		t.Error("success response must have 'result' field")
	}
	if errVal, ok := raw["error"]; ok && errVal != nil {
		t.Error("success response must not have 'error' field (or it must be null)")
	}
}

// ============================================================================
// SECTION 3: HTTP Method Restrictions
// ============================================================================

func TestQA_HTTP_MethodRestrictions(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	methods := []struct {
		method     string
		wantStatus int
	}{
		{http.MethodPost, http.StatusOK},
		{http.MethodDelete, http.StatusNoContent},
		{http.MethodGet, http.StatusMethodNotAllowed},
		{http.MethodPut, http.StatusMethodNotAllowed},
		{http.MethodPatch, http.StatusMethodNotAllowed},
		{http.MethodOptions, http.StatusMethodNotAllowed},
		{http.MethodHead, http.StatusMethodNotAllowed},
	}

	for _, tc := range methods {
		t.Run(tc.method, func(t *testing.T) {
			var req *http.Request
			if tc.method == http.MethodPost {
				req = httptest.NewRequest(tc.method, "/mcp/v1",
					strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, "/mcp/v1", nil)
			}
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

func TestQA_HTTP_AllowHeader_On405(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1", nil)
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	allow := w.Header().Get("Allow")
	if allow == "" {
		t.Error("405 response should include Allow header")
	}
	if !strings.Contains(allow, "POST") {
		t.Errorf("Allow header should include POST: %q", allow)
	}
	if !strings.Contains(allow, "DELETE") {
		t.Errorf("Allow header should include DELETE: %q", allow)
	}
}

func TestQA_HTTP_ContentType_Required(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	tests := []struct {
		name     string
		ct       string
		wantFail bool
	}{
		{"application/json", "application/json", false},
		{"application/json; charset=utf-8", "application/json; charset=utf-8", false},
		{"text/plain", "text/plain", true},
		{"text/html", "text/html", true},
		{"application/xml", "application/xml", true},
		{"no content type", "", true},
		{"multipart/form-data", "multipart/form-data", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp/v1",
				strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			if tc.ct != "" {
				req.Header.Set("Content-Type", tc.ct)
			}
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)

			if tc.wantFail {
				if w.Code != http.StatusUnsupportedMediaType {
					t.Errorf("status = %d, want 415", w.Code)
				}
			} else {
				if w.Code == http.StatusUnsupportedMediaType {
					t.Error("should accept this content type")
				}
			}
		})
	}
}

// ============================================================================
// SECTION 4: Body Size Limit
// ============================================================================

func TestQA_BodySizeLimit(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	tests := []struct {
		name     string
		size     int
		wantFail bool
	}{
		{"small body (100 bytes)", 100, false},
		{"medium body (10KB)", 10 * 1024, false},
		{"near limit (1MB - 1)", 1<<20 - 100, false},
		{"over limit (1MB + 1)", 1<<20 + 1, true},
		{"way over limit (2MB)", 2 << 20, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build a request with a large padding field
			prefix := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"data":"`
			suffix := `"}}`
			padLen := tc.size - len(prefix) - len(suffix)
			if padLen < 0 {
				padLen = 0
			}
			body := prefix + strings.Repeat("x", padLen) + suffix

			req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)

			if tc.wantFail {
				if w.Code != http.StatusRequestEntityTooLarge {
					t.Errorf("status = %d, want 413 for oversized body", w.Code)
				}
			} else {
				// Should succeed or return a parse/method error (not 413)
				if w.Code == http.StatusRequestEntityTooLarge {
					t.Error("should not reject body under limit")
				}
			}
		})
	}
}

// ============================================================================
// SECTION 5: Notifications (no ID = no response body)
// ============================================================================

func TestQA_Notification_NoResponseBody(t *testing.T) {
	called := false
	handler := &qaHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			called = true
			return nil, nil
		},
	}
	transport := NewTransport(handler)

	// Notification = request without "id" field
	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("notification status = %d, want 204", w.Code)
	}
	if !called {
		t.Error("handler should be called even for notifications")
	}
	respBody, _ := io.ReadAll(w.Body)
	if len(respBody) > 0 {
		t.Errorf("notification should have empty body, got %d bytes", len(respBody))
	}
}

func TestQA_Notification_InBatch_ExcludedFromResponse(t *testing.T) {
	handler := &qaHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			return map[string]string{"ok": "true"}, nil
		},
	}
	transport := NewTransport(handler)

	body := `[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","method":"notifications/initialized"},
		{"jsonrpc":"2.0","id":2,"method":"ping"}
	]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resps := qaParseBatchResponse(t, w)
	// Only 2 responses (the notification should be excluded)
	if len(resps) != 2 {
		t.Errorf("batch response count = %d, want 2 (notification excluded)", len(resps))
	}
}

// ============================================================================
// SECTION 6: Batch Requests
// ============================================================================

func TestQA_Batch_SingleRequest(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	body := `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resps := qaParseBatchResponse(t, w)
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Errorf("unexpected error: %v", resps[0].Error)
	}
}

func TestQA_Batch_MixedSuccessAndError(t *testing.T) {
	handler := &qaHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			if method == "fail" {
				return nil, NewMethodNotFound("no such method: fail")
			}
			return map[string]string{}, nil
		},
	}
	transport := NewTransport(handler)

	body := `[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","id":2,"method":"fail"},
		{"jsonrpc":"2.0","id":3,"method":"ping"}
	]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resps := qaParseBatchResponse(t, w)
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Error("first request should succeed")
	}
	if resps[1].Error == nil {
		t.Error("second request should fail")
	}
	if resps[2].Error != nil {
		t.Error("third request should succeed")
	}
}

func TestQA_Batch_Empty(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	body := `[]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resp := qaParseResponse(t, w)
	if resp.Error == nil {
		t.Fatal("empty batch should return error")
	}
	if resp.Error.Code != InvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, InvalidRequest)
	}
}

func TestQA_Batch_InvalidElement(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	// A batch where one element has bad jsonrpc version
	body := `[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"1.0","id":2,"method":"ping"}
	]`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resps := qaParseBatchResponse(t, w)
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	// First should succeed
	if resps[0].Error != nil {
		t.Error("first request should succeed")
	}
	// Second should fail with InvalidRequest
	if resps[1].Error == nil {
		t.Fatal("second request should fail")
	}
	if resps[1].Error.Code != InvalidRequest {
		t.Errorf("error code = %d, want %d", resps[1].Error.Code, InvalidRequest)
	}
}

// ============================================================================
// SECTION 7: Edge Cases
// ============================================================================

func TestQA_EmptyBody(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resp := qaParseResponse(t, w)
	if resp.Error == nil {
		t.Fatal("empty body should return error")
	}
	if resp.Error.Code != ParseError {
		t.Errorf("error code = %d, want %d (ParseError)", resp.Error.Code, ParseError)
	}
}

func TestQA_WhitespaceOnlyBody(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader("   \n\t  "))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	resp := qaParseResponse(t, w)
	if resp.Error == nil {
		t.Fatal("whitespace-only body should return error")
	}
}

func TestQA_ExtraFieldsInRequest(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	// JSON-RPC 2.0 allows extra fields — they should be ignored
	body := `{"jsonrpc":"2.0","id":1,"method":"ping","extra_field":"value","another":42}`
	w := qaPost(t, transport, body)
	resp := qaParseResponse(t, w)
	if resp.Error != nil {
		t.Errorf("extra fields should be ignored, got error: %v", resp.Error)
	}
}

func TestQA_UnicodeInMethod(t *testing.T) {
	handler := &qaHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			return nil, NewMethodNotFound("unknown method: " + method)
		},
	}
	transport := NewTransport(handler)

	body := `{"jsonrpc":"2.0","id":1,"method":"tëst/méthöd"}`
	w := qaPost(t, transport, body)
	resp := qaParseResponse(t, w)
	if resp.Error == nil {
		t.Fatal("expected MethodNotFound error")
	}
	if resp.Error.Code != MethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, MethodNotFound)
	}
}

func TestQA_NullParams(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":null}`
	w := qaPost(t, transport, body)
	resp := qaParseResponse(t, w)
	if resp.Error != nil {
		t.Errorf("null params should be accepted: %v", resp.Error)
	}
}

func TestQA_MissingParams(t *testing.T) {
	transport := NewTransport(&qaHandler{})

	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	w := qaPost(t, transport, body)
	resp := qaParseResponse(t, w)
	if resp.Error != nil {
		t.Errorf("missing params should be accepted: %v", resp.Error)
	}
}

// ============================================================================
// SECTION 8: Session Management
// ============================================================================

func TestQA_Session_InitializeCreatesSession(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"qa-test","version":"1.0"}}}`
	w := qaPost(t, transport, body)

	sid := w.Header().Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatal("initialize should return Mcp-Session-Id header")
	}
	if len(sid) != 64 {
		t.Errorf("session ID length = %d, want 64 (32 bytes hex)", len(sid))
	}

	// Verify session exists in store
	_, ok := sessions.GetSession(sid)
	if !ok {
		t.Error("session should be stored after initialize")
	}
}

func TestQA_Session_InitializeResultFields(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"qa-test","version":"1.0"}}}`
	w := qaPost(t, transport, body)
	resp := qaParseResponse(t, w)

	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}

	var result InitializeResult
	json.Unmarshal(resp.Result, &result)

	if result.ProtocolVersion != "2025-03-26" {
		t.Errorf("protocolVersion = %q, want 2025-03-26", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("serverInfo.name = %q", result.ServerInfo.Name)
	}
	if result.ServerInfo.Version != "1.0.0" {
		t.Errorf("serverInfo.version = %q", result.ServerInfo.Version)
	}
	if result.Capabilities.Tools == nil {
		t.Error("tools capability should be present")
	}
	if result.Capabilities.Resources == nil {
		t.Error("resources capability should be present")
	}
	if result.Capabilities.Prompts == nil {
		t.Error("prompts capability should be present")
	}
}

func TestQA_Session_DeleteRemovesSession(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	// Initialize to get a session
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"qa-test","version":"1.0"}}}`
	w := qaPost(t, transport, body)
	sid := w.Header().Get("Mcp-Session-Id")

	// DELETE to terminate session
	req := httptest.NewRequest(http.MethodDelete, "/mcp/v1", nil)
	req.Header.Set("Mcp-Session-Id", sid)
	w2 := httptest.NewRecorder()
	transport.ServeHTTP(w2, req)

	if w2.Code != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", w2.Code)
	}

	// Verify session is gone
	_, ok := sessions.GetSession(sid)
	if ok {
		t.Error("session should be deleted after DELETE")
	}
}

func TestQA_Session_DeleteNonexistentSessionNoError(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	req := httptest.NewRequest(http.MethodDelete, "/mcp/v1", nil)
	req.Header.Set("Mcp-Session-Id", "nonexistent-session-id")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("DELETE nonexistent session status = %d, want 204", w.Code)
	}
}

func TestQA_Session_MultipleInitializesCreateDifferentSessions(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"qa-test","version":"1.0"}}}`

	w1 := qaPost(t, transport, body)
	sid1 := w1.Header().Get("Mcp-Session-Id")

	w2 := qaPost(t, transport, body)
	sid2 := w2.Header().Get("Mcp-Session-Id")

	if sid1 == sid2 {
		t.Error("different initialize calls should create different sessions")
	}
}

// ============================================================================
// SECTION 9: Initialize with various params
// ============================================================================

func TestQA_Initialize_EmptyParams(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	// Empty params object
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	w := qaPost(t, transport, body)
	resp := qaParseResponse(t, w)

	// Should succeed (fields are optional/defaults)
	if resp.Error != nil {
		t.Errorf("empty params should be accepted: %v", resp.Error)
	}
}

func TestQA_Initialize_WithCapabilities(t *testing.T) {
	handler := &qaHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
		"protocolVersion":"2025-03-26",
		"capabilities":{"roots":{"listChanged":true},"sampling":{}},
		"clientInfo":{"name":"Claude Desktop","version":"2.0"}
	}}`
	w := qaPost(t, transport, body)

	sid := w.Header().Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatal("should create session with capabilities")
	}

	session, ok := sessions.GetSession(sid)
	if !ok {
		t.Fatal("session not found")
	}
	if session.ClientCapabilities == nil {
		t.Fatal("client capabilities not stored")
	}
	if session.ClientCapabilities.Roots == nil {
		t.Fatal("roots capability not stored")
	}
	if !session.ClientCapabilities.Roots.ListChanged {
		t.Error("roots.listChanged should be true")
	}
}
