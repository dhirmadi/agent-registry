package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
	"github.com/agent-smit/agentic-registry/internal/store"
	"github.com/google/uuid"
)

// =============================================================================
// E2E Integration Tests: A2A Protocol + Full Router Stack
//
// These tests exercise A2A endpoints through the full chi router with auth
// middleware, CSRF, rate limiting, and security headers — verifying the
// end-to-end flow from agent mutation through A2A card serving.
// =============================================================================

// --- A2A integration test setup ---

type a2aIntegSetup struct {
	server     *httptest.Server
	agentStore *mockAgentStore
	auditStore *mockAuditStoreForAPI
	sessions   *mockSessionLookupForInteg
	apiKeys    *mockAPIKeyLookupForInteg
	userID     uuid.UUID
}

func newA2AIntegSetup(t *testing.T) *a2aIntegSetup {
	t.Helper()

	userID := uuid.New()
	csrfToken := "test-csrf-token"

	sessions := &mockSessionLookupForInteg{
		sessions: map[string]mockSessionData{
			"admin-session": {
				userID:    userID,
				role:      "admin",
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
			"areg_test_key_abc": {
				userID: userID,
				role:   "admin",
			},
			"areg_viewer_key": {
				userID: uuid.New(),
				role:   "viewer",
			},
		},
	}

	agentStore := newMockAgentStore()
	auditStore := &mockAuditStoreForAPI{}
	authHandler := &mockAuthRouteHandler{}
	rl := ratelimit.NewRateLimiter()

	agentsHandler := NewAgentsHandler(agentStore, auditStore, nil)
	a2aHandler := NewA2AHandler(agentStore, "http://registry.example.com")

	cfg := RouterConfig{
		Health:      &HealthHandler{DB: &mockPinger{}},
		Auth:        authHandler,
		Agents:      agentsHandler,
		A2A:         a2aHandler,
		AuthMW:      AuthMiddleware(sessions, apiKeys),
		RateLimiter: rl,
	}

	router := NewRouter(cfg)
	server := httptest.NewServer(router)

	return &a2aIntegSetup{
		server:     server,
		agentStore: agentStore,
		auditStore: auditStore,
		sessions:   sessions,
		apiKeys:    apiKeys,
		userID:     userID,
	}
}

func (s *a2aIntegSetup) close() {
	s.server.Close()
}

func (s *a2aIntegSetup) adminRequest(method, path string, body interface{}) (*http.Request, error) {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, s.server.URL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName(), Value: "admin-session"})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName(), Value: "test-csrf-token"})
	req.Header.Set("X-CSRF-Token", "test-csrf-token")
	return req, nil
}

func (s *a2aIntegSetup) viewerRequest(method, path string) (*http.Request, error) {
	req, err := http.NewRequest(method, s.server.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName(), Value: "viewer-session"})
	return req, nil
}

