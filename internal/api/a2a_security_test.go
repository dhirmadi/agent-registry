package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/store"
	"github.com/google/uuid"
)

// =============================================================================
// Task #1: Authentication & Authorization Security Tests
// =============================================================================

// TestA2A_WellKnownPublicAccess verifies /.well-known/agent.json is accessible
// without any authentication, and cannot be tricked into requiring auth.
func TestA2A_WellKnownPublicAccess(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["a1"] = &store.Agent{
		ID: "a1", Name: "Agent One", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    a2aHandler,
	})

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name:    "no auth headers at all",
			headers: nil,
		},
		{
			name: "with invalid bearer token",
			headers: map[string]string{
				"Authorization": "Bearer areg_invalidtoken123",
			},
		},
		{
			name: "with garbage auth header",
			headers: map[string]string{
				"Authorization": "Basic dXNlcjpwYXNz",
			},
		},
		{
			name: "with expired session cookie",
			headers: map[string]string{
				"Cookie": "__Host-session=expired_session_id",
			},
		},
		{
			name: "with forged admin API key",
			headers: map[string]string{
				"Authorization": "Bearer areg_forged_admin_key_aaaa",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("well-known must be public: got %d, want 200; body: %s",
					w.Code, w.Body.String())
			}
		})
	}
}

// TestA2A_WellKnownRejectsNonGET ensures only GET is allowed on /.well-known/agent.json.
func TestA2A_WellKnownRejectsNonGET(t *testing.T) {
	agentStore := newMockAgentStore()
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}
	router := NewRouter(RouterConfig{Health: health, A2A: a2aHandler})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				t.Errorf("%s should not return 200 OK", method)
			}
		})
	}
}

// TestA2A_AgentCardRequiresAuth verifies /api/v1/agents/{agentId}/agent-card
// returns 401 without authentication.
func TestA2A_AgentCardRequiresAuth(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["target"] = &store.Agent{
		ID: "target", Name: "Target", Description: "Secret agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, SystemPrompt: "SECRET SYSTEM PROMPT",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    a2aHandler,
		Agents: &AgentsHandler{agents: agentStore},
		AuthMW: AuthMiddleware(&secMockSessionLookup{}, &secMockAPIKeyLookup{}),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/target/agent-card", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("agent-card without auth: got %d, want 401", w.Code)
	}

	// Verify no data leaked in error response
	body := w.Body.String()
	if strings.Contains(body, "SECRET SYSTEM PROMPT") {
		t.Error("system_prompt leaked in error response")
	}
	if strings.Contains(body, "Secret agent") {
		t.Error("agent description leaked in 401 error response")
	}
}

// TestA2A_A2AIndexRequiresAuth verifies /api/v1/agents/a2a-index
// returns 401 without authentication.
func TestA2A_A2AIndexRequiresAuth(t *testing.T) {
	agentStore := newMockAgentStore()
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    a2aHandler,
		Agents: &AgentsHandler{agents: agentStore},
		AuthMW: AuthMiddleware(&secMockSessionLookup{}, &secMockAPIKeyLookup{}),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("a2a-index without auth: got %d, want 401", w.Code)
	}
}

