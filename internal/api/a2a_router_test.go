package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// Task #8: Router and Middleware Integration Tests for A2A Endpoints
// =============================================================================

// makeA2ARouter creates a router with auth middleware and A2A + Agents handlers wired up.
// The fakeAuth middleware simulates the auth middleware by reading from context
// or rejecting if no user is set.
func makeA2ARouter(agentStore *mockAgentStore) (chi.Router, *A2AHandler) {
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	agentsHandler := NewAgentsHandler(agentStore, &mockAuditStoreForAPI{}, nil)
	health := &HealthHandler{}

	fakeAuthMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If context already has a user, pass through
			if _, ok := auth.UserIDFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

			// Check for test auth header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				RespondError(w, r, apierrors.Unauthorized("authentication required"))
				return
			}

			// Parse role from auth header: "Bearer test-<role>"
			parts := strings.SplitN(authHeader, "test-", 2)
			if len(parts) != 2 {
				RespondError(w, r, apierrors.Unauthorized("invalid credentials"))
				return
			}
			role := parts[1]
			ctx := auth.ContextWithUser(r.Context(), uuid.New(), role, "test")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    a2aHandler,
		Agents: agentsHandler,
		AuthMW: fakeAuthMW,
	})

	return router, a2aHandler
}

func seedTestAgent(s *mockAgentStore) {
	now := time.Now()
	s.agents["pmo"] = &store.Agent{
		ID: "pmo", Name: "PMO Agent", Description: "Project management",
		Tools: json.RawMessage(`[{"name":"plan","source":"internal","server_label":"","description":"Plan"}]`),
		ExamplePrompts: json.RawMessage(`["Create a sprint"]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}

// --- Well-known: no auth required ---

func TestA2ARouter_WellKnown_NoAuthRequired(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	// No auth header at all
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("well-known should not require auth: got %d, want 200; body: %s",
			w.Code, w.Body.String())
	}

	var card A2AAgentCard
	if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if card.ProtocolVersion != "0.3.0" {
		t.Errorf("protocolVersion: got %q, want %q", card.ProtocolVersion, "0.3.0")
	}
}

// --- Agent card: requires viewer+ role ---

func TestA2ARouter_AgentCard_RequiresAuth(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	// No auth header
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pmo/agent-card", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("agent-card without auth: got %d, want 401; body: %s",
			w.Code, w.Body.String())
	}
}

func TestA2ARouter_AgentCard_ViewerAllowed(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pmo/agent-card", nil)
	req.Header.Set("Authorization", "Bearer test-viewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("viewer should access agent-card: got %d, want 200; body: %s",
			w.Code, w.Body.String())
	}
}

func TestA2ARouter_AgentCard_EditorAllowed(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pmo/agent-card", nil)
	req.Header.Set("Authorization", "Bearer test-editor")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("editor should access agent-card: got %d, want 200; body: %s",
			w.Code, w.Body.String())
	}
}

func TestA2ARouter_AgentCard_AdminAllowed(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pmo/agent-card", nil)
	req.Header.Set("Authorization", "Bearer test-admin")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("admin should access agent-card: got %d, want 200; body: %s",
			w.Code, w.Body.String())
	}
}

// --- A2A Index: requires viewer+ role ---

func TestA2ARouter_Index_RequiresAuth(t *testing.T) {
	agentStore := newMockAgentStore()
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("a2a-index without auth: got %d, want 401; body: %s",
			w.Code, w.Body.String())
	}
}

func TestA2ARouter_Index_ViewerAllowed(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	req.Header.Set("Authorization", "Bearer test-viewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("viewer should access a2a-index: got %d, want 200; body: %s",
			w.Code, w.Body.String())
	}
}

// --- Security headers on A2A responses ---

func TestA2ARouter_WellKnown_SecurityHeaders(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}

	for header, expected := range expectedHeaders {
		got := w.Header().Get(header)
		if got != expected {
			t.Errorf("well-known %s: got %q, want %q", header, got, expected)
		}
	}

	// CSP should be set
	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header should be set")
	}

	// Permissions-Policy should be set
	pp := w.Header().Get("Permissions-Policy")
	if pp == "" {
		t.Error("Permissions-Policy header should be set")
	}
}

func TestA2ARouter_AgentCard_SecurityHeaders(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/pmo/agent-card", nil)
	req.Header.Set("Authorization", "Bearer test-viewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// All security headers should be present on API endpoints too
	expectedHeaders := []string{
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
		"Permissions-Policy",
	}

	for _, header := range expectedHeaders {
		if w.Header().Get(header) == "" {
			t.Errorf("agent-card should have %s header", header)
		}
	}
}

// --- CORS on A2A responses ---

func TestA2ARouter_WellKnown_CORSNoOrigin(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	// No Origin header
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// No CORS headers should be set when no Origin is provided
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("no CORS headers expected without Origin header")
	}
}

func TestA2ARouter_WellKnown_CORSCrossOrigin(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Cross-origin should NOT get CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("cross-origin requests should not get CORS headers")
	}
}

func TestA2ARouter_WellKnown_CORSSameOrigin(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Host = "registry.example.com"
	req.Header.Set("Origin", "https://registry.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Same-origin should get CORS headers
	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "https://registry.example.com" {
		t.Errorf("same-origin CORS: got %q, want %q", acao, "https://registry.example.com")
	}
}

func TestA2ARouter_WellKnown_Preflight(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/agent.json", nil)
	req.Host = "registry.example.com"
	req.Header.Set("Origin", "https://registry.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight: got %d, want 204", w.Code)
	}
}

// --- Auth failure error envelope format ---

func TestA2ARouter_AuthFailure_EnvelopeFormat(t *testing.T) {
	agentStore := newMockAgentStore()
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	// No auth
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	// Error response should follow standard envelope
	if raw["success"] != false {
		t.Errorf("error response: success should be false, got %v", raw["success"])
	}
	if raw["error"] == nil {
		t.Error("error response should have 'error' field")
	}

	errObj, ok := raw["error"].(map[string]interface{})
	if !ok {
		t.Fatal("error should be an object")
	}
	if _, ok := errObj["code"]; !ok {
		t.Error("error should have 'code' field")
	}
	if _, ok := errObj["message"]; !ok {
		t.Error("error should have 'message' field")
	}

	// meta should be present
	if raw["meta"] == nil {
		t.Error("error response should have 'meta' field")
	}
}

func TestA2ARouter_RoleForbidden_EnvelopeFormat(t *testing.T) {
	agentStore := newMockAgentStore()
	router, _ := makeA2ARouter(agentStore)

	// Use a role that doesn't have access (e.g., "norole" is not viewer/editor/admin)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	req.Header.Set("Authorization", "Bearer test-norole")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid role, got %d; body: %s", w.Code, w.Body.String())
	}

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	if raw["success"] != false {
		t.Error("forbidden response should have success=false")
	}

	errObj := raw["error"].(map[string]interface{})
	if errObj["code"] != "FORBIDDEN" {
		t.Errorf("error code: got %v, want FORBIDDEN", errObj["code"])
	}
}

// --- Content-Type on responses ---

func TestA2ARouter_WellKnown_ContentTypeJSON(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

func TestA2ARouter_Index_ContentTypeJSON(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	req.Header.Set("Authorization", "Bearer test-viewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// --- Well-known does not go through rate limiting ---

func TestA2ARouter_WellKnown_NotRateLimited(t *testing.T) {
	// Well-known is mounted before the /api/v1 group which has rate limiting.
	// It should work even with a rate limiter configured.
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	// Send many requests rapidly — well-known should never get rate-limited
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("well-known request %d got %d, expected 200", i, w.Code)
		}
	}
}

// --- HTTP method enforcement ---

func TestA2ARouter_AgentCard_OnlyGET(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/agents/pmo/agent-card", nil)
			req.Header.Set("Authorization", "Bearer test-editor")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				t.Errorf("agent-card should not accept %s (got 200)", method)
			}
		})
	}
}

func TestA2ARouter_Index_OnlyGET(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/agents/a2a-index", nil)
			req.Header.Set("Authorization", "Bearer test-editor")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				t.Errorf("a2a-index should not accept %s (got 200)", method)
			}
		})
	}
}

// --- Route precedence: a2a-index should not conflict with {agentId} ---

func TestA2ARouter_A2AIndexNotCapturedAsAgentID(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)
	router, _ := makeA2ARouter(agentStore)

	// a2a-index is a fixed route that should take precedence over the {agentId} param
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	req.Header.Set("Authorization", "Bearer test-viewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("a2a-index should be a separate route: got %d; body: %s",
			w.Code, w.Body.String())
	}

	// Should return index format (with agent_cards), not single agent format
	env := parseEnvelope(t, w)
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if _, ok := data["agent_cards"]; !ok {
		t.Error("expected 'agent_cards' in response — route may have matched {agentId} instead of a2a-index")
	}
}

// --- Well-known path takes precedence over SPA catch-all ---

func TestA2ARouter_WellKnown_PrecedenceOverSPA(t *testing.T) {
	agentStore := newMockAgentStore()
	seedTestAgent(agentStore)

	// Create router without SPA (WebFS=nil) to isolate well-known behavior
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    a2aHandler,
		// WebFS is nil — no SPA
	})

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("well-known should take precedence: got %d", w.Code)
	}

	// Should be JSON, not HTML
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q, should contain application/json", ct)
	}
}
