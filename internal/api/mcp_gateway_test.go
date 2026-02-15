package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/gateway"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mocks ---

type mockGatewayServerStore struct {
	server *store.MCPServer
	err    error
	list   []store.MCPServer
}

func (m *mockGatewayServerStore) GetByLabel(_ context.Context, label string) (*store.MCPServer, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.server != nil && m.server.Label == label {
		return m.server, nil
	}
	return nil, &notFoundErr{}
}

func (m *mockGatewayServerStore) List(_ context.Context) ([]store.MCPServer, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.list, nil
}

type notFoundErr struct{}

func (e *notFoundErr) Error() string { return "not found" }

type mockProxyForwarder struct {
	resp    *gateway.ProxyResponse
	err     error
	lastReq *gateway.ProxyRequest
}

func (m *mockProxyForwarder) Forward(_ context.Context, req gateway.ProxyRequest) (*gateway.ProxyResponse, error) {
	m.lastReq = &req
	return m.resp, m.err
}

type mockTrustDefaults struct {
	records []gateway.TrustDefaultRecord
}

func (m *mockTrustDefaults) List(_ context.Context) ([]gateway.TrustDefaultRecord, error) {
	return m.records, nil
}

type mockTrustRules struct {
	records []gateway.TrustRuleRecord
}

func (m *mockTrustRules) List(_ context.Context, _ uuid.UUID) ([]gateway.TrustRuleRecord, error) {
	return m.records, nil
}

type mockAgentTrust struct {
	overrides map[string]string
}

func (m *mockAgentTrust) GetTrustOverrides(_ context.Context, _ string) (map[string]string, error) {
	return m.overrides, nil
}

// safeAuditMock is a thread-safe audit mock for testing async audit calls.
type safeAuditMock struct {
	mu      sync.Mutex
	entries []*store.AuditEntry
}

func (m *safeAuditMock) Insert(_ context.Context, entry *store.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *safeAuditMock) getEntries() []*store.AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*store.AuditEntry, len(m.entries))
	copy(cp, m.entries)
	return cp
}

// --- Helpers ---

var testEncKey = []byte("12345678901234567890123456789012")

func newTestGatewayHandler(
	servers MCPGatewayServerStore,
	trustClassifier *gateway.TrustClassifier,
	cb *gateway.CircuitBreaker,
	forwarder Forwarder,
	rl *ratelimit.RateLimiter,
) *MCPGatewayHandler {
	return NewMCPGatewayHandler(servers, &safeAuditMock{}, trustClassifier, cb, forwarder, rl, testEncKey)
}

func newTestGatewayHandlerWithAudit(
	servers MCPGatewayServerStore,
	audit *safeAuditMock,
	trustClassifier *gateway.TrustClassifier,
	cb *gateway.CircuitBreaker,
	forwarder Forwarder,
	rl *ratelimit.RateLimiter,
) *MCPGatewayHandler {
	return NewMCPGatewayHandler(servers, audit, trustClassifier, cb, forwarder, rl, testEncKey)
}

func makeGatewayRequest(t *testing.T, handler http.HandlerFunc, serverLabel, toolName string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	} else {
		reqBody = []byte(`{"arguments":{}}`)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/proxy/"+serverLabel+"/tools/"+toolName, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("serverLabel", serverLabel)
	rctx.URLParams.Add("toolName", toolName)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = auth.ContextWithUser(ctx, uuid.New(), "admin", "session")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func parseGatewayEnvelope(t *testing.T, rr *httptest.ResponseRecorder) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("parse response: %v (body: %s)", err, rr.Body.String())
	}
	return env
}

func enabledMCPServer() *store.MCPServer {
	return &store.MCPServer{
		ID:             uuid.New(),
		Label:          "test-server",
		Endpoint:       "http://localhost:9999/mcp",
		AuthType:       "none",
		AuthCredential: "",
		IsEnabled:      true,
		CircuitBreaker: json.RawMessage(`{"fail_threshold":3,"open_duration_s":30}`),
	}
}