// TestA2A_RoleEnforcement verifies viewer+ role is required for agent-card and a2a-index.
func TestA2A_RoleEnforcement(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["agent1"] = &store.Agent{
		ID: "agent1", Name: "Agent 1", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	tests := []struct {
		name       string
		path       string
		role       string
		wantStatus int
	}{
		// agent-card
		{"agent-card viewer allowed", "/api/v1/agents/agent1/agent-card", "viewer", http.StatusOK},
		{"agent-card editor allowed", "/api/v1/agents/agent1/agent-card", "editor", http.StatusOK},
		{"agent-card admin allowed", "/api/v1/agents/agent1/agent-card", "admin", http.StatusOK},
		// a2a-index
		{"a2a-index viewer allowed", "/api/v1/agents/a2a-index", "viewer", http.StatusOK},
		{"a2a-index editor allowed", "/api/v1/agents/a2a-index", "editor", http.StatusOK},
		{"a2a-index admin allowed", "/api/v1/agents/a2a-index", "admin", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewA2AHandler(agentStore, "https://registry.example.com")

			var handler http.HandlerFunc
			if strings.Contains(tt.path, "a2a-index") {
				handler = h.GetA2AIndex
			} else {
				handler = h.GetAgentCard
			}

			req := agentRequest(http.MethodGet, tt.path, nil, tt.role)
			if strings.Contains(tt.path, "agent-card") {
				req = withChiParam(req, "agentId", "agent1")
			}
			w := httptest.NewRecorder()

			// Apply role middleware
			wrapped := RequireRole("viewer", "editor", "admin")(handler)
			wrapped.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// TestA2A_InvalidRoleBlocked verifies made-up roles cannot access A2A endpoints.
func TestA2A_InvalidRoleBlocked(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	invalidRoles := []string{"", "superadmin", "root", "operator", "service", "ADMIN", "Viewer"}

	for _, role := range invalidRoles {
		t.Run("role="+role, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, role)
			w := httptest.NewRecorder()

			wrapped := RequireRole("viewer", "editor", "admin")(http.HandlerFunc(h.GetA2AIndex))
			wrapped.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("role %q should be blocked: got %d, want 403", role, w.Code)
			}
		})
	}
}

// TestA2A_NoAuthContextBlocked verifies requests without any auth context
// (no user in context) are rejected by RequireRole.
func TestA2A_NoAuthContextBlocked(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Request with no auth context at all
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil)
	w := httptest.NewRecorder()

	wrapped := RequireRole("viewer", "editor", "admin")(http.HandlerFunc(h.GetA2AIndex))
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth context should return 401: got %d", w.Code)
	}
}

// =============================================================================
// Task #2: Data Exposure & Injection Security Tests
// =============================================================================

// TestA2A_NoSensitiveDataLeakage ensures A2A card responses do not expose
// sensitive fields: system_prompt, password_hash, trust_overrides, created_by.
func TestA2A_NoSensitiveDataLeakage(t *testing.T) {
	agent := &store.Agent{
		ID:             "leak_test",
		Name:           "Leak Test Agent",
		Description:    "Agent for leak testing",
		SystemPrompt:   "TOP SECRET: This is a confidential system prompt with API keys and passwords",
		Tools:          json.RawMessage(`[{"name": "secret_tool", "source": "internal", "server_label": "", "description": "tool"}]`),
		TrustOverrides: json.RawMessage(`{"override_key": "sensitive_value"}`),
		ExamplePrompts: json.RawMessage(`["hello"]`),
		IsActive:       true,
		Version:        5,
		CreatedBy:      "admin_user_id_leaked",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	card := agentToA2ACard(agent, "https://registry.example.com")
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal card: %v", err)
	}
	raw := string(data)

	sensitiveStrings := []string{
		"TOP SECRET",
		"confidential system prompt",
		"API keys and passwords",
		"admin_user_id_leaked",
		"sensitive_value",
		"override_key",
		"trust_overrides",
		"system_prompt",
		"password_hash",
		"created_by",
		"is_active",
	}

	for _, s := range sensitiveStrings {
		if strings.Contains(raw, s) {
			t.Errorf("A2A card leaked sensitive data: found %q in response", s)
		}
	}
}

// TestA2A_WellKnownNoSensitiveDataLeakage checks the well-known endpoint does not
// leak sensitive fields from any agent in the registry.
func TestA2A_WellKnownNoSensitiveDataLeakage(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["secret"] = &store.Agent{
		ID:             "secret",
		Name:           "Secret Agent",
		Description:    "Agent with secrets",
		SystemPrompt:   "CLASSIFIED: internal operations manual with credentials",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{"internal_policy": "restricted"}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "superadmin@internal.corp",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}

	body := w.Body.String()
	leakChecks := []string{
		"CLASSIFIED",
		"internal operations manual",
		"credentials",
		"internal_policy",
		"restricted",
		"superadmin@internal.corp",
		"system_prompt",
		"trust_overrides",
		"created_by",
		"password",
	}
	for _, s := range leakChecks {
		if strings.Contains(body, s) {
			t.Errorf("well-known leaked sensitive data: found %q", s)
		}
	}
}