func (s *a2aIntegSetup) apiKeyRequest(method, path, key string) (*http.Request, error) {
	req, err := http.NewRequest(method, s.server.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	return req, nil
}

var noRedirectClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// =============================================================================
// E2E Scenario 1: Create Agent → A2A Card Available
// =============================================================================

func TestA2AInteg_CreateAgent_CardAvailable(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Step 1: Create an agent (id must be lowercase, letters/digits/underscores)
	agentBody := map[string]interface{}{
		"id":          "e2e_agent_1",
		"name":        "E2E Test Agent",
		"description": "An agent for E2E testing",
	}

	req, _ := setup.adminRequest(http.MethodPost, "/api/v1/agents", agentBody)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		t.Fatalf("expected 201, got %d; body: %s", resp.StatusCode, body.String())
	}

	// Step 2: Fetch the agent card via A2A endpoint
	req, _ = setup.viewerRequest(http.MethodGet, "/api/v1/agents/e2e_agent_1/agent-card")
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("get agent card failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for agent card, got %d", resp.StatusCode)
	}

	var env Envelope
	json.NewDecoder(resp.Body).Decode(&env)
	if !env.Success {
		t.Fatal("expected success=true for agent card response")
	}

	// Verify card fields
	cardData, _ := json.Marshal(env.Data)
	var card A2AAgentCard
	json.Unmarshal(cardData, &card)

	if card.Name != "E2E Test Agent" {
		t.Errorf("card name = %q, want 'E2E Test Agent'", card.Name)
	}
	if card.Description != "An agent for E2E testing" {
		t.Errorf("card description = %q, want 'An agent for E2E testing'", card.Description)
	}
	if card.ProtocolVersion != "0.3.0" {
		t.Errorf("protocolVersion = %q, want '0.3.0'", card.ProtocolVersion)
	}
	if !strings.Contains(card.URL, "e2e_agent_1") {
		t.Errorf("card URL %q should contain agent ID", card.URL)
	}
	if card.URL != "http://registry.example.com/api/v1/agents/e2e_agent_1" {
		t.Errorf("card URL = %q, want full external URL", card.URL)
	}

	// Step 3: Verify the agent appears in the well-known card
	resp, err = http.Get(setup.server.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("well-known request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("well-known expected 200, got %d", resp.StatusCode)
	}

	var wellKnown A2AAgentCard
	json.NewDecoder(resp.Body).Decode(&wellKnown)

	found := false
	for _, skill := range wellKnown.Skills {
		if skill.ID == "e2e_agent_1" {
			found = true
			if skill.Name != "E2E Test Agent" {
				t.Errorf("well-known skill name = %q, want 'E2E Test Agent'", skill.Name)
			}
			break
		}
	}
	if !found {
		t.Error("created agent not found in well-known card skills")
	}

	// Step 4: Verify the agent appears in the A2A index
	req, _ = setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index")
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("a2a-index request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a2a-index expected 200, got %d", resp.StatusCode)
	}

	var indexEnv Envelope
	json.NewDecoder(resp.Body).Decode(&indexEnv)

	indexData, _ := json.Marshal(indexEnv.Data)
	var indexResult map[string]interface{}
	json.Unmarshal(indexData, &indexResult)

	agentCards, ok := indexResult["agent_cards"].([]interface{})
	if !ok || len(agentCards) == 0 {
		t.Fatal("expected at least one card in a2a-index")
	}
}

// =============================================================================
// E2E Scenario 2: Update Agent → Card Reflects New Data
// =============================================================================

func TestA2AInteg_UpdateAgent_CardUpdated(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed an agent
	now := time.Now().UTC()
	setup.agentStore.agents["update-test"] = &store.Agent{
		ID:             "update-test",
		Name:           "Original Name",
		Description:    "Original description",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Verify original card
	req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/update-test/agent-card")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("get card failed: %v", err)
	}
	defer resp.Body.Close()

	var env Envelope
	json.NewDecoder(resp.Body).Decode(&env)
	cardData, _ := json.Marshal(env.Data)
	var card A2AAgentCard
	json.Unmarshal(cardData, &card)
	if card.Name != "Original Name" {
		t.Fatalf("card name = %q, want 'Original Name'", card.Name)
	}

	// Update the agent
	updateBody := map[string]interface{}{
		"name":        "Updated Name",
		"description": "Updated description",
	}
	req, _ = setup.adminRequest(http.MethodPut, "/api/v1/agents/update-test", updateBody)
	req.Header.Set("If-Match", now.Format(time.RFC3339Nano))
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		t.Fatalf("expected 200 for update, got %d; body: %s", resp.StatusCode, body.String())
	}

	// Verify updated card
	req, _ = setup.viewerRequest(http.MethodGet, "/api/v1/agents/update-test/agent-card")
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("get updated card failed: %v", err)
	}
	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&env)
	cardData, _ = json.Marshal(env.Data)
	json.Unmarshal(cardData, &card)
	if card.Name != "Updated Name" {
		t.Errorf("updated card name = %q, want 'Updated Name'", card.Name)
	}
	if card.Description != "Updated description" {
		t.Errorf("updated card description = %q, want 'Updated description'", card.Description)
	}
	if card.Version != "2" {
		t.Errorf("updated card version = %q, want '2'", card.Version)
	}
}