func encryptCredential(t *testing.T, plaintext string) string {
	t.Helper()
	encrypted, err := auth.Encrypt([]byte(plaintext), testEncKey)
	if err != nil {
		t.Fatalf("encrypt credential: %v", err)
	}
	return base64.StdEncoding.EncodeToString(encrypted)
}

// --- 1. Happy path ---

func TestGateway_UpstreamSuccess_NoAuth(t *testing.T) {
	srv := enabledMCPServer()
	servers := &mockGatewayServerStore{server: srv}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{"result":"ok"}`), Latency: 50 * time.Millisecond}}
	rl := ratelimit.NewRateLimiter()
	h := newTestGatewayHandler(servers, tc, cb, forwarder, rl)
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", map[string]interface{}{"arguments": map[string]string{"key": "value"}})
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	env := parseGatewayEnvelope(t, rr)
	if !env.Success {
		t.Error("expected success=true")
	}
}

func TestGateway_UpstreamSuccess_BearerAuth(t *testing.T) {
	srv := enabledMCPServer()
	srv.AuthType = "bearer"
	srv.AuthCredential = encryptCredential(t, "my-bearer-token")
	servers := &mockGatewayServerStore{server: srv}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{"result":"ok"}`), Latency: 10 * time.Millisecond}}
	rl := ratelimit.NewRateLimiter()
	h := newTestGatewayHandler(servers, tc, cb, forwarder, rl)
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if forwarder.lastReq == nil {
		t.Fatal("forwarder was not called")
	}
	if forwarder.lastReq.AuthType != "bearer" {
		t.Errorf("auth type = %q, want bearer", forwarder.lastReq.AuthType)
	}
	if forwarder.lastReq.AuthCredential != "my-bearer-token" {
		t.Errorf("credential = %q, want my-bearer-token", forwarder.lastReq.AuthCredential)
	}
}

