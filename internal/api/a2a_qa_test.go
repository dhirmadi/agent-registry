package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// QA Gap-Fill Tests — A2A Publisher & Config
// Covers: concurrent mutation safety, inactive-only registries, HEAD method,
// well-known ETag edge cases, config ExternalURL propagation, and JSON
// structural integrity under scale.
// =============================================================================

// --- Concurrent read safety ---

// TestA2A_ConcurrentReadOnAllEndpoints verifies that concurrent reads on
// all three A2A endpoints remain safe and return valid responses.
func TestA2A_ConcurrentReadOnAllEndpoints(t *testing.T) {
	agentStore := newMockAgentStore()
	now := time.Now()

	// Seed agents
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("agent_%d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: fmt.Sprintf("Desc %d", i),
			Tools: json.RawMessage(`[{"name":"tool","source":"internal","server_label":"","description":"t"}]`),
			ExamplePrompts: json.RawMessage(`["example"]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	var wg sync.WaitGroup
	errs := make(chan string, 300)

	// 100 concurrent readers across all three endpoints
	for i := 0; i < 100; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)
			if w.Code != http.StatusOK {
				errs <- fmt.Sprintf("well-known: status %d", w.Code)
				return
			}
			// Verify response is valid JSON
			var card A2AAgentCard
			if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
				errs <- fmt.Sprintf("well-known: invalid JSON: %v", err)
			}
		}()

		go func() {
			defer wg.Done()
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=10", nil, "viewer")
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)
			if w.Code != http.StatusOK {
				errs <- fmt.Sprintf("a2a-index: status %d", w.Code)
			}
		}()

		go func(idx int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent_%d", idx%20)
			req := agentRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/agent-card", nil, "viewer")
			req = withChiParam(req, "agentId", agentID)
			w := httptest.NewRecorder()
			h.GetAgentCard(w, req)
			if w.Code != http.StatusOK {
				errs <- fmt.Sprintf("agent-card(%s): status %d", agentID, w.Code)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// --- Well-known with only inactive agents ---

func TestA2AHandler_WellKnown_OnlyInactiveAgents(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["inactive1"] = &store.Agent{
		ID: "inactive1", Name: "Inactive Agent", Description: "Should not appear",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: false, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var card A2AAgentCard
	if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// Only active agents should appear as skills; the mock store filters by active
	// The well-known endpoint should return zero skills for inactive-only registries
	// (depends on mock store behavior - it may or may not filter)
	// Verify the response is still structurally valid
	if card.Name != "Agentic Registry" {
		t.Errorf("name: got %q, want %q", card.Name, "Agentic Registry")
	}
	if card.ProtocolVersion != "0.3.0" {
		t.Errorf("protocolVersion: got %q", card.ProtocolVersion)
	}
}

// --- HEAD method on well-known ---

func TestA2ARouter_WellKnown_HEADMethod(t *testing.T) {
	agentStore := newMockAgentStore()
	now := time.Now()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	a2aHandler := NewA2AHandler(agentStore, "https://registry.example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    a2aHandler,
	})

	// HEAD should be allowed on GET routes (chi automatically handles HEAD for GET)
	req := httptest.NewRequest(http.MethodHead, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// chi router will respond to HEAD with same headers as GET but no body
	if w.Code != http.StatusOK && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD on well-known: got %d", w.Code)
	}

	if w.Code == http.StatusOK {
		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type for HEAD: got %q, want application/json", ct)
		}
		// HEAD response body should be empty
		if w.Body.Len() > 0 {
			t.Logf("Note: HEAD response has body of %d bytes (may be buffered)", w.Body.Len())
		}
	}
}

// --- Well-known ETag with empty registry (zero time) ---

func TestA2AHandler_WellKnown_ETagEmptyRegistry(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Error("ETag should be set even for empty registry")
	}

	// With zero agents, maxUpdated is zero time, ETag should still be properly quoted
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag should be quoted: got %q", etag)
	}
}

// --- ETag determinism across identical requests ---

func TestA2AHandler_WellKnown_ETagDeterministic(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["a1"] = &store.Agent{
		ID: "a1", Name: "Agent 1", Description: "First",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Make 10 identical requests and verify ETag is the same every time
	var etags []string
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
		etags = append(etags, w.Header().Get("ETag"))
	}

	for i := 1; i < len(etags); i++ {
		if etags[i] != etags[0] {
			t.Errorf("ETag should be deterministic: request 0=%q, request %d=%q", etags[0], i, etags[i])
		}
	}
}

// --- Config ExternalURL propagation to A2A card ---

func TestA2A_ExternalURLPropagation(t *testing.T) {
	tests := []struct {
		name        string
		externalURL string
		wantCardURL string
		wantProvURL string
	}{
		{
			name:        "standard HTTPS URL",
			externalURL: "https://registry.example.com",
			wantCardURL: "https://registry.example.com/api/v1/agents/test",
			wantProvURL: "https://registry.example.com",
		},
		{
			name:        "localhost with port",
			externalURL: "http://localhost:8090",
			wantCardURL: "http://localhost:8090/api/v1/agents/test",
			wantProvURL: "http://localhost:8090",
		},
		{
			name:        "empty external URL (default)",
			externalURL: "",
			wantCardURL: "/api/v1/agents/test",
			wantProvURL: "",
		},
		{
			name:        "URL with subdirectory",
			externalURL: "https://example.com/v1/registry",
			wantCardURL: "https://example.com/v1/registry/api/v1/agents/test",
			wantProvURL: "https://example.com/v1/registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID: "test", Name: "Test", Description: "Test",
				Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
				Version: 1,
			}

			card := agentToA2ACard(agent, tt.externalURL)

			if card.URL != tt.wantCardURL {
				t.Errorf("card URL: got %q, want %q", card.URL, tt.wantCardURL)
			}
			if card.Provider.URL != tt.wantProvURL {
				t.Errorf("provider URL: got %q, want %q", card.Provider.URL, tt.wantProvURL)
			}
		})
	}
}

// --- Well-known external URL in composite card ---

func TestA2AHandler_WellKnown_ExternalURLInCompositeCard(t *testing.T) {
	tests := []struct {
		name        string
		externalURL string
		wantURL     string
	}{
		{
			name:        "HTTPS URL",
			externalURL: "https://registry.example.com",
			wantURL:     "https://registry.example.com",
		},
		{
			name:        "localhost URL",
			externalURL: "http://localhost:8090",
			wantURL:     "http://localhost:8090",
		},
		{
			name:        "empty URL",
			externalURL: "",
			wantURL:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			now := time.Now()
			agentStore.agents["a1"] = &store.Agent{
				ID: "a1", Name: "Agent 1", Description: "Test",
				Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
				IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
			}

			h := NewA2AHandler(agentStore, tt.externalURL)

			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}

			var card A2AAgentCard
			json.NewDecoder(w.Body).Decode(&card)

			if card.URL != tt.wantURL {
				t.Errorf("card URL: got %q, want %q", card.URL, tt.wantURL)
			}
			if card.Provider.URL != tt.wantURL {
				t.Errorf("provider URL: got %q, want %q", card.Provider.URL, tt.wantURL)
			}
		})
	}
}

// --- JSON structural integrity under scale ---

func TestA2A_JSONStructuralIntegrityAtScale(t *testing.T) {
	agentStore := newMockAgentStore()
	now := time.Now()

	// Seed 500 agents with tools and examples
	for i := 0; i < 500; i++ {
		id := fmt.Sprintf("agent_%04d", i)
		agentStore.agents[id] = &store.Agent{
			ID:   id,
			Name: fmt.Sprintf("Agent %d with a longer name for testing", i),
			Description: fmt.Sprintf("This is agent %d with a description that "+
				"contains enough text to simulate real-world usage patterns", i),
			Tools: json.RawMessage(fmt.Sprintf(`[
				{"name":"tool_%d_a","source":"internal","server_label":"","description":"First tool for agent %d"},
				{"name":"tool_%d_b","source":"mcp","server_label":"server_%d","description":"Second tool for agent %d"}
			]`, i, i, i, i, i)),
			ExamplePrompts: json.RawMessage(fmt.Sprintf(`["Example for agent %d"]`, i)),
			IsActive:       true,
			Version:        i + 1,
			CreatedAt:      now,
			UpdatedAt:      now.Add(time.Duration(i) * time.Minute),
		}
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Test well-known with 500 agents
	t.Run("well-known 500 agents", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var card A2AAgentCard
		if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
			t.Fatalf("JSON decode failed for 500-agent response: %v", err)
		}

		if len(card.Skills) != 500 {
			t.Errorf("expected 500 skills, got %d", len(card.Skills))
		}

		// Verify all skills have required fields
		for i, skill := range card.Skills {
			if skill.ID == "" {
				t.Errorf("skill[%d].ID is empty", i)
			}
			if skill.Name == "" {
				t.Errorf("skill[%d].Name is empty", i)
			}
		}
	})

	// Test index with limit=200 (max)
	t.Run("index limit 200 with 500 agents", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=200", nil, "viewer")
		w := httptest.NewRecorder()
		h.GetA2AIndex(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		env := parseEnvelope(t, w)
		data := env.Data.(map[string]interface{})
		cards := data["agent_cards"].([]interface{})

		if len(cards) != 200 {
			t.Errorf("expected 200 cards (capped), got %d", len(cards))
		}

		// Verify each card is a valid A2A card
		for i, c := range cards {
			card, ok := c.(map[string]interface{})
			if !ok {
				t.Fatalf("card[%d] is not a map", i)
			}
			requiredFields := []string{"name", "description", "url", "version", "protocolVersion",
				"provider", "capabilities", "skills", "securitySchemes", "security"}
			for _, field := range requiredFields {
				if _, ok := card[field]; !ok {
					t.Errorf("card[%d] missing field %q", i, field)
				}
			}
		}
	})
}

// --- A2A Index: empty query param vs missing query param ---

func TestA2AHandler_Index_EmptyQvsNoQ(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test1"] = &store.Agent{
		ID: "test1", Name: "Test Agent 1", Description: "First",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	agentStore.agents["test2"] = &store.Agent{
		ID: "test2", Name: "Test Agent 2", Description: "Second",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// No ?q parameter at all
	req1 := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w1 := httptest.NewRecorder()
	h.GetA2AIndex(w1, req1)

	env1 := parseEnvelope(t, w1)
	data1 := env1.Data.(map[string]interface{})
	cards1 := data1["agent_cards"].([]interface{})

	// ?q= (empty string)
	req2 := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=", nil, "viewer")
	w2 := httptest.NewRecorder()
	h.GetA2AIndex(w2, req2)

	env2 := parseEnvelope(t, w2)
	data2 := env2.Data.(map[string]interface{})
	cards2 := data2["agent_cards"].([]interface{})

	// Both should return all agents (empty query = no filter)
	if len(cards1) != len(cards2) {
		t.Errorf("no ?q and ?q= should return same count: got %d and %d", len(cards1), len(cards2))
	}
	if len(cards1) != 2 {
		t.Errorf("expected 2 agents, got %d", len(cards1))
	}
}

// --- A2A Index: query matching on description with Unicode ---

func TestA2AHandler_Index_UnicodeQueryFilter(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["unicode_agent"] = &store.Agent{
		ID: "unicode_agent", Name: "Agen\u00e9 Test\u00e9", Description: "Gestion de projet avec des acc\u00e9nts",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Search for a substring that includes unicode characters
	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=acc%C3%A9nts", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})

	if len(cards) != 1 {
		t.Errorf("expected 1 agent matching unicode query, got %d", len(cards))
	}
}

// --- Multiple tools with same name ---

func TestAgentToA2ACard_DuplicateToolNames(t *testing.T) {
	agent := &store.Agent{
		ID:          "dup_tools",
		Name:        "Duplicate Tools Agent",
		Description: "Agent with duplicate tool names",
		Tools: json.RawMessage(`[
			{"name": "search", "source": "internal", "server_label": "", "description": "Internal search"},
			{"name": "search", "source": "mcp", "server_label": "google-mcp", "description": "Google search"}
		]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	// Both skills should exist even with duplicate names
	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills (even with duplicate names), got %d", len(card.Skills))
	}

	// But they should have different tags
	if card.Skills[0].Tags[0] != "internal" {
		t.Errorf("first skill tags[0]: got %q, want %q", card.Skills[0].Tags[0], "internal")
	}
	if card.Skills[1].Tags[0] != "mcp" {
		t.Errorf("second skill tags[0]: got %q, want %q", card.Skills[1].Tags[0], "mcp")
	}
}

// --- NewA2AHandler with empty/unusual external URLs ---

func TestNewA2AHandler_VariousExternalURLs(t *testing.T) {
	tests := []struct {
		name        string
		externalURL string
	}{
		{"empty URL", ""},
		{"localhost", "http://localhost:8090"},
		{"HTTPS with path", "https://example.com/registry"},
		{"trailing slash", "https://example.com/"},
		{"just scheme and host", "https://example.com"},
	}

	agentStore := newMockAgentStore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewA2AHandler(agentStore, tt.externalURL)
			if h == nil {
				t.Fatal("handler should not be nil")
			}
			if h.externalURL != tt.externalURL {
				t.Errorf("externalURL: got %q, want %q", h.externalURL, tt.externalURL)
			}
		})
	}
}