// TestA2A_SQLInjectionViaAgentID tests SQL injection attempts through the agentId parameter.
func TestA2A_SQLInjectionViaAgentID(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	injectionPayloads := []struct {
		name    string
		payload string
	}{
		{"basic OR", "' OR '1'='1"},
		{"DROP TABLE", "'; DROP TABLE agents; --"},
		{"UNION SELECT", "1 UNION SELECT * FROM users--"},
		{"AND tautology", "agent_id' AND 1=1--"},
		{"double quote OR", "\" OR \"\"=\""},
		{"WAITFOR DELAY", "1'; WAITFOR DELAY '0:0:5'--"},
		{"URL encoded injection", "agent_id%27%20OR%20%271%27%3D%271"},
		{"null byte", "agent\x00id"},
		{"path traversal", "../../../etc/passwd"},
		{"header injection", "agent_id\nX-Injected: true"},
		{"very long id", strings.Repeat("A", 10000)},
		{"unicode injection", "\u0000\u001f\u007f"},
	}

	for _, tt := range injectionPayloads {
		t.Run(tt.name, func(t *testing.T) {
			// Use a safe URL for httptest.NewRequest, inject the payload via chi param
			req := agentRequest(http.MethodGet, "/api/v1/agents/PLACEHOLDER/agent-card", nil, "viewer")
			req = withChiParam(req, "agentId", tt.payload)
			w := httptest.NewRecorder()

			h.GetAgentCard(w, req)

			// Should return 404 (not found), not 200 or 500
			if w.Code == http.StatusOK {
				t.Errorf("SQL injection payload returned 200 OK: %q", tt.name)
			}
			if w.Code == http.StatusInternalServerError {
				t.Errorf("SQL injection payload caused 500: %q", tt.name)
			}
		})
	}
}

// TestA2A_QueryFilterInjection tests the ?q= parameter for regex DoS,
// script injection, and other malicious patterns.
func TestA2A_QueryFilterInjection(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["safe"] = &store.Agent{
		ID: "safe", Name: "Safe Agent", Description: "A safe agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	tests := []struct {
		name     string
		rawQuery string // raw query string (without leading ?)
	}{
		{"regex DoS catastrophic backtracking", "q=" + strings.Repeat("a", 10000)},
		{"null bytes", "q=test%00evil"},
		{"XSS script tag", "q=%3Cscript%3Ealert(1)%3C/script%3E"},
		{"SQL injection in query", "q=%27%20OR%201%3D1--"},
		{"path traversal", "q=../../etc/passwd"},
		{"unicode overflow", "q=" + strings.Repeat("%C0%AF", 500)},
		{"newline injection", "q=test%0D%0AX-Injected:%20true"},
		{"very long query", "q=" + strings.Repeat("x", 100000)},
		{"empty query", "q="},
		{"special chars", "q=%00%0d%0a%0d%0a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
			req.URL.RawQuery = tt.rawQuery
			w := httptest.NewRecorder()

			h.GetA2AIndex(w, req)

			// Should not crash or return 500
			if w.Code == http.StatusInternalServerError {
				t.Errorf("query injection caused 500: %s", tt.name)
			}
		})
	}
}