func TestGateway_UpstreamSuccess_BasicAuth(t *testing.T) {
	srv := enabledMCPServer()
	srv.AuthType = "basic"
	srv.AuthCredential = encryptCredential(t, "dXNlcjpwYXNz")
	servers := &mockGatewayServerStore{server: srv}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{"result":"ok"}`), Latency: 10 * time.Millisecond}}
	rl := ratelimit.NewRateLimiter()
	h := newTestGatewayHandler(servers, tc, cb, forwarder, rl)
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if forwarder.lastReq.AuthType != "basic" {
		t.Errorf("auth type = %q, want basic", forwarder.lastReq.AuthType)
	}
	if forwarder.lastReq.AuthCredential != "dXNlcjpwYXNz" {
		t.Errorf("credential = %q, want dXNlcjpwYXNz", forwarder.lastReq.AuthCredential)
	}
}

// --- 2. Server errors ---

func TestGateway_ServerNotFound(t *testing.T) {
	servers := &mockGatewayServerStore{err: &notFoundErr{}}
	h := newTestGatewayHandler(servers, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "nonexistent", "some_tool", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
	env := parseGatewayEnvelope(t, rr)
	if env.Success {
		t.Error("expected success=false")
	}
}

func TestGateway_ServerDisabled(t *testing.T) {
	srv := enabledMCPServer()
	srv.IsEnabled = false
	servers := &mockGatewayServerStore{server: srv}
	h := newTestGatewayHandler(servers, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- 3. Circuit breaker ---

func TestGateway_CircuitBreakerOpen(t *testing.T) {
	srv := enabledMCPServer()
	servers := &mockGatewayServerStore{server: srv}
	cb := gateway.NewCircuitBreaker()
	cbCfg := gateway.CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 10 * time.Second}
	cb.RecordFailure("test-server", cbCfg)
	h := newTestGatewayHandler(servers, gateway.NewTrustClassifier(nil, nil, nil), cb, &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// --- 4. Trust policy ---

func TestGateway_TrustBlocked(t *testing.T) {
	srv := enabledMCPServer()
	defaults := &mockTrustDefaults{records: []gateway.TrustDefaultRecord{{ToolPattern: "*", Tier: "block", Priority: 1}}}
	tc := gateway.NewTrustClassifier(nil, defaults, nil)
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, tc, gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestGateway_TrustReview(t *testing.T) {
	srv := enabledMCPServer()
	defaults := &mockTrustDefaults{records: []gateway.TrustDefaultRecord{{ToolPattern: "*", Tier: "review", Priority: 1}}}
	tc := gateway.NewTrustClassifier(nil, defaults, nil)
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, tc, gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestGateway_TrustAutoAllowsExecution(t *testing.T) {
	srv := enabledMCPServer()
	defaults := &mockTrustDefaults{records: []gateway.TrustDefaultRecord{
		{ToolPattern: "*_read", Tier: "auto", Priority: 1},
		{ToolPattern: "*", Tier: "block", Priority: 2},
	}}
	tc := gateway.NewTrustClassifier(nil, defaults, nil)
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{}`), Latency: 1 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, tc, gateway.NewCircuitBreaker(), forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "file_read", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGateway_TrustWithWorkspaceAndAgent(t *testing.T) {
	srv := enabledMCPServer()
	wsID := uuid.New()
	rules := &mockTrustRules{records: []gateway.TrustRuleRecord{{ToolPattern: "dangerous_*", Tier: "block"}}}
	agentTrust := &mockAgentTrust{overrides: map[string]string{"dangerous_tool": "auto"}}
	tc := gateway.NewTrustClassifier(rules, nil, agentTrust)
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{}`), Latency: 1 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, tc, gateway.NewCircuitBreaker(), forwarder, ratelimit.NewRateLimiter())
	body := map[string]interface{}{"arguments": map[string]string{}, "workspace_id": wsID.String(), "agent_id": "agent-1"}
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "dangerous_tool", body)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (agent override), got %d", rr.Code)
	}
}

// --- 5. Rate limiting ---

func TestGateway_RateLimitExceeded(t *testing.T) {
	srv := enabledMCPServer()
	rl := ratelimit.NewRateLimiter()
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{}`), Latency: 1 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), forwarder, rl)

	userID := uuid.New()
	for i := 0; i < 61; i++ {
		reqBody := []byte(`{"arguments":{}}`)
		req := httptest.NewRequest(http.MethodPost, "/mcp/v1/proxy/test-server/tools/some_tool", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("serverLabel", "test-server")
		rctx.URLParams.Add("toolName", "some_tool")
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = auth.ContextWithUser(ctx, userID, "admin", "session")
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.ProxyToolCall(rr, req)

		if i == 60 && rr.Code != http.StatusTooManyRequests {
			t.Errorf("request #61: expected 429, got %d", rr.Code)
		}
	}
}

// --- 6. Credential errors ---

func TestGateway_CredentialDecodeFailure(t *testing.T) {
	srv := enabledMCPServer()
	srv.AuthType = "bearer"
	srv.AuthCredential = "!!!not-valid-base64!!!"
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
	env := parseGatewayEnvelope(t, rr)
	errObj := env.Error.(map[string]interface{})
	if errObj["message"] != "credential decode failed" {
		t.Errorf("error = %q, want 'credential decode failed'", errObj["message"])
	}
}

func TestGateway_CredentialDecryptFailure(t *testing.T) {
	srv := enabledMCPServer()
	srv.AuthType = "bearer"
	srv.AuthCredential = base64.StdEncoding.EncodeToString([]byte("short"))
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
	env := parseGatewayEnvelope(t, rr)
	errObj := env.Error.(map[string]interface{})
	if errObj["message"] != "credential decrypt failed" {
		t.Errorf("error = %q, want 'credential decrypt failed'", errObj["message"])
	}
}

// --- 7. Upstream errors ---

func TestGateway_UpstreamError_Returns502(t *testing.T) {
	srv := enabledMCPServer()
	forwarder := &mockProxyForwarder{err: context.DeadlineExceeded}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
}

func TestGateway_Upstream5xx_RecordsCircuitFailure(t *testing.T) {
	srv := enabledMCPServer()
	srv.CircuitBreaker = json.RawMessage(`{"fail_threshold":1,"open_duration_s":30}`)
	cb := gateway.NewCircuitBreaker()
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 500, Body: json.RawMessage(`{"error":"internal"}`), Latency: 100 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), cb, forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (forwarded), got %d", rr.Code)
	}
	if cb.State("test-server") != gateway.CircuitOpen {
		t.Error("expected circuit open after 5xx with threshold=1")
	}
}

