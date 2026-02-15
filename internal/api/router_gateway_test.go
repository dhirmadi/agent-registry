package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/gateway"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// fakeAuthMW injects a user context for testing router-level auth.
func fakeAuthMW(userID uuid.UUID, role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.ContextWithUser(r.Context(), userID, role, "session")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func buildGatewayRouter(gw *MCPGatewayHandler, authMW func(http.Handler) http.Handler) http.Handler {
	return NewRouter(RouterConfig{
		Health:     &HealthHandler{},
		MCPGateway: gw,
		AuthMW:     authMW,
	})
}

func TestRouter_GatewayRoutesRegistered(t *testing.T) {
	srv := &store.MCPServer{
		ID: uuid.New(), Label: "test-srv", Endpoint: "http://localhost:9999",
		AuthType: "none", IsEnabled: true,
		CircuitBreaker: json.RawMessage(`{"fail_threshold":5,"open_duration_s":30}`),
	}
	servers := &mockGatewayServerStore{server: srv}
	forwarder := &mockProxyForwarder{resp: &gateway.ProxyResponse{
		StatusCode: 200, Body: json.RawMessage(`{"ok":true}`), Latency: time.Millisecond,
	}}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	rl := ratelimit.NewRateLimiter()
	gw := NewMCPGatewayHandler(servers, &safeAuditMock{}, tc, cb, forwarder, rl, testEncKey)

	userID := uuid.New()
	router := buildGatewayRouter(gw, fakeAuthMW(userID, "admin"))

	// POST /mcp/v1/proxy/{serverLabel}/tools/{toolName}
	body := []byte(`{"arguments":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/proxy/test-srv/tools/my_tool", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("POST proxy: expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// GET /mcp/v1/tools
	req2 := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("GET tools: expected 200, got %d", rr2.Code)
	}
}

func TestRouter_GatewayRoutesAbsentWhenNil(t *testing.T) {
	router := NewRouter(RouterConfig{
		Health:     &HealthHandler{},
		MCPGateway: nil,
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp/v1/proxy/srv/tools/tool", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when gateway nil, got %d", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /mcp/v1/tools when gateway nil, got %d", rr2.Code)
	}
}

func TestRouter_GatewayRequiresAuth(t *testing.T) {
	servers := &mockGatewayServerStore{list: []store.MCPServer{}}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	rl := ratelimit.NewRateLimiter()
	gw := NewMCPGatewayHandler(servers, &safeAuditMock{}, tc, cb, &mockProxyForwarder{}, rl, testEncKey)

	// Use an auth middleware that rejects (simulates no valid session)
	rejectAuthMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Don't set user context — RequireRole will reject
			next.ServeHTTP(w, r)
		})
	}

	router := buildGatewayRouter(gw, rejectAuthMW)

	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

func TestRouter_GatewayRoleRestriction(t *testing.T) {
	servers := &mockGatewayServerStore{list: []store.MCPServer{}}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	rl := ratelimit.NewRateLimiter()
	gw := NewMCPGatewayHandler(servers, &safeAuditMock{}, tc, cb, &mockProxyForwarder{}, rl, testEncKey)

	tests := []struct {
		role       string
		wantStatus int
	}{
		{"viewer", http.StatusOK},
		{"editor", http.StatusOK},
		{"admin", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			router := buildGatewayRouter(gw, fakeAuthMW(uuid.New(), tt.role))
			req := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("role %s: expected %d, got %d", tt.role, tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestRouter_GatewayNoAuthMiddleware(t *testing.T) {
	// When AuthMW is nil, gateway routes should still work (no auth enforcement)
	servers := &mockGatewayServerStore{list: []store.MCPServer{}}
	tc := gateway.NewTrustClassifier(nil, nil, nil)
	cb := gateway.NewCircuitBreaker()
	rl := ratelimit.NewRateLimiter()
	gw := NewMCPGatewayHandler(servers, &safeAuditMock{}, tc, cb, &mockProxyForwarder{}, rl, testEncKey)

	router := NewRouter(RouterConfig{
		Health:     &HealthHandler{},
		MCPGateway: gw,
		AuthMW:     nil, // No auth middleware
	})

	// Without auth context, RequireRole will reject with 401
	req := httptest.NewRequest(http.MethodGet, "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// RequireRole checks context — no user in context = 401
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth context, got %d", rr.Code)
	}
}