// =============================================================================
// E2E Scenario 3: Delete Agent → Card Returns 404
// =============================================================================

func TestA2AInteg_DeleteAgent_CardGone(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed an agent
	now := time.Now().UTC()
	setup.agentStore.agents["delete-test"] = &store.Agent{
		ID:             "delete-test",
		Name:           "Doomed Agent",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Verify card exists
	req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/delete-test/agent-card")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("get card failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 before delete, got %d", resp.StatusCode)
	}

	// Delete the agent
	req, _ = setup.adminRequest(http.MethodDelete, "/api/v1/agents/delete-test", nil)
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for delete, got %d", resp.StatusCode)
	}

	// The individual card is still fetchable by ID (agent record exists, just inactive).
	// What matters is that it's excluded from the well-known and index views.

	// Verify agent is gone from well-known
	resp, err = http.Get(setup.server.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("well-known request failed: %v", err)
	}
	defer resp.Body.Close()

	var wellKnown A2AAgentCard
	json.NewDecoder(resp.Body).Decode(&wellKnown)
	for _, skill := range wellKnown.Skills {
		if skill.ID == "delete-test" {
			t.Error("deleted agent should not appear in well-known card")
		}
	}
}

// =============================================================================
// E2E Scenario 4: Auth Integration — Well-Known is Public, Index Requires Auth
// =============================================================================

func TestA2AInteg_AuthBoundaries(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	t.Run("well-known is public (no auth required)", func(t *testing.T) {
		resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for unauthenticated well-known, got %d", resp.StatusCode)
		}
	})

	t.Run("a2a-index requires authentication", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/api/v1/agents/a2a-index", nil)
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated a2a-index, got %d", resp.StatusCode)
		}
	})

	t.Run("agent-card requires authentication", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/api/v1/agents/any-agent/agent-card", nil)
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated agent-card, got %d", resp.StatusCode)
		}
	})

	t.Run("a2a-index accessible via API key", func(t *testing.T) {
		req, _ := setup.apiKeyRequest(http.MethodGet, "/api/v1/agents/a2a-index", "areg_viewer_key")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for API key a2a-index, got %d", resp.StatusCode)
		}
	})

	t.Run("a2a-index accessible via session (viewer role)", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for viewer session a2a-index, got %d", resp.StatusCode)
		}
	})
}

// =============================================================================
// E2E Scenario 5: Search Filters A2A Index
// =============================================================================

func TestA2AInteg_SearchFilters(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed multiple agents
	now := time.Now().UTC()
	agents := []struct {
		id   string
		name string
		desc string
	}{
		{"search-alpha", "Alpha Agent", "Handles alpha operations"},
		{"search-beta", "Beta Agent", "Handles beta operations"},
		{"search-gamma", "Gamma Agent", "Handles alpha-related tasks"},
	}

	for _, a := range agents {
		setup.agentStore.agents[a.id] = &store.Agent{
			ID:             a.id,
			Name:           a.name,
			Description:    a.desc,
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       true,
			Version:        1,
			CreatedBy:      "system",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	t.Run("search by name matches", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=Alpha")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		// "Alpha" matches "Alpha Agent" by name and "Gamma Agent" by description ("alpha-related")
		if len(cards) < 1 {
			t.Fatal("expected at least 1 matching card for 'Alpha'")
		}

		// Every card should contain "alpha" in name or description
		for _, c := range cards {
			cMap := c.(map[string]interface{})
			name := strings.ToLower(cMap["name"].(string))
			desc := strings.ToLower(cMap["description"].(string))
			if !strings.Contains(name, "alpha") && !strings.Contains(desc, "alpha") {
				t.Errorf("card %q/%q should not match 'Alpha'", cMap["name"], cMap["description"])
			}
		}
	})

	t.Run("search with no results returns empty", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=nonexistent")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		if len(cards) != 0 {
			t.Errorf("expected 0 cards for 'nonexistent', got %d", len(cards))
		}
	})

	t.Run("search is case-insensitive", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=bEtA")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		if len(cards) != 1 {
			t.Fatalf("expected 1 card for 'bEtA', got %d", len(cards))
		}
	})
}