func TestGateway_Upstream4xx_NoCircuitBreaker(t *testing.T) {
	srv := enabledMCPServer()
	srv.CircuitBreaker = json.RawMessage(`{"fail_threshold":1,"open_duration_s":30}`)
	cb := gateway.NewCircuitBreaker()
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 400, Body: json.RawMessage(`{"error":"bad"}`), Latency: 10 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), cb, forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if cb.State("test-server") != gateway.CircuitClosed {
		t.Error("expected circuit closed after 4xx")
	}
}

func TestGateway_UpstreamError_RecordsCircuitFailure(t *testing.T) {
	srv := enabledMCPServer()
	srv.CircuitBreaker = json.RawMessage(`{"fail_threshold":1,"open_duration_s":60}`)
	cb := gateway.NewCircuitBreaker()
	forwarder := &mockProxyForwarder{err: fmt.Errorf("connection refused")}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), cb, forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
	if cb.State("test-server") != gateway.CircuitOpen {
		t.Error("expected circuit open after connection error with threshold=1")
	}
}

func TestGateway_UpstreamSuccess_RecordsCircuitSuccess(t *testing.T) {
	srv := enabledMCPServer()
	cb := gateway.NewCircuitBreaker()
	cbCfg := gateway.CircuitBreakerConfig{FailThreshold: 3, OpenDuration: 30 * time.Second}
	cb.RecordFailure("test-server", cbCfg)
	cb.RecordFailure("test-server", cbCfg)
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{}`), Latency: 1 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), cb, forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if cb.State("test-server") != gateway.CircuitClosed {
		t.Error("expected circuit closed after success")
	}
}

// --- 8. Audit verification ---

func TestGateway_Audit_SuccessfulCall(t *testing.T) {
	srv := enabledMCPServer()
	audit := &safeAuditMock{}
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{}`), Latency: 1 * time.Millisecond}}
	h := newTestGatewayHandlerWithAudit(&mockGatewayServerStore{server: srv}, audit, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "my_tool", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	time.Sleep(100 * time.Millisecond)
	entries := audit.getEntries()
	if len(entries) == 0 {
		t.Fatal("expected audit entry")
	}
	if entries[0].Action != "gateway_tool_call" {
		t.Errorf("action = %q, want gateway_tool_call", entries[0].Action)
	}
	if entries[0].ResourceID != "test-server/my_tool" {
		t.Errorf("resource = %q, want test-server/my_tool", entries[0].ResourceID)
	}
}