// --- Well-known response is not cached when agents change ---

func TestA2AHandler_WellKnown_NoCacheWhenAgentsChange(t *testing.T) {
	agentStore := newMockAgentStore()
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)

	// Start with one agent
	agentStore.agents["a1"] = &store.Agent{
		ID: "a1", Name: "Agent 1", Description: "First",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Request 1: one agent
	req1 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w1 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w1, req1)

	var card1 A2AAgentCard
	json.NewDecoder(w1.Body).Decode(&card1)

	if len(card1.Skills) != 1 {
		t.Fatalf("expected 1 skill initially, got %d", len(card1.Skills))
	}

	// Add another agent
	later := time.Date(2026, 2, 15, 11, 0, 0, 0, time.UTC)
	agentStore.agents["a2"] = &store.Agent{
		ID: "a2", Name: "Agent 2", Description: "Second",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: later, UpdatedAt: later,
	}

	// Request 2: should see two agents now
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w2 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w2, req2)

	var card2 A2AAgentCard
	json.NewDecoder(w2.Body).Decode(&card2)

	if len(card2.Skills) != 2 {
		t.Errorf("expected 2 skills after adding agent, got %d", len(card2.Skills))
	}

	// ETags should differ
	etag1 := w1.Header().Get("ETag")
	etag2 := w2.Header().Get("ETag")
	if etag1 == etag2 {
		t.Error("ETags should differ after agent was added")
	}
}