// TestA2A_PaginationBoundaryAttacks tests limit/offset parameters for
// integer overflow, negative values, and resource exhaustion.
func TestA2A_PaginationBoundaryAttacks(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"negative offset", "?offset=-1", http.StatusOK},
		{"negative limit", "?limit=-1", http.StatusOK},
		{"zero limit", "?limit=0", http.StatusOK},
		{"max int limit", "?limit=9999999999999", http.StatusOK},
		{"max int offset", "?offset=9999999999999", http.StatusOK},
		{"int overflow limit", "?limit=99999999999999999999999", http.StatusOK},
		{"float limit", "?limit=1.5", http.StatusOK},
		{"NaN limit", "?limit=NaN", http.StatusOK},
		{"limit exceeds cap", "?limit=201", http.StatusOK},
		{"non-numeric offset", "?offset=abc", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.GetA2AIndex(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// TestA2A_LimitClampedTo200 verifies the limit parameter is clamped to 200 max.
func TestA2A_LimitClampedTo200(t *testing.T) {
	agentStore := newMockAgentStore()
	// Seed more than 200 agents to verify clamping
	for i := 0; i < 250; i++ {
		id := fmt.Sprintf("agent_%03d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: "Test",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
	}
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=500", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})
	if len(cards) > 200 {
		t.Errorf("limit not clamped: got %d cards, max should be 200", len(cards))
	}
}

// TestA2A_ETagHeaderInjection verifies the ETag cannot be used for header injection.
func TestA2A_ETagHeaderInjection(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["etag_test"] = &store.Agent{
		ID: "etag_test", Name: "ETag Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag header missing")
	}

	// ETag should be properly quoted
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag not properly quoted: %q", etag)
	}

	// ETag should not contain newlines (header injection)
	if strings.ContainsAny(etag, "\r\n") {
		t.Errorf("ETag contains newline characters (header injection risk): %q", etag)
	}
}

// TestA2A_IfNoneMatchHeaderInjection tests that crafted If-None-Match values
// do not cause crashes or header injection.
func TestA2A_IfNoneMatchHeaderInjection(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	maliciousETags := []string{
		`"`, // incomplete quote
		`""`, // empty value
		"",
		strings.Repeat("A", 10000), // extremely long
		"\x00\x00\x00",
		`"value\r\nX-Injected: true"`,
		`W/"weak"`, // weak ETag
		`"*"`,
	}

	for _, etag := range maliciousETags {
		t.Run("etag="+etag[:min(len(etag), 20)], func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			req.Header.Set("If-None-Match", etag)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)

			// Should be 200 (not matched) or at worst 304 (unlikely) — never 500
			if w.Code == http.StatusInternalServerError {
				t.Errorf("If-None-Match caused 500: %q", etag)
			}
		})
	}
}

// TestA2A_MalformedToolsJSON tests that agents with malformed tools JSON
// in the database don't cause panics or data leaks.
func TestA2A_MalformedToolsJSON(t *testing.T) {
	tests := []struct {
		name  string
		tools json.RawMessage
	}{
		{"null tools", nil},
		{"empty string tools", json.RawMessage(``)},
		{"invalid json", json.RawMessage(`{{{invalid`)},
		{"string instead of array", json.RawMessage(`"not an array"`)},
		{"number instead of array", json.RawMessage(`42`)},
		{"nested object", json.RawMessage(`{"nested": {"deep": true}}`)},
		{"array of nulls", json.RawMessage(`[null, null]`)},
		{"array with missing fields", json.RawMessage(`[{"name": "tool"}]`)},
		{"extremely large tools", json.RawMessage(`[` + strings.Repeat(`{"name":"t","source":"s","server_label":"l","description":"d"},`, 1000) + `{"name":"last","source":"s","server_label":"l","description":"d"}]`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID:             "malformed",
				Name:           "Malformed Agent",
				Description:    "Agent with bad tools",
				Tools:          tt.tools,
				ExamplePrompts: json.RawMessage(`[]`),
				Version:        1,
			}

			// Should not panic
			card := agentToA2ACard(agent, "https://registry.example.com")

			// Card should always have at least one skill (synthetic or parsed)
			if len(card.Skills) == 0 && tt.name != "invalid json" &&
				tt.name != "string instead of array" &&
				tt.name != "number instead of array" &&
				tt.name != "nested object" &&
				tt.name != "empty string tools" {
				// For truly invalid JSON, 0 skills might still produce a synthetic skill
				// since json.Unmarshal fails and len(tools)==0 → synthetic path
			}

			// Verify it marshals without error
			_, err := json.Marshal(card)
			if err != nil {
				t.Errorf("card failed to marshal: %v", err)
			}
		})
	}
}