func TestGateway_Audit_TrustDenied(t *testing.T) {
	srv := enabledMCPServer()
	audit := &safeAuditMock{}
	defaults := &mockTrustDefaults{records: []gateway.TrustDefaultRecord{{ToolPattern: "*", Tier: "block", Priority: 1}}}
	tc := gateway.NewTrustClassifier(nil, defaults, nil)
	h := newTestGatewayHandlerWithAudit(&mockGatewayServerStore{server: srv}, audit, tc, gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "blocked_tool", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	time.Sleep(100 * time.Millisecond)
	entries := audit.getEntries()
	if len(entries) == 0 {
		t.Fatal("expected audit entry for trust denied")
	}
	var details map[string]interface{}
	json.Unmarshal(entries[0].Details, &details)
	if details["outcome"] != "trust_denied" {
		t.Errorf("outcome = %q, want trust_denied", details["outcome"])
	}
}

func TestGateway_Audit_CircuitOpen(t *testing.T) {
	srv := enabledMCPServer()
	audit := &safeAuditMock{}
	cb := gateway.NewCircuitBreaker()
	cbCfg := gateway.CircuitBreakerConfig{FailThreshold: 1, OpenDuration: 10 * time.Second}
	cb.RecordFailure("test-server", cbCfg)
	h := newTestGatewayHandlerWithAudit(&mockGatewayServerStore{server: srv}, audit, gateway.NewTrustClassifier(nil, nil, nil), cb, &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	time.Sleep(100 * time.Millisecond)
	entries := audit.getEntries()
	if len(entries) == 0 {
		t.Fatal("expected audit entry for circuit open")
	}
	var details map[string]interface{}
	json.Unmarshal(entries[0].Details, &details)
	if details["outcome"] != "circuit_open" {
		t.Errorf("outcome = %q, want circuit_open", details["outcome"])
	}
}

// --- 9. ListTools ---

func TestGateway_ListTools_ReturnsEnabledServers(t *testing.T) {
	servers := &mockGatewayServerStore{list: []store.MCPServer{
		{Label: "enabled-server", Endpoint: "http://a.com", IsEnabled: true},
		{Label: "disabled-server", Endpoint: "http://b.com", IsEnabled: false},
		{Label: "another-enabled", Endpoint: "http://c.com", IsEnabled: true},
	}}
	h := newTestGatewayHandler(servers, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	h.ListTools(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	env := parseGatewayEnvelope(t, rr)
	data := env.Data.(map[string]interface{})
	serversList := data["servers"].([]interface{})
	if len(serversList) != 2 {
		t.Errorf("expected 2 enabled servers, got %d", len(serversList))
	}
}

func TestGateway_ListTools_EmptyList(t *testing.T) {
	servers := &mockGatewayServerStore{list: []store.MCPServer{}}
	h := newTestGatewayHandler(servers, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	h.ListTools(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGateway_ListTools_StoreError(t *testing.T) {
	servers := &mockGatewayServerStore{err: fmt.Errorf("db connection lost")}
	h := newTestGatewayHandler(servers, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	h.ListTools(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// --- 10. Input validation ---

func TestGateway_InvalidRequestBody(t *testing.T) {
	srv := enabledMCPServer()
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/proxy/test-server/tools/some_tool", bytes.NewReader([]byte("not json")))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("serverLabel", "test-server")
	rctx.URLParams.Add("toolName", "some_tool")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = auth.ContextWithUser(ctx, uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ProxyToolCall(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGateway_EmptyURLParams(t *testing.T) {
	h := newTestGatewayHandler(&mockGatewayServerStore{}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), &mockProxyForwarder{}, ratelimit.NewRateLimiter())
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/proxy//tools/", bytes.NewReader([]byte(`{}`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("serverLabel", "")
	rctx.URLParams.Add("toolName", "")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = auth.ContextWithUser(ctx, uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ProxyToolCall(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- 11. Default circuit breaker config ---

func TestGateway_DefaultCircuitBreakerConfig(t *testing.T) {
	srv := enabledMCPServer()
	srv.CircuitBreaker = nil
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{StatusCode: 200, Body: json.RawMessage(`{}`), Latency: 1 * time.Millisecond}}
	h := newTestGatewayHandler(&mockGatewayServerStore{server: srv}, gateway.NewTrustClassifier(nil, nil, nil), gateway.NewCircuitBreaker(), forwarder, ratelimit.NewRateLimiter())
	rr := makeGatewayRequest(t, h.ProxyToolCall, "test-server", "some_tool", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestParseCircuitBreakerCfg(t *testing.T) {
	tests := []struct {
		name      string
		input     json.RawMessage
		wantFail  int
		wantDur   time.Duration
		wantError bool
	}{
		{"nil uses defaults", nil, 5, 30 * time.Second, false},
		{"empty uses defaults", json.RawMessage(``), 5, 30 * time.Second, false},
		{"valid config", json.RawMessage(`{"fail_threshold":10,"open_duration_s":60}`), 10, 60 * time.Second, false},
		{"zero values use defaults", json.RawMessage(`{"fail_threshold":0,"open_duration_s":0}`), 5, 30 * time.Second, false},
		{"invalid JSON", json.RawMessage(`not json`), 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseCircuitBreakerCfg(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.FailThreshold != tt.wantFail {
				t.Errorf("fail_threshold = %d, want %d", cfg.FailThreshold, tt.wantFail)
			}
			if cfg.OpenDuration != tt.wantDur {
				t.Errorf("open_duration = %v, want %v", cfg.OpenDuration, tt.wantDur)
			}
		})
	}
}