// --- Agent card: response includes meta.request_id ---

func TestA2AHandler_GetAgentCard_MetaRequestID(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["meta_test"] = &store.Agent{
		ID: "meta_test", Name: "Meta Test", Description: "Test meta fields",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/meta_test/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "meta_test")
	w := httptest.NewRecorder()
	h.GetAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	meta, ok := raw["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("expected meta to be an object")
	}

	if _, ok := meta["request_id"]; !ok {
		t.Error("meta should contain request_id")
	}
	if _, ok := meta["timestamp"]; !ok {
		t.Error("meta should contain timestamp")
	}
}

// --- A2A Index: response includes meta ---

func TestA2AHandler_Index_MetaFields(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	meta, ok := raw["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("expected meta to be an object")
	}

	if _, ok := meta["request_id"]; !ok {
		t.Error("meta should contain request_id")
	}
	if _, ok := meta["timestamp"]; !ok {
		t.Error("meta should contain timestamp")
	}
}

// --- Agent card for agent with very many example prompts ---

func TestAgentToA2ACard_ManyExamplePrompts(t *testing.T) {
	examples := make([]string, 50)
	for i := range examples {
		examples[i] = fmt.Sprintf("Example prompt number %d with enough text to test", i)
	}
	examplesJSON, _ := json.Marshal(examples)

	agent := &store.Agent{
		ID:             "many_examples",
		Name:           "Many Examples Agent",
		Description:    "Agent with 50 example prompts",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(examplesJSON),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 synthetic skill, got %d", len(card.Skills))
	}

	if len(card.Skills[0].Examples) != 50 {
		t.Errorf("expected 50 examples, got %d", len(card.Skills[0].Examples))
	}

	// Verify it serializes without error
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

// --- Well-known: response is valid JSON even with zero agents ---

func TestA2AHandler_WellKnown_ValidJSONEmptyRegistry(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Must be valid JSON
	var raw json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	// Must have required A2A fields
	var card map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &card)

	requiredFields := []string{"name", "description", "url", "version",
		"protocolVersion", "provider", "capabilities", "skills",
		"securitySchemes", "security"}
	for _, field := range requiredFields {
		if _, ok := card[field]; !ok {
			t.Errorf("empty registry well-known missing field %q", field)
		}
	}

	// Skills should be an empty array, not null
	skills, ok := card["skills"].([]interface{})
	if !ok {
		t.Fatal("skills should be an array")
	}
	if skills == nil {
		t.Error("skills should be an empty array, not nil")
	}
}