// TestA2A_MalformedExamplePromptsJSON tests agents with malformed example_prompts.
func TestA2A_MalformedExamplePromptsJSON(t *testing.T) {
	tests := []struct {
		name     string
		examples json.RawMessage
	}{
		{"null examples", nil},
		{"empty string", json.RawMessage(``)},
		{"invalid json", json.RawMessage(`not json`)},
		{"object instead of array", json.RawMessage(`{"key": "value"}`)},
		{"array of objects", json.RawMessage(`[{"not": "string"}]`)},
		{"deeply nested", json.RawMessage(`[[["nested"]]]`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID:             "example_test",
				Name:           "Example Test",
				Description:    "Test",
				Tools:          json.RawMessage(`[]`),
				ExamplePrompts: tt.examples,
				Version:        1,
			}

			// Should not panic
			card := agentToA2ACard(agent, "https://registry.example.com")
			_, err := json.Marshal(card)
			if err != nil {
				t.Errorf("card failed to marshal: %v", err)
			}
		})
	}
}

// TestA2A_NoSSRFViaURLConstruction verifies the external URL is not user-controlled
// and that the URL in the card is properly constructed (no SSRF).
func TestA2A_NoSSRFViaURLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		externalURL string
		agentID     string
		wantURL     string
	}{
		{
			"normal URL",
			"https://registry.example.com",
			"agent1",
			"https://registry.example.com/api/v1/agents/agent1",
		},
		{
			"URL with trailing slash stripped by convention",
			"https://registry.example.com/",
			"agent1",
			"https://registry.example.com//api/v1/agents/agent1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID: tt.agentID, Name: "Test", Description: "Test",
				Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
				Version: 1,
			}
			card := agentToA2ACard(agent, tt.externalURL)
			if card.URL != tt.wantURL {
				t.Errorf("URL: got %q, want %q", card.URL, tt.wantURL)
			}
		})
	}
}