// =============================================================================
// E2E Scenario 6: Well-Known ETag Conditional Requests
// =============================================================================

func TestA2AInteg_WellKnown_ETagConditional(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed an agent
	now := time.Now().UTC()
	setup.agentStore.agents["etag-test"] = &store.Agent{
		ID:             "etag-test",
		Name:           "ETag Agent",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// First request — get the ETag
	resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header on well-known response")
	}

	cacheControl := resp.Header.Get("Cache-Control")
	if !strings.Contains(cacheControl, "max-age=60") {
		t.Errorf("Cache-Control = %q, want 'public, max-age=60'", cacheControl)
	}

	// Second request with If-None-Match — should get 304
	req, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/.well-known/agent.json", nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("conditional request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304 with matching ETag, got %d", resp2.StatusCode)
	}

	// Third request with stale ETag — should get 200
	req, _ = http.NewRequest(http.MethodGet, setup.server.URL+"/.well-known/agent.json", nil)
	req.Header.Set("If-None-Match", `"stale-etag"`)
	resp3, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("stale etag request failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with stale ETag, got %d", resp3.StatusCode)
	}
}

// =============================================================================
// E2E Scenario 7: Security Headers on A2A Responses
// =============================================================================

func TestA2AInteg_SecurityHeaders(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Test well-known endpoint
	resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	headers := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":          "DENY",
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}

	for name, want := range headers {
		got := resp.Header.Get(name)
		if got != want {
			t.Errorf("well-known %s = %q, want %q", name, got, want)
		}
	}

	// Verify Content-Type is application/json
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("well-known Content-Type = %q, want application/json", ct)
	}
}

// =============================================================================
// E2E Scenario 8: Special Characters in Agent Names
// =============================================================================

func TestA2AInteg_SpecialCharacters(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	tests := []struct {
		id   string
		name string
		desc string
	}{
		{"unicode-agent", "Agente Español", "Maneja operaciones con señales"},
		{"emoji-agent", "Test Agent", "Handles <script>alert(1)</script> safely"},
		{"quotes-agent", `Agent "Double"`, `Description with 'single' and "double" quotes`},
	}

	now := time.Now().UTC()
	for _, tc := range tests {
		setup.agentStore.agents[tc.id] = &store.Agent{
			ID:             tc.id,
			Name:           tc.name,
			Description:    tc.desc,
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       true,
			Version:        1,
			CreatedBy:      "system",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/"+tc.id+"/agent-card")
			resp, err := noRedirectClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}

			var env Envelope
			json.NewDecoder(resp.Body).Decode(&env)
			cardData, _ := json.Marshal(env.Data)
			var card A2AAgentCard
			json.Unmarshal(cardData, &card)

			if card.Name != tc.name {
				t.Errorf("name = %q, want %q", card.Name, tc.name)
			}
			if card.Description != tc.desc {
				t.Errorf("description = %q, want %q", card.Description, tc.desc)
			}
		})
	}

	// Also verify the well-known JSON is valid despite special chars
	resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("well-known request failed: %v", err)
	}
	defer resp.Body.Close()

	var wellKnown A2AAgentCard
	if err := json.NewDecoder(resp.Body).Decode(&wellKnown); err != nil {
		t.Fatalf("well-known JSON decode failed with special chars: %v", err)
	}
	if len(wellKnown.Skills) != len(tests) {
		t.Errorf("well-known skills = %d, want %d", len(wellKnown.Skills), len(tests))
	}
}

// =============================================================================
// E2E Scenario 9: Pagination at Scale
// =============================================================================

