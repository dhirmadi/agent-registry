package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock auth handler that implements AuthRouteHandler ---

type mockAuthRouteHandler struct {
	loginCallCount int
}

func (m *mockAuthRouteHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	m.loginCallCount++
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "logged in"})
}

func (m *mockAuthRouteHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "logged out"})
}

func (m *mockAuthRouteHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"id":       "00000000-0000-0000-0000-000000000001",
		"username": "testuser",
		"role":     "admin",
	})
}

func (m *mockAuthRouteHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "password changed"})
}

func (m *mockAuthRouteHandler) HandleGoogleStart(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "google start"})
}

func (m *mockAuthRouteHandler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "google callback"})
}

func (m *mockAuthRouteHandler) HandleUnlinkGoogle(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "google unlinked"})
}

// --- Mock session lookup for AuthMiddleware ---

type mockSessionLookupForInteg struct {
	sessions map[string]mockSessionData
}

type mockSessionData struct {
	userID    uuid.UUID
	role      string
	csrfToken string
}

func (m *mockSessionLookupForInteg) GetSessionUser(_ context.Context, sessionID string) (uuid.UUID, string, string, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return uuid.Nil, "", "", fmt.Errorf("session not found")
	}
	return s.userID, s.role, s.csrfToken, nil
}

func (m *mockSessionLookupForInteg) TouchSession(_ context.Context, _ string) error {
	return nil
}

// --- Mock API key lookup for AuthMiddleware ---

type mockAPIKeyLookupForInteg struct {
	keys map[string]mockAPIKeyData
}

type mockAPIKeyData struct {
	userID uuid.UUID
	role   string
}

func (m *mockAPIKeyLookupForInteg) ValidateAPIKey(_ context.Context, key string) (uuid.UUID, string, error) {
	k, ok := m.keys[key]
	if !ok {
		return uuid.Nil, "", fmt.Errorf("invalid API key")
	}
	return k.userID, k.role, nil
}

// --- Helper to build a full integration test router ---

type integTestSetup struct {
	server      *httptest.Server
	agentStore  *mockAgentStore
	auditStore  *mockAuditStoreForAPI
	sessions    *mockSessionLookupForInteg
	apiKeys     *mockAPIKeyLookupForInteg
	rateLimiter *ratelimit.RateLimiter
	authHandler *mockAuthRouteHandler
	userID      uuid.UUID
}

func newIntegTestSetup(t *testing.T) *integTestSetup {
	t.Helper()

	userID := uuid.New()
	csrfToken := "test-csrf-token"

	sessions := &mockSessionLookupForInteg{
		sessions: map[string]mockSessionData{
			"valid-session": {
				userID:    userID,
				role:      "admin",
				csrfToken: csrfToken,
			},
			"editor-session": {
				userID:    uuid.New(),
				role:      "editor",
				csrfToken: csrfToken,
			},
			"viewer-session": {
				userID:    uuid.New(),
				role:      "viewer",
				csrfToken: csrfToken,
			},
		},
	}

	apiKeys := &mockAPIKeyLookupForInteg{
		keys: map[string]mockAPIKeyData{
			"areg_valid_key_12345": {
				userID: userID,
				role:   "admin",
			},
		},
	}

	agentStore := newMockAgentStore()
	auditStore := &mockAuditStoreForAPI{}
	authHandler := &mockAuthRouteHandler{}
	rl := ratelimit.NewRateLimiter()

	agentsHandler := NewAgentsHandler(agentStore, auditStore, nil)

	cfg := RouterConfig{
		Health:    &HealthHandler{DB: &mockPinger{}},
		Auth:      authHandler,
		Agents:    agentsHandler,
		AuthMW:    AuthMiddleware(sessions, apiKeys),
		RateLimiter: rl,
	}

	router := NewRouter(cfg)
	server := httptest.NewServer(router)

	return &integTestSetup{
		server:      server,
		agentStore:  agentStore,
		auditStore:  auditStore,
		sessions:    sessions,
		apiKeys:     apiKeys,
		rateLimiter: rl,
		authHandler: authHandler,
		userID:      userID,
	}
}