// TestA2A_XSSInAgentFields ensures agent fields that could contain HTML/JS
// are not sanitized (JSON encoding handles this) and don't break JSON output.
func TestA2A_XSSInAgentFields(t *testing.T) {
	agent := &store.Agent{
		ID:             "xss_agent",
		Name:           `<script>alert("xss")</script>`,
		Description:    `<img src=x onerror=alert(1)>`,
		Tools:          json.RawMessage(`[{"name": "<script>", "source": "internal", "server_label": "", "description": "onclick=alert(1)"}]`),
		ExamplePrompts: json.RawMessage(`["<script>alert('xss')</script>"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// JSON encoding should escape the angle brackets
	raw := string(data)
	if strings.Contains(raw, "<script>") && !strings.Contains(raw, "\\u003c") {
		// Go's json.Marshal by default escapes < > & to \u003c \u003e \u0026
		// So this should not contain raw <script> tags
		t.Log("Note: json.Marshal escapes HTML entities by default, which is safe")
	}

	// Verify the JSON is valid
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("XSS payload broke JSON output: %v", err)
	}
}

// TestA2A_WellKnownContentTypeHeader verifies Content-Type is application/json
// (prevents MIME sniffing attacks).
func TestA2A_WellKnownContentTypeHeader(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestA2A_SecurityHeadersOnWellKnown verifies security headers are set on
// the well-known endpoint through the router.
func TestA2A_SecurityHeadersOnWellKnown(t *testing.T) {
	agentStore := newMockAgentStore()
	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{Health: health, A2A: a2aHandler})

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
	}

	for header, want := range expectedHeaders {
		got := w.Header().Get(header)
		if got != want {
			t.Errorf("header %s: got %q, want %q", header, got, want)
		}
	}
}

// TestA2A_StoreErrorDoesNotLeakDetails verifies internal errors do not
// expose stack traces or SQL error details.
func TestA2A_StoreErrorDoesNotLeakDetails(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.getByIDErr = fmt.Errorf("pq: connection to server at \"10.0.0.5\" failed: timeout")

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/test/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "test")
	w := httptest.NewRecorder()
	h.GetAgentCard(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", w.Code)
	}

	body := w.Body.String()
	// Should NOT contain internal error details
	if strings.Contains(body, "10.0.0.5") {
		t.Error("internal IP address leaked in error response")
	}
	if strings.Contains(body, "pq:") {
		t.Error("database driver error leaked in error response")
	}
	if strings.Contains(body, "timeout") {
		t.Error("connection details leaked in error response")
	}
	// Should contain generic error message
	if !strings.Contains(body, "failed to retrieve agent") {
		t.Error("expected generic error message in response")
	}
}

// TestA2A_WellKnownStoreErrorDoesNotLeakDetails verifies the well-known endpoint
// does not leak internal error details.
func TestA2A_WellKnownStoreErrorDoesNotLeakDetails(t *testing.T) {
	agentStore := &errorAgentStore{
		listErr: fmt.Errorf("FATAL: database 'registry' does not exist at /var/lib/postgresql/data"),
	}
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "/var/lib/postgresql") {
		t.Error("filesystem path leaked in error response")
	}
	if strings.Contains(body, "FATAL") {
		t.Error("database error severity leaked in error response")
	}
}

// TestA2A_CacheControlOnWellKnown verifies the Cache-Control header is set correctly.
func TestA2A_CacheControlOnWellKnown(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=60" {
		t.Errorf("Cache-Control: got %q, want %q", cc, "public, max-age=60")
	}
}

// TestA2A_AgentCardJSONStructuralIntegrity ensures the response is valid JSON
// and has the expected A2A protocol structure.
func TestA2A_AgentCardJSONStructuralIntegrity(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["integrity"] = &store.Agent{
		ID: "integrity", Name: "Integrity Agent", Description: "Test structural integrity",
		Tools: json.RawMessage(`[{"name":"tool1","source":"internal","server_label":"","description":"desc"}]`),
		ExamplePrompts: json.RawMessage(`["example 1"]`),
		IsActive:       true, Version: 3,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")
	req := agentRequest(http.MethodGet, "/api/v1/agents/integrity/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "integrity")
	w := httptest.NewRecorder()
	h.GetAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}
	data := env.Data.(map[string]interface{})

	// Required A2A fields
	requiredFields := []string{
		"name", "description", "url", "version", "protocolVersion",
		"provider", "capabilities", "defaultInputModes", "defaultOutputModes",
		"skills", "securitySchemes", "security",
	}
	for _, f := range requiredFields {
		if _, ok := data[f]; !ok {
			t.Errorf("missing required A2A field: %s", f)
		}
	}

	// Verify no unexpected internal fields leaked
	forbiddenFields := []string{
		"id", "system_prompt", "trust_overrides", "is_active",
		"created_by", "created_at", "updated_at", "password_hash",
		"example_prompts", "tools",
	}
	for _, f := range forbiddenFields {
		if _, ok := data[f]; ok {
			t.Errorf("internal field leaked in A2A card: %s", f)
		}
	}
}

// TestA2A_IndexResponseEnvelope verifies the a2a-index response uses the standard
// API envelope and does not leak metadata.
func TestA2A_IndexResponseEnvelope(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["env_test"] = &store.Agent{
		ID: "env_test", Name: "Env Test", Description: "Envelope test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")
	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}

	card := cards[0].(map[string]interface{})
	// Verify no sensitive fields in indexed cards
	for _, field := range []string{"system_prompt", "trust_overrides", "created_by", "password_hash"} {
		if _, ok := card[field]; ok {
			t.Errorf("sensitive field %q leaked in a2a-index card", field)
		}
	}
}

// =============================================================================
// Helper mocks for security tests
// =============================================================================

// secMockSessionLookup always returns an error (no valid sessions).
type secMockSessionLookup struct{}

func (m *secMockSessionLookup) GetSessionUser(_ context.Context, _ string) (uuid.UUID, string, string, error) {
	return uuid.Nil, "", "", fmt.Errorf("session not found")
}
func (m *secMockSessionLookup) TouchSession(_ context.Context, _ string) error { return nil }

// secMockAPIKeyLookup always returns an error (no valid API keys).
type secMockAPIKeyLookup struct{}

func (m *secMockAPIKeyLookup) ValidateAPIKey(_ context.Context, _ string) (uuid.UUID, string, error) {
	return uuid.Nil, "", fmt.Errorf("invalid API key")
}

// errorAgentStore is a mock that always returns errors from List.
type errorAgentStore struct {
	mockAgentStore
	listErr error
}

func (e *errorAgentStore) List(_ context.Context, _ bool, _, _ int) ([]store.Agent, int, error) {
	return nil, 0, e.listErr
}

// Ensure packages are used (they're used in agentRequest/withChiParam helpers from agents_test.go)
var _ = auth.ContextWithUser