func TestA2AInteg_PaginationAtScale(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed 50 agents
	now := time.Now().UTC()
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("scale-agent-%03d", i)
		setup.agentStore.agents[id] = &store.Agent{
			ID:             id,
			Name:           fmt.Sprintf("Scale Agent %d", i),
			Description:    fmt.Sprintf("Scale test agent number %d", i),
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       true,
			Version:        1,
			CreatedBy:      "system",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	t.Run("default limit returns 20", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		total := int(result["total"].(float64))

		if len(cards) != 20 {
			t.Errorf("expected 20 cards (default limit), got %d", len(cards))
		}
		if total != 50 {
			t.Errorf("expected total=50, got %d", total)
		}
	})

	t.Run("custom limit respected", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=10")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		if len(cards) != 10 {
			t.Errorf("expected 10 cards, got %d", len(cards))
		}
	})

	t.Run("limit capped at 200", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=999")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		// With 50 agents and limit capped at 200, should get all 50
		if len(cards) != 50 {
			t.Errorf("expected 50 cards (all), got %d", len(cards))
		}
	})

	t.Run("offset past total returns empty", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index?offset=100")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		cards := result["agent_cards"].([]interface{})
		if len(cards) != 0 {
			t.Errorf("expected 0 cards past total, got %d", len(cards))
		}
	})

	t.Run("well-known handles 50 agents", func(t *testing.T) {
		resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var wellKnown A2AAgentCard
		if err := json.NewDecoder(resp.Body).Decode(&wellKnown); err != nil {
			t.Fatalf("JSON decode failed: %v", err)
		}
		if len(wellKnown.Skills) != 50 {
			t.Errorf("well-known skills = %d, want 50", len(wellKnown.Skills))
		}
	})
}

// =============================================================================
// E2E Scenario 10: Concurrent Reads Through Full Stack
// =============================================================================