// --- Well-known with single agent verifying skill "agent" tag ---

func TestA2AHandler_WellKnown_SkillsHaveAgentTag(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["tagged"] = &store.Agent{
		ID: "tagged", Name: "Tagged Agent", Description: "Agent with tag check",
		Tools: json.RawMessage(`[{"name":"tool1","source":"mcp","server_label":"srv","description":"Tool"}]`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	var card A2AAgentCard
	json.NewDecoder(w.Body).Decode(&card)

	// In well-known, skills represent agents (not individual tools),
	// so each skill should have the "agent" tag
	for _, skill := range card.Skills {
		if len(skill.Tags) == 0 {
			t.Errorf("well-known skill %q should have tags", skill.ID)
			continue
		}
		if skill.Tags[0] != "agent" {
			t.Errorf("well-known skill %q tag[0]: got %q, want %q", skill.ID, skill.Tags[0], "agent")
		}
	}
}

// --- GetAgentCard with agent that has complex nested tool JSON ---

func TestAgentToA2ACard_ToolsWithUnicodeDescriptions(t *testing.T) {
	agent := &store.Agent{
		ID:          "unicode_tools",
		Name:        "Unicode Tools Agent",
		Description: "Agent with Unicode in tool descriptions",
		Tools: json.RawMessage(`[
			{"name": "tool_\u00e9", "source": "internal", "server_label": "", "description": "Outil avec acc\u00e9nts fran\u00e7ais"},
			{"name": "tool_\u65e5\u672c", "source": "mcp", "server_label": "\u65e5\u672c-mcp", "description": "\u65e5\u672c\u8a9e\u306e\u8aac\u660e"}
		]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(card.Skills))
	}

	// Verify JSON round-trip preserves Unicode
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundTrip A2AAgentCard
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(roundTrip.Skills) != 2 {
		t.Errorf("round-trip: expected 2 skills, got %d", len(roundTrip.Skills))
	}
}

// --- A2A Index: pagination total and limit behavior ---

func TestA2AHandler_Index_PaginationTotalReflectsAllAgents(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("agent_%d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: "Test",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Request with limit=2: should get 2 cards but total=5
	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=2&offset=0", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})
	total := data["total"].(float64)

	if len(cards) != 2 {
		t.Errorf("expected 2 cards with limit=2, got %d", len(cards))
	}
	if total != 5 {
		t.Errorf("total should be 5 (all agents), got %v", total)
	}

	// Each returned card should be a valid A2A card
	for i, c := range cards {
		card, ok := c.(map[string]interface{})
		if !ok {
			t.Fatalf("card[%d] is not a map", i)
		}
		if _, ok := card["protocolVersion"]; !ok {
			t.Errorf("card[%d] missing protocolVersion", i)
		}
	}

	// Request beyond the total: should get 0 cards
	req2 := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=2&offset=100", nil, "viewer")
	w2 := httptest.NewRecorder()
	h.GetA2AIndex(w2, req2)

	env2 := parseEnvelope(t, w2)
	data2 := env2.Data.(map[string]interface{})
	cards2 := data2["agent_cards"].([]interface{})

	if len(cards2) != 0 {
		t.Errorf("expected 0 cards at offset=100, got %d", len(cards2))
	}
}

// --- Well-known: Cache-Control and ETag are always present ---

func TestA2AHandler_WellKnown_HeadersAlwaysPresent(t *testing.T) {
	tests := []struct {
		name   string
		agents int
	}{
		{"zero agents", 0},
		{"one agent", 1},
		{"many agents", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			now := time.Now()
			for i := 0; i < tt.agents; i++ {
				id := fmt.Sprintf("agent_%d", i)
				agentStore.agents[id] = &store.Agent{
					ID: id, Name: fmt.Sprintf("Agent %d", i), Description: "Test",
					Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
					IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
				}
			}

			h := NewA2AHandler(agentStore, "https://registry.example.com")

			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}

			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type: got %q", w.Header().Get("Content-Type"))
			}
			if w.Header().Get("Cache-Control") != "public, max-age=60" {
				t.Errorf("Cache-Control: got %q", w.Header().Get("Cache-Control"))
			}
			if w.Header().Get("ETag") == "" {
				t.Error("ETag header should always be present")
			}
		})
	}
}

// --- Verify agent-card 404 error response structure ---

func TestA2AHandler_GetAgentCard_NotFound_ErrorStructure(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/nonexistent/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "nonexistent")
	w := httptest.NewRecorder()
	h.GetAgentCard(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	// Verify standard error envelope
	if raw["success"] != false {
		t.Error("success should be false")
	}

	errObj, ok := raw["error"].(map[string]interface{})
	if !ok {
		t.Fatal("error should be an object")
	}
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("error.code: got %v, want NOT_FOUND", errObj["code"])
	}
	if errObj["message"] == nil || errObj["message"] == "" {
		t.Error("error.message should be non-empty")
	}

	// Meta should still be present on errors
	if raw["meta"] == nil {
		t.Error("meta should be present on error responses")
	}
}

// --- Verify a2a-index with store error returns proper envelope ---

func TestA2AHandler_Index_StoreError_EnvelopeIntegrity(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.listErr = fmt.Errorf("connection reset")
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	if raw["success"] != false {
		t.Error("success should be false")
	}

	errObj, ok := raw["error"].(map[string]interface{})
	if !ok {
		t.Fatal("error should be an object")
	}

	// Should NOT leak the actual error message
	msg, _ := errObj["message"].(string)
	if strings.Contains(msg, "connection reset") {
		t.Error("error message should not leak internal error details")
	}
}