func (s *integTestSetup) close() {
	s.server.Close()
}

// sessionRequest creates a request with valid session cookie and CSRF headers.
func (s *integTestSetup) sessionRequest(method, path string, body interface{}, sessionID string) (*http.Request, error) {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}

	req, err := http.NewRequest(method, s.server.URL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionID})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf-token"})
	req.Header.Set("X-CSRF-Token", "test-csrf-token")
	return req, nil
}

// --- Integration tests ---

func TestIntegration_HealthEndpoints(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"healthz returns 200", "/healthz", http.StatusOK},
		{"readyz returns 200", "/readyz", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(setup.server.URL + tt.path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestIntegration_AuthLoginFlow(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	body := bytes.NewBufferString(`{"username":"admin","password":"pass"}`)
	resp, err := http.Post(setup.server.URL+"/auth/login", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var env Envelope
	json.NewDecoder(resp.Body).Decode(&env)
	if !env.Success {
		t.Fatal("expected success=true in login response")
	}
}

func TestIntegration_AuthMutationAuditTrail(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	// Create an agent via POST /api/v1/agents with session auth
	agentBody := map[string]interface{}{
		"id":   "audit_test_agent",
		"name": "Audit Test Agent",
	}

	req, err := setup.sessionRequest(http.MethodPost, "/api/v1/agents", agentBody, "valid-session")
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		t.Fatalf("expected 201, got %d; body: %s", resp.StatusCode, body.String())
	}

	// Verify audit store received an entry
	if len(setup.auditStore.entries) == 0 {
		t.Fatal("expected at least one audit entry after agent creation")
	}

	entry := setup.auditStore.entries[len(setup.auditStore.entries)-1]
	if entry.Action != "agent_create" {
		t.Fatalf("expected audit action 'agent_create', got %q", entry.Action)
	}
	if entry.ResourceType != "agent" {
		t.Fatalf("expected resource_type 'agent', got %q", entry.ResourceType)
	}
	if entry.ResourceID != "audit_test_agent" {
		t.Fatalf("expected resource_id 'audit_test_agent', got %q", entry.ResourceID)
	}
}

func TestIntegration_CSRFEnforcement(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	agentBody := map[string]interface{}{
		"id":   "csrf_test",
		"name": "CSRF Test Agent",
	}

	t.Run("POST without CSRF token returns 403", func(t *testing.T) {
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(agentBody)

		req, _ := http.NewRequest(http.MethodPost, setup.server.URL+"/api/v1/agents", &buf)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-session"})
		// No CSRF cookie or header

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("POST with valid CSRF token succeeds", func(t *testing.T) {
		agentBody["id"] = "csrf_test_ok"
		req, _ := setup.sessionRequest(http.MethodPost, "/api/v1/agents", agentBody, "valid-session")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			var body bytes.Buffer
			body.ReadFrom(resp.Body)
			t.Fatalf("expected 201, got %d; body: %s", resp.StatusCode, body.String())
		}
	})
}

func TestIntegration_RoleEnforcement_ViewerBlocked(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	agentBody := map[string]interface{}{
		"id":   "viewer_blocked",
		"name": "Viewer Blocked Agent",
	}

	req, _ := setup.sessionRequest(http.MethodPost, "/api/v1/agents", agentBody, "viewer-session")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating agent, got %d", resp.StatusCode)
	}
}

func TestIntegration_RoleEnforcement_EditorAllowed(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	agentBody := map[string]interface{}{
		"id":   "editor_allowed",
		"name": "Editor Allowed Agent",
	}

	req, _ := setup.sessionRequest(http.MethodPost, "/api/v1/agents", agentBody, "editor-session")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		t.Fatalf("expected 201 for editor creating agent, got %d; body: %s", resp.StatusCode, body.String())
	}
}

func TestIntegration_IfMatchConcurrency(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Seed an agent
	now := time.Now().UTC()
	setup.agentStore.agents["concurrency_test"] = &store.Agent{
		ID:             "concurrency_test",
		Name:           "Concurrency Test",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	t.Run("PUT without If-Match returns 400", func(t *testing.T) {
		updateBody := map[string]interface{}{"name": "Updated"}
		req, _ := setup.sessionRequest(http.MethodPut, "/api/v1/agents/concurrency_test", updateBody, "valid-session")
		// No If-Match header

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 without If-Match, got %d", resp.StatusCode)
		}
	})

	t.Run("PUT with stale If-Match returns 409", func(t *testing.T) {
		updateBody := map[string]interface{}{"name": "Updated"}
		req, _ := setup.sessionRequest(http.MethodPut, "/api/v1/agents/concurrency_test", updateBody, "valid-session")
		// Stale etag
		req.Header.Set("If-Match", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 with stale etag, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_RateLimitLogin(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Login rate limit is 5 per 15 minutes per IP.
	// We set X-Real-IP so chi's RealIP middleware normalizes RemoteAddr
	// to a consistent value (without ephemeral port).
	for i := 0; i < 5; i++ {
		body := bytes.NewBufferString(`{"username":"admin","password":"pass"}`)
		req, _ := http.NewRequest(http.MethodPost, setup.server.URL+"/auth/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", "10.0.0.1")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}

	// 6th request should be rate limited
	body := bytes.NewBufferString(`{"username":"admin","password":"pass"}`)
	req, _ := http.NewRequest(http.MethodPost, setup.server.URL+"/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.0.0.1")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("6th request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on 6th login, got %d", resp.StatusCode)
	}

	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header on rate-limited response")
	}
}

func TestIntegration_RateLimitAPI(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	t.Run("GET rate limit header shows 300", func(t *testing.T) {
		req, _ := setup.sessionRequest(http.MethodGet, "/api/v1/agents", nil, "valid-session")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		limitHeader := resp.Header.Get("X-RateLimit-Limit")
		if limitHeader != "300" {
			t.Fatalf("expected X-RateLimit-Limit=300 for GET, got %q", limitHeader)
		}
	})

	t.Run("POST rate limit header shows 60", func(t *testing.T) {
		agentBody := map[string]interface{}{
			"id":   "ratelimit_post_test",
			"name": "Rate Limit POST Test",
		}
		req, _ := setup.sessionRequest(http.MethodPost, "/api/v1/agents", agentBody, "valid-session")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		limitHeader := resp.Header.Get("X-RateLimit-Limit")
		if limitHeader != "60" {
			t.Fatalf("expected X-RateLimit-Limit=60 for POST, got %q", limitHeader)
		}
	})

	t.Run("PUT rate limit header shows 60", func(t *testing.T) {
		req, _ := setup.sessionRequest(http.MethodPut, "/api/v1/agents/some_agent", map[string]interface{}{"name": "Updated"}, "valid-session")
		req.Header.Set("If-Match", time.Now().UTC().Format(time.RFC3339Nano))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		limitHeader := resp.Header.Get("X-RateLimit-Limit")
		if limitHeader != "60" {
			t.Fatalf("expected X-RateLimit-Limit=60 for PUT, got %q", limitHeader)
		}
	})

	t.Run("DELETE rate limit header shows 60", func(t *testing.T) {
		req, _ := setup.sessionRequest(http.MethodDelete, "/api/v1/agents/some_agent", nil, "valid-session")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		limitHeader := resp.Header.Get("X-RateLimit-Limit")
		if limitHeader != "60" {
			t.Fatalf("expected X-RateLimit-Limit=60 for DELETE, got %q", limitHeader)
		}
	})
}

func TestIntegration_APIKeyAuth(t *testing.T) {
	setup := newIntegTestSetup(t)
	defer setup.close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	req, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer areg_valid_key_12345")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		t.Fatalf("expected 200 with valid API key, got %d; body: %s", resp.StatusCode, body.String())
	}

	var env Envelope
	resp2, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/api/v1/agents", nil)
	resp2.Header.Set("Authorization", "Bearer areg_valid_key_12345")
	httpResp, err := client.Do(resp2)
	if err != nil {
		t.Fatalf("API key request failed: %v", err)
	}
	defer httpResp.Body.Close()

	json.NewDecoder(httpResp.Body).Decode(&env)
	if !env.Success {
		t.Fatal("expected success=true for API key auth")
	}
}