func TestA2AInteg_ConcurrentReads(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed agents
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("concurrent-%d", i)
		setup.agentStore.agents[id] = &store.Agent{
			ID:             id,
			Name:           fmt.Sprintf("Concurrent Agent %d", i),
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       true,
			Version:        1,
			CreatedBy:      "system",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*3)

	for i := 0; i < goroutines; i++ {
		// Well-known (public, no auth)
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
			if err != nil {
				errCh <- fmt.Errorf("well-known: %w", err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("well-known: status %d", resp.StatusCode)
			}
		}()

		// A2A index (API key auth)
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/api/v1/agents/a2a-index", nil)
			req.Header.Set("Authorization", "Bearer areg_test_key_abc")
			resp, err := noRedirectClient.Do(req)
			if err != nil {
				errCh <- fmt.Errorf("a2a-index: %w", err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("a2a-index: status %d", resp.StatusCode)
			}
		}()

		// Agent card (API key auth)
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx%5)
			req, _ := http.NewRequest(http.MethodGet, setup.server.URL+"/api/v1/agents/"+id+"/agent-card", nil)
			req.Header.Set("Authorization", "Bearer areg_test_key_abc")
			resp, err := noRedirectClient.Do(req)
			if err != nil {
				errCh <- fmt.Errorf("agent-card %s: %w", id, err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("agent-card %s: status %d", id, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

// =============================================================================
// E2E Scenario 11: A2A Card Structural Integrity Through Full Stack
// =============================================================================

func TestA2AInteg_CardStructuralIntegrity(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Seed an agent with tools
	now := time.Now().UTC()
	setup.agentStore.agents["struct-test"] = &store.Agent{
		ID:          "struct-test",
		Name:        "Structured Agent",
		Description: "Agent with tools and prompts",
		Tools: json.RawMessage(`[
			{"name":"web_search","description":"Search the web","source":"internal"},
			{"name":"file_edit","description":"Edit files","source":"mcp","server_label":"editor"}
		]`),
		TrustOverrides: json.RawMessage(`{"allow_write": true}`),
		ExamplePrompts: json.RawMessage(`["Search for weather", "Edit config file"]`),
		IsActive:       true,
		Version:        7,
		CreatedBy:      "admin",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/struct-test/agent-card")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var env Envelope
	json.NewDecoder(resp.Body).Decode(&env)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	cardData, _ := json.Marshal(env.Data)
	var card A2AAgentCard
	json.Unmarshal(cardData, &card)

	// Verify all A2A protocol fields
	if card.Name != "Structured Agent" {
		t.Errorf("name = %q", card.Name)
	}
	if card.Version != "7" {
		t.Errorf("version = %q, want '7'", card.Version)
	}
	if card.ProtocolVersion != "0.3.0" {
		t.Errorf("protocolVersion = %q", card.ProtocolVersion)
	}
	if card.Provider.Organization != "Agentic Registry" {
		t.Errorf("provider.organization = %q", card.Provider.Organization)
	}
	if card.Provider.URL != "http://registry.example.com" {
		t.Errorf("provider.url = %q", card.Provider.URL)
	}

	// Verify skills derived from tools
	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills (from tools), got %d", len(card.Skills))
	}
	if card.Skills[0].ID != "web_search" {
		t.Errorf("skill[0].id = %q, want 'web_search'", card.Skills[0].ID)
	}
	if card.Skills[0].Tags[0] != "internal" {
		t.Errorf("skill[0].tags[0] = %q, want 'internal'", card.Skills[0].Tags[0])
	}
	if card.Skills[1].ID != "file_edit" {
		t.Errorf("skill[1].id = %q, want 'file_edit'", card.Skills[1].ID)
	}
	// Second skill should have source + server_label as tags
	if len(card.Skills[1].Tags) != 2 || card.Skills[1].Tags[1] != "editor" {
		t.Errorf("skill[1].tags = %v, want [mcp, editor]", card.Skills[1].Tags)
	}

	// Example prompts attached to first skill only
	if len(card.Skills[0].Examples) != 2 {
		t.Errorf("skill[0].examples = %d, want 2", len(card.Skills[0].Examples))
	}
	if len(card.Skills[1].Examples) != 0 {
		t.Errorf("skill[1].examples = %d, want 0", len(card.Skills[1].Examples))
	}

	// Security schemes
	if _, ok := card.SecuritySchemes["bearerAuth"]; !ok {
		t.Error("missing bearerAuth in securitySchemes")
	}
	if card.SecuritySchemes["bearerAuth"].Type != "http" {
		t.Error("bearerAuth.type should be 'http'")
	}

	// Default modes
	if len(card.DefaultInputModes) != 1 || card.DefaultInputModes[0] != "text" {
		t.Errorf("defaultInputModes = %v", card.DefaultInputModes)
	}
}

// =============================================================================
// E2E Scenario 12: Inactive Agents Excluded from A2A
// =============================================================================

func TestA2AInteg_InactiveAgentsExcluded(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	now := time.Now().UTC()
	setup.agentStore.agents["active-agent"] = &store.Agent{
		ID:             "active-agent",
		Name:           "Active Agent",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	setup.agentStore.agents["inactive-agent"] = &store.Agent{
		ID:             "inactive-agent",
		Name:           "Inactive Agent",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       false,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	t.Run("well-known excludes inactive", func(t *testing.T) {
		resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var card A2AAgentCard
		json.NewDecoder(resp.Body).Decode(&card)

		for _, skill := range card.Skills {
			if skill.ID == "inactive-agent" {
				t.Error("inactive agent should not appear in well-known")
			}
		}

		found := false
		for _, skill := range card.Skills {
			if skill.ID == "active-agent" {
				found = true
			}
		}
		if !found {
			t.Error("active agent should appear in well-known")
		}
	})

	t.Run("a2a-index excludes inactive", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		data, _ := json.Marshal(env.Data)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		total := int(result["total"].(float64))
		if total != 1 {
			t.Errorf("expected total=1 (active only), got %d", total)
		}
	})
}

// =============================================================================
// E2E Scenario 13: Response Envelope Consistency
// =============================================================================

func TestA2AInteg_EnvelopeConsistency(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	now := time.Now().UTC()
	setup.agentStore.agents["envelope-test"] = &store.Agent{
		ID:             "envelope-test",
		Name:           "Envelope Agent",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	t.Run("agent-card returns envelope with meta", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/envelope-test/agent-card")
		// Set X-Request-Id to verify it propagates into the envelope
		req.Header.Set("X-Request-Id", "test-req-001")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)

		if !env.Success {
			t.Error("expected success=true")
		}
		if env.Error != nil {
			t.Error("expected error=nil for success response")
		}
		if env.Meta.RequestID != "test-req-001" {
			t.Errorf("meta.request_id = %q, want 'test-req-001'", env.Meta.RequestID)
		}
		if env.Meta.Timestamp == "" {
			t.Error("expected non-empty meta.timestamp")
		}
	})

	t.Run("a2a-index returns envelope with meta", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index")
		req.Header.Set("X-Request-Id", "test-req-002")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)

		if !env.Success {
			t.Error("expected success=true")
		}
		if env.Meta.RequestID != "test-req-002" {
			t.Errorf("meta.request_id = %q, want 'test-req-002'", env.Meta.RequestID)
		}
	})

	t.Run("well-known returns raw JSON (no envelope)", func(t *testing.T) {
		resp, err := http.Get(setup.server.URL + "/.well-known/agent.json")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var raw map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&raw)

		// Well-known returns raw A2A card, not an envelope
		if _, hasSuccess := raw["success"]; hasSuccess {
			t.Error("well-known should return raw JSON, not an envelope")
		}
		if _, hasName := raw["name"]; !hasName {
			t.Error("well-known should have 'name' field (A2A card)")
		}
		if _, hasProtocol := raw["protocolVersion"]; !hasProtocol {
			t.Error("well-known should have 'protocolVersion' field")
		}
	})

	t.Run("404 agent-card returns error envelope", func(t *testing.T) {
		req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/nonexistent/agent-card")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}

		var env Envelope
		json.NewDecoder(resp.Body).Decode(&env)
		if env.Success {
			t.Error("expected success=false for 404")
		}
		if env.Error == nil {
			t.Error("expected error object in 404 response")
		}
	})
}

// =============================================================================
// E2E Scenario 14: A2A Card from Agent Created via API Key
// =============================================================================

func TestA2AInteg_APIKeyCreateThenCard(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	// Create agent via API key (id must be lowercase letters/digits/underscores)
	agentBody := map[string]interface{}{
		"id":          "apikey_created",
		"name":        "API Key Agent",
		"description": "Created via service-to-service auth",
	}

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(agentBody)

	req, _ := http.NewRequest(http.MethodPost, setup.server.URL+"/api/v1/agents", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer areg_test_key_abc")

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		t.Fatalf("expected 201, got %d; body: %s", resp.StatusCode, body.String())
	}

	// Fetch card via different auth method (session)
	req, _ = setup.viewerRequest(http.MethodGet, "/api/v1/agents/apikey_created/agent-card")
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("get card failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var env Envelope
	json.NewDecoder(resp.Body).Decode(&env)
	cardData, _ := json.Marshal(env.Data)
	var card A2AAgentCard
	json.Unmarshal(cardData, &card)

	if card.Name != "API Key Agent" {
		t.Errorf("name = %q, want 'API Key Agent'", card.Name)
	}
}

// =============================================================================
// E2E Scenario 15: Rate Limit Headers on A2A Index
// =============================================================================

func TestA2AInteg_RateLimitHeaders(t *testing.T) {
	setup := newA2AIntegSetup(t)
	defer setup.close()

	req, _ := setup.viewerRequest(http.MethodGet, "/api/v1/agents/a2a-index")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// GET should have read rate limit (300/min)
	limitHeader := resp.Header.Get("X-RateLimit-Limit")
	if limitHeader != "300" {
		t.Errorf("X-RateLimit-Limit = %q, want '300' for GET", limitHeader)
	}

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}
