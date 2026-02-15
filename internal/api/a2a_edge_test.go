package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// Task #6: Edge Case and Error Handling Tests
// =============================================================================

// --- agentToA2ACard edge cases ---

func TestAgentToA2ACard_EmptyName(t *testing.T) {
	agent := &store.Agent{
		ID:             "empty_name",
		Name:           "",
		Description:    "Agent with empty name",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if card.Name != "" {
		t.Errorf("expected empty name, got %q", card.Name)
	}
	// Synthetic skill should still use the agent's fields
	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	if card.Skills[0].Name != "" {
		t.Errorf("expected empty skill name, got %q", card.Skills[0].Name)
	}
}

func TestAgentToA2ACard_EmptyDescription(t *testing.T) {
	agent := &store.Agent{
		ID:             "empty_desc",
		Name:           "Agent",
		Description:    "",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if card.Description != "" {
		t.Errorf("expected empty description, got %q", card.Description)
	}
	if card.Skills[0].Description != "" {
		t.Errorf("expected empty skill description, got %q", card.Skills[0].Description)
	}
}

func TestAgentToA2ACard_VeryLongStrings(t *testing.T) {
	longName := strings.Repeat("A", 10000)
	longDesc := strings.Repeat("B", 50000)

	agent := &store.Agent{
		ID:             "long_strings",
		Name:           longName,
		Description:    longDesc,
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if card.Name != longName {
		t.Errorf("expected long name to be preserved, got len=%d", len(card.Name))
	}
	if card.Description != longDesc {
		t.Errorf("expected long description to be preserved, got len=%d", len(card.Description))
	}

	// Card should still serialize to valid JSON
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal card with long strings: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestAgentToA2ACard_MalformedToolsJSON(t *testing.T) {
	tests := []struct {
		name  string
		tools json.RawMessage
	}{
		{"invalid JSON", json.RawMessage(`{not valid}`)},
		{"string instead of array", json.RawMessage(`"just a string"`)},
		{"number instead of array", json.RawMessage(`42`)},
		{"null JSON", json.RawMessage(`null`)},
		{"object instead of array", json.RawMessage(`{"name": "test"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID:             "malformed_tools",
				Name:           "Malformed Tools Agent",
				Description:    "Agent with malformed tools",
				Tools:          tt.tools,
				ExamplePrompts: json.RawMessage(`[]`),
				Version:        1,
			}

			// Should not panic
			card := agentToA2ACard(agent, "https://example.com")

			// When tools unmarshal fails, should get synthetic skill
			if len(card.Skills) != 1 {
				t.Fatalf("expected 1 synthetic skill for malformed tools, got %d", len(card.Skills))
			}
			if card.Skills[0].ID != "malformed_tools" {
				t.Errorf("synthetic skill ID: got %q, want %q", card.Skills[0].ID, "malformed_tools")
			}
		})
	}
}

func TestAgentToA2ACard_MalformedExamplePrompts(t *testing.T) {
	tests := []struct {
		name     string
		examples json.RawMessage
	}{
		{"invalid JSON", json.RawMessage(`{not valid}`)},
		{"number instead of array", json.RawMessage(`42`)},
		{"string instead of array", json.RawMessage(`"just a string"`)},
		{"null JSON", json.RawMessage(`null`)},
		{"object instead of array", json.RawMessage(`{"prompt": "test"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID:             "malformed_examples",
				Name:           "Malformed Examples Agent",
				Description:    "Agent with malformed examples",
				Tools:          json.RawMessage(`[]`),
				ExamplePrompts: tt.examples,
				Version:        1,
			}

			// Should not panic
			card := agentToA2ACard(agent, "https://example.com")

			if len(card.Skills) != 1 {
				t.Fatalf("expected 1 skill, got %d", len(card.Skills))
			}
			// Examples should be empty or nil when unmarshal fails
		})
	}
}

func TestAgentToA2ACard_SpecialCharactersInFields(t *testing.T) {
	agent := &store.Agent{
		ID:             "special-chars_123",
		Name:           `Agent "with" <special> & 'chars'`,
		Description:    "Description with \ttabs, \nnewlines, and unicode: \u00e9\u00e8\u00ea",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`["Prompt with \"quotes\""]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	// Should produce valid JSON without corruption
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal card: %v", err)
	}

	// Verify it round-trips cleanly
	var roundTrip A2AAgentCard
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("failed to unmarshal card: %v", err)
	}
	if roundTrip.Name != agent.Name {
		t.Errorf("name not preserved after round-trip: got %q, want %q", roundTrip.Name, agent.Name)
	}
}

func TestAgentToA2ACard_ZeroVersion(t *testing.T) {
	agent := &store.Agent{
		ID:             "zero_version",
		Name:           "Zero Version Agent",
		Description:    "Agent with version 0",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        0,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if card.Version != "0" {
		t.Errorf("version: got %q, want %q", card.Version, "0")
	}
}

func TestAgentToA2ACard_LargeVersion(t *testing.T) {
	agent := &store.Agent{
		ID:             "large_version",
		Name:           "Large Version Agent",
		Description:    "Agent with very large version",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        999999999,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if card.Version != "999999999" {
		t.Errorf("version: got %q, want %q", card.Version, "999999999")
	}
}

func TestAgentToA2ACard_EmptyExternalURL(t *testing.T) {
	agent := &store.Agent{
		ID:             "test",
		Name:           "Test",
		Description:    "Test agent",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "")

	if card.URL != "/api/v1/agents/test" {
		t.Errorf("url: got %q, want %q", card.URL, "/api/v1/agents/test")
	}
	if card.Provider.URL != "" {
		t.Errorf("provider url: got %q, want %q", card.Provider.URL, "")
	}
}

func TestAgentToA2ACard_ToolsWithEmptyFields(t *testing.T) {
	agent := &store.Agent{
		ID:          "empty_tool_fields",
		Name:        "Empty Tool Fields Agent",
		Description: "Agent with tools that have empty fields",
		Tools: json.RawMessage(`[
			{"name": "", "source": "", "server_label": "", "description": ""}
		]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	skill := card.Skills[0]
	if skill.ID != "" {
		t.Errorf("skill id: got %q, want %q", skill.ID, "")
	}
	if skill.Name != "" {
		t.Errorf("skill name: got %q, want %q", skill.Name, "")
	}
}

func TestAgentToA2ACard_ManyTools(t *testing.T) {
	// Generate 100 tools
	tools := make([]map[string]string, 100)
	for i := 0; i < 100; i++ {
		tools[i] = map[string]string{
			"name":         fmt.Sprintf("tool_%d", i),
			"source":       "internal",
			"server_label": "",
			"description":  fmt.Sprintf("Tool number %d", i),
		}
	}
	toolsJSON, _ := json.Marshal(tools)

	agent := &store.Agent{
		ID:             "many_tools",
		Name:           "Many Tools Agent",
		Description:    "Agent with 100 tools",
		Tools:          json.RawMessage(toolsJSON),
		ExamplePrompts: json.RawMessage(`["Example"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if len(card.Skills) != 100 {
		t.Fatalf("expected 100 skills, got %d", len(card.Skills))
	}
	// Only first skill should have examples
	if len(card.Skills[0].Examples) != 1 {
		t.Errorf("first skill examples: got %d, want 1", len(card.Skills[0].Examples))
	}
	for i := 1; i < 100; i++ {
		if len(card.Skills[i].Examples) != 0 {
			t.Errorf("skill[%d] should have 0 examples, got %d", i, len(card.Skills[i].Examples))
			break
		}
	}
}

// --- GetAgentCard handler edge cases ---

func TestA2AHandler_GetAgentCard_EmptyAgentID(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents//agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "")
	w := httptest.NewRecorder()

	h.GetAgentCard(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty agentId, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestA2AHandler_GetAgentCard_SpecialCharsInAgentID(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"url encoded", "%2F%2E%2E"},
		{"html injection", "<script>alert(1)</script>"},
		{"sql injection", "'; DROP TABLE agents; --"},
		{"null bytes", "agent\x00id"},
		{"unicode", "\u202e\u0041\u0042"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			h := NewA2AHandler(agentStore, "https://example.com")

			req := agentRequest(http.MethodGet, "/api/v1/agents/test/agent-card", nil, "viewer")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.GetAgentCard(w, req)

			// Should get 404, not a panic or 500
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for %q, got %d", tt.agentID, w.Code)
			}

			// Verify response is valid JSON
			var raw json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Errorf("response is not valid JSON: %v", err)
			}
		})
	}
}

func TestA2AHandler_GetAgentCard_StoreError(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.getByIDErr = fmt.Errorf("connection reset by peer")
	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/test/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "test")
	w := httptest.NewRecorder()

	h.GetAgentCard(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for store error, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	if env.Success {
		t.Error("expected success=false for store error")
	}
}

// --- GetWellKnownAgentCard edge cases ---

func TestA2AHandler_WellKnown_StoreError(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.listErr = fmt.Errorf("database connection lost")
	h := NewA2AHandler(agentStore, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()

	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for store error, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestA2AHandler_WellKnown_ETagMismatch(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("If-None-Match", `"totally-wrong-etag"`)
	w := httptest.NewRecorder()

	h.GetWellKnownAgentCard(w, req)

	// Mismatched ETag should return 200 with full body, not 304
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for mismatched ETag, got %d", w.Code)
	}
}

func TestA2AHandler_WellKnown_EmptyIfNoneMatch(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("If-None-Match", "")
	w := httptest.NewRecorder()

	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty If-None-Match, got %d", w.Code)
	}
}

func TestA2AHandler_WellKnown_ManyAgents(t *testing.T) {
	agentStore := newMockAgentStore()
	now := time.Now()

	// Seed 200 agents
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("agent_%03d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: fmt.Sprintf("Desc %d", i),
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://example.com")

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

	if len(card.Skills) != 200 {
		t.Errorf("expected 200 skills, got %d", len(card.Skills))
	}
}

func TestA2AHandler_WellKnown_ETagChangesOnUpdate(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	// First request
	req1 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w1 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w1, req1)
	etag1 := w1.Header().Get("ETag")

	// Update agent timestamp
	later := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	agentStore.agents["test"].UpdatedAt = later

	// Second request
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w2 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w2, req2)
	etag2 := w2.Header().Get("ETag")

	if etag1 == etag2 {
		t.Errorf("ETag should change after update: both %q", etag1)
	}

	// Using old ETag should get full response, not 304
	req3 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req3.Header.Set("If-None-Match", etag1)
	w3 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 with stale ETag, got %d", w3.Code)
	}
}

// --- GetA2AIndex edge cases ---

func TestA2AHandler_Index_PaginationEdgeCases(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("agent_%d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: fmt.Sprintf("Desc %d", i),
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	tests := []struct {
		name      string
		query     string
		wantCards int
	}{
		{"offset beyond total", "?offset=100", 0},
		{"limit=0 defaults to 20", "?limit=0", 5},
		{"negative limit defaults to 20", "?limit=-5", 5},
		{"negative offset defaults to 0", "?offset=-10", 5},
		{"limit exceeds max capped at 200", "?limit=500", 5},
		{"non-numeric offset", "?offset=abc", 5},
		{"non-numeric limit", "?limit=xyz", 5},
		{"limit=1 offset=4 (last item)", "?limit=1&offset=4", 1},
		{"limit=1 offset=5 (one past end)", "?limit=1&offset=5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.GetA2AIndex(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			data := env.Data.(map[string]interface{})
			cards := data["agent_cards"].([]interface{})

			if len(cards) != tt.wantCards {
				t.Errorf("expected %d cards, got %d", tt.wantCards, len(cards))
			}
		})
	}
}

func TestA2AHandler_Index_FilterSpecialCharacters(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test Agent", Description: "A test agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"percent encoding", "?q=%25", http.StatusOK},
		{"angle brackets", "?q=%3Cscript%3E", http.StatusOK},
		{"double quotes", "?q=%22test%22", http.StatusOK},
		{"single quotes", "?q=%27test%27", http.StatusOK},
		{"ampersand", "?q=test%26other", http.StatusOK},
		{"empty query", "?q=", http.StatusOK},
		{"spaces", "?q=test+agent", http.StatusOK},
		{"unicode", "?q=%C3%A9", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.GetA2AIndex(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			// Response should always be valid JSON
			var raw json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Errorf("response is not valid JSON: %v", err)
			}
		})
	}
}

func TestA2AHandler_Index_StoreError(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.listErr = fmt.Errorf("database unavailable")
	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()

	h.GetA2AIndex(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for store error, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if env.Success {
		t.Error("expected success=false for store error")
	}
}

func TestA2AHandler_Index_FilterAndPaginationCombined(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()

	// Seed agents with varied names
	for _, name := range []string{"PMO Agent", "Development Agent", "QA Agent", "PMO Assistant", "Design Agent"} {
		id := strings.ReplaceAll(strings.ToLower(name), " ", "_")
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: name, Description: "Desc",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	// Filter by "pmo" — should match "PMO Agent" and "PMO Assistant"
	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=pmo", nil, "viewer")
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
		t.Errorf("expected 2 PMO cards, got %d", len(cards))
	}
	// When filtering, total reflects filtered count
	if total != float64(len(cards)) {
		t.Errorf("total should equal filtered count: got %v, want %d", total, len(cards))
	}
}

func TestA2AHandler_Index_EmptyRegistryReturnsEmptyArray(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()

	h.GetA2AIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})

	// Should be an empty array, not null
	if cards == nil {
		t.Error("expected empty array, got nil")
	}
	if len(cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(cards))
	}
}

// =============================================================================
// Task #7: Protocol Compliance Tests — A2A v0.3.0
// =============================================================================

// --- Required field validation ---

func TestA2AProtocol_AgentCard_RequiredFields(t *testing.T) {
	agent := &store.Agent{
		ID:          "protocol_test",
		Name:        "Protocol Test Agent",
		Description: "Agent for protocol compliance testing",
		Tools: json.RawMessage(`[
			{"name": "tool1", "source": "internal", "server_label": "", "description": "A tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Do something"]`),
		IsActive:       true,
		Version:        5,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal card: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// A2A v0.3.0 required fields
	requiredFields := []string{
		"name",
		"description",
		"url",
		"version",
		"protocolVersion",
		"provider",
		"capabilities",
		"defaultInputModes",
		"defaultOutputModes",
		"skills",
		"securitySchemes",
		"security",
	}

	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("A2A required field %q is missing from Agent Card", field)
		}
	}
}

func TestA2AProtocol_ProtocolVersionIs030(t *testing.T) {
	agent := &store.Agent{
		ID:             "version_check",
		Name:           "Version Check",
		Description:    "Check protocol version",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if card.ProtocolVersion != "0.3.0" {
		t.Errorf("protocolVersion: got %q, want %q", card.ProtocolVersion, "0.3.0")
	}
}

func TestA2AProtocol_ConstantVersion(t *testing.T) {
	if a2aProtocolVersion != "0.3.0" {
		t.Errorf("a2aProtocolVersion constant: got %q, want %q", a2aProtocolVersion, "0.3.0")
	}
}

func TestA2AProtocol_ProviderStructure(t *testing.T) {
	agent := &store.Agent{
		ID:             "provider_check",
		Name:           "Provider Check",
		Description:    "Check provider structure",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	data, _ := json.Marshal(card)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	provider, ok := raw["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("provider should be an object")
	}

	// A2A spec: provider must have organization and url
	if _, ok := provider["organization"]; !ok {
		t.Error("provider.organization is missing")
	}
	if _, ok := provider["url"]; !ok {
		t.Error("provider.url is missing")
	}

	if provider["organization"] != "Agentic Registry" {
		t.Errorf("provider.organization: got %v, want %q", provider["organization"], "Agentic Registry")
	}
	if provider["url"] != "https://registry.example.com" {
		t.Errorf("provider.url: got %v, want %q", provider["url"], "https://registry.example.com")
	}
}

func TestA2AProtocol_CapabilitiesStructure(t *testing.T) {
	agent := &store.Agent{
		ID:             "caps_check",
		Name:           "Capabilities Check",
		Description:    "Check capabilities",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	caps, ok := raw["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("capabilities should be an object")
	}

	// A2A spec: capabilities should have streaming and pushNotifications
	streaming, ok := caps["streaming"]
	if !ok {
		t.Error("capabilities.streaming is missing")
	}
	if streaming != false {
		t.Errorf("capabilities.streaming: got %v, want false", streaming)
	}

	pushNotifications, ok := caps["pushNotifications"]
	if !ok {
		t.Error("capabilities.pushNotifications is missing")
	}
	if pushNotifications != false {
		t.Errorf("capabilities.pushNotifications: got %v, want false", pushNotifications)
	}
}

func TestA2AProtocol_SecuritySchemesStructure(t *testing.T) {
	agent := &store.Agent{
		ID:             "sec_check",
		Name:           "Security Check",
		Description:    "Check security schemes",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// securitySchemes should be a map
	schemes, ok := raw["securitySchemes"].(map[string]interface{})
	if !ok {
		t.Fatal("securitySchemes should be an object")
	}

	// Should contain bearerAuth
	bearerAuth, ok := schemes["bearerAuth"].(map[string]interface{})
	if !ok {
		t.Fatal("securitySchemes.bearerAuth should be an object")
	}

	if bearerAuth["type"] != "http" {
		t.Errorf("bearerAuth.type: got %v, want %q", bearerAuth["type"], "http")
	}
	if bearerAuth["scheme"] != "bearer" {
		t.Errorf("bearerAuth.scheme: got %v, want %q", bearerAuth["scheme"], "bearer")
	}

	// security should reference bearerAuth
	security, ok := raw["security"].([]interface{})
	if !ok {
		t.Fatal("security should be an array")
	}
	if len(security) != 1 {
		t.Fatalf("security should have 1 entry, got %d", len(security))
	}

	secEntry, ok := security[0].(map[string]interface{})
	if !ok {
		t.Fatal("security[0] should be an object")
	}
	if _, ok := secEntry["bearerAuth"]; !ok {
		t.Error("security[0] should reference bearerAuth")
	}
}

func TestA2AProtocol_SkillStructure(t *testing.T) {
	agent := &store.Agent{
		ID:          "skill_check",
		Name:        "Skill Check Agent",
		Description: "Check A2A skill structure",
		Tools: json.RawMessage(`[
			{"name": "code_review", "source": "mcp", "server_label": "github-mcp", "description": "Review code changes"}
		]`),
		ExamplePrompts: json.RawMessage(`["Review PR #123"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	skills, ok := raw["skills"].([]interface{})
	if !ok || len(skills) == 0 {
		t.Fatal("skills should be a non-empty array")
	}

	skill, ok := skills[0].(map[string]interface{})
	if !ok {
		t.Fatal("skills[0] should be an object")
	}

	// A2A spec: each skill must have id, name, description
	requiredSkillFields := []string{"id", "name", "description", "tags", "examples"}
	for _, field := range requiredSkillFields {
		if _, ok := skill[field]; !ok {
			t.Errorf("skill field %q is missing", field)
		}
	}

	if skill["id"] != "code_review" {
		t.Errorf("skill.id: got %v, want %q", skill["id"], "code_review")
	}
	if skill["name"] != "code_review" {
		t.Errorf("skill.name: got %v, want %q", skill["name"], "code_review")
	}
	if skill["description"] != "Review code changes" {
		t.Errorf("skill.description: got %v, want %q", skill["description"], "Review code changes")
	}

	// Tags should include source and server_label
	tags, ok := skill["tags"].([]interface{})
	if !ok {
		t.Fatal("skill.tags should be an array")
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags (source + server_label), got %d", len(tags))
	}
	if tags[0] != "mcp" {
		t.Errorf("tag[0]: got %v, want %q", tags[0], "mcp")
	}
	if tags[1] != "github-mcp" {
		t.Errorf("tag[1]: got %v, want %q", tags[1], "github-mcp")
	}

	// Examples should be present
	examples, ok := skill["examples"].([]interface{})
	if !ok {
		t.Fatal("skill.examples should be an array")
	}
	if len(examples) != 1 || examples[0] != "Review PR #123" {
		t.Errorf("skill.examples: got %v, want [Review PR #123]", examples)
	}
}

func TestA2AProtocol_DefaultModes(t *testing.T) {
	agent := &store.Agent{
		ID:             "modes_check",
		Name:           "Modes Check",
		Description:    "Check default modes",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// defaultInputModes and defaultOutputModes should be arrays of strings
	inputModes, ok := raw["defaultInputModes"].([]interface{})
	if !ok {
		t.Fatal("defaultInputModes should be an array")
	}
	if len(inputModes) != 1 || inputModes[0] != "text" {
		t.Errorf("defaultInputModes: got %v, want [text]", inputModes)
	}

	outputModes, ok := raw["defaultOutputModes"].([]interface{})
	if !ok {
		t.Fatal("defaultOutputModes should be an array")
	}
	if len(outputModes) != 1 || outputModes[0] != "text" {
		t.Errorf("defaultOutputModes: got %v, want [text]", outputModes)
	}
}

func TestA2AProtocol_FieldTypes(t *testing.T) {
	agent := &store.Agent{
		ID:          "types_check",
		Name:        "Types Check Agent",
		Description: "Check all field types match spec",
		Tools: json.RawMessage(`[
			{"name": "test_tool", "source": "internal", "server_label": "", "description": "A test tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Test example"]`),
		Version:        3,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// name: string
	if _, ok := raw["name"].(string); !ok {
		t.Error("name should be a string")
	}
	// description: string
	if _, ok := raw["description"].(string); !ok {
		t.Error("description should be a string")
	}
	// url: string
	if _, ok := raw["url"].(string); !ok {
		t.Error("url should be a string")
	}
	// version: string (not number)
	if _, ok := raw["version"].(string); !ok {
		t.Error("version should be a string")
	}
	// protocolVersion: string
	if _, ok := raw["protocolVersion"].(string); !ok {
		t.Error("protocolVersion should be a string")
	}
	// provider: object
	if _, ok := raw["provider"].(map[string]interface{}); !ok {
		t.Error("provider should be an object")
	}
	// capabilities: object
	if _, ok := raw["capabilities"].(map[string]interface{}); !ok {
		t.Error("capabilities should be an object")
	}
	// defaultInputModes: array
	if _, ok := raw["defaultInputModes"].([]interface{}); !ok {
		t.Error("defaultInputModes should be an array")
	}
	// defaultOutputModes: array
	if _, ok := raw["defaultOutputModes"].([]interface{}); !ok {
		t.Error("defaultOutputModes should be an array")
	}
	// skills: array
	if _, ok := raw["skills"].([]interface{}); !ok {
		t.Error("skills should be an array")
	}
	// securitySchemes: object
	if _, ok := raw["securitySchemes"].(map[string]interface{}); !ok {
		t.Error("securitySchemes should be an object")
	}
	// security: array
	if _, ok := raw["security"].([]interface{}); !ok {
		t.Error("security should be an array")
	}
}

// --- Well-known endpoint protocol compliance ---

func TestA2AProtocol_WellKnown_RawJSON_NoEnvelope(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// Well-known MUST NOT be wrapped in an envelope
	if _, ok := raw["success"]; ok {
		t.Error("well-known should NOT contain 'success' envelope field")
	}
	if _, ok := raw["data"]; ok {
		t.Error("well-known should NOT contain 'data' envelope field")
	}
	if _, ok := raw["meta"]; ok {
		t.Error("well-known should NOT contain 'meta' envelope field")
	}

	// MUST contain A2A card fields directly
	if _, ok := raw["name"]; !ok {
		t.Error("well-known should contain 'name' directly")
	}
	if _, ok := raw["protocolVersion"]; !ok {
		t.Error("well-known should contain 'protocolVersion' directly")
	}
	if _, ok := raw["skills"]; !ok {
		t.Error("well-known should contain 'skills' directly")
	}
}

func TestA2AProtocol_WellKnown_ContentType(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

func TestA2AProtocol_WellKnown_CacheHeaders(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=60" {
		t.Errorf("Cache-Control: got %q, want %q", cc, "public, max-age=60")
	}

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Error("ETag header should be set")
	}
	// ETag should be quoted
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag should be quoted: got %q", etag)
	}
}

func TestA2AProtocol_WellKnown_CompositeCard(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["agent1"] = &store.Agent{
		ID: "agent1", Name: "Agent One", Description: "First agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	agentStore.agents["agent2"] = &store.Agent{
		ID: "agent2", Name: "Agent Two", Description: "Second agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 2, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	var card A2AAgentCard
	json.NewDecoder(w.Body).Decode(&card)

	// Well-known card represents the registry itself, not individual agents
	if card.Name != "Agentic Registry" {
		t.Errorf("name: got %q, want %q", card.Name, "Agentic Registry")
	}
	if card.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", card.Version, "1.0.0")
	}
	if card.URL != "https://registry.example.com" {
		t.Errorf("url: got %q, want %q", card.URL, "https://registry.example.com")
	}
	// Each registered agent becomes a skill
	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills from 2 agents, got %d", len(card.Skills))
	}

	// Skills should have "agent" tag
	for _, skill := range card.Skills {
		if len(skill.Tags) != 1 || skill.Tags[0] != "agent" {
			t.Errorf("skill %q tags: got %v, want [agent]", skill.ID, skill.Tags)
		}
	}
}

// --- API endpoint (agent-card) protocol compliance ---

func TestA2AProtocol_AgentCard_EnvelopeWrapping(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test Agent", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/test/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "test")
	w := httptest.NewRecorder()

	h.GetAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	// API endpoint MUST be wrapped in standard envelope
	if _, ok := raw["success"]; !ok {
		t.Error("API agent-card should be wrapped in envelope with 'success'")
	}
	if _, ok := raw["data"]; !ok {
		t.Error("API agent-card should be wrapped in envelope with 'data'")
	}
	if _, ok := raw["meta"]; !ok {
		t.Error("API agent-card should be wrapped in envelope with 'meta'")
	}

	// success should be true
	if raw["success"] != true {
		t.Errorf("success: got %v, want true", raw["success"])
	}

	// data should contain the A2A card
	data, ok := raw["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data should be an object")
	}
	if _, ok := data["protocolVersion"]; !ok {
		t.Error("data should contain A2A card with protocolVersion")
	}
}

func TestA2AProtocol_AgentCard_ContentType(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/test/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "test")
	w := httptest.NewRecorder()

	h.GetAgentCard(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// --- A2A Index protocol compliance ---

func TestA2AProtocol_Index_EnvelopeWrapping(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()

	h.GetA2AIndex(w, req)

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	// Index MUST be wrapped in envelope
	if _, ok := raw["success"]; !ok {
		t.Error("a2a-index should have 'success' in envelope")
	}
	if _, ok := raw["data"]; !ok {
		t.Error("a2a-index should have 'data' in envelope")
	}

	data := raw["data"].(map[string]interface{})
	if _, ok := data["agent_cards"]; !ok {
		t.Error("data should contain 'agent_cards'")
	}
	if _, ok := data["total"]; !ok {
		t.Error("data should contain 'total'")
	}
}

func TestA2AProtocol_Index_CardsAreValidA2A(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test Agent", Description: "Test",
		Tools: json.RawMessage(`[
			{"name": "test_tool", "source": "internal", "server_label": "", "description": "A tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Example"]`),
		IsActive:       true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()

	h.GetA2AIndex(w, req)

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})

	if len(cards) == 0 {
		t.Fatal("expected at least one card")
	}

	card := cards[0].(map[string]interface{})

	// Each card in the index should be a valid A2A card
	requiredFields := []string{
		"name", "description", "url", "version", "protocolVersion",
		"provider", "capabilities", "defaultInputModes", "defaultOutputModes",
		"skills", "securitySchemes", "security",
	}
	for _, field := range requiredFields {
		if _, ok := card[field]; !ok {
			t.Errorf("card in index is missing A2A required field %q", field)
		}
	}

	if card["protocolVersion"] != "0.3.0" {
		t.Errorf("card protocolVersion: got %v, want %q", card["protocolVersion"], "0.3.0")
	}
}

// --- HTTP method compliance ---

func TestA2AProtocol_WellKnown_OnlyGETAllowed(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    h,
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				t.Errorf("well-known should not accept %s (got 200)", method)
			}
		})
	}
}

// --- No internal data leakage ---

func TestA2AProtocol_NoInternalDataLeakage(t *testing.T) {
	now := time.Now()
	agent := &store.Agent{
		ID:             "leak_test",
		Name:           "Leak Test Agent",
		Description:    "Testing for data leaks",
		SystemPrompt:   "SECRET SYSTEM PROMPT DO NOT EXPOSE",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{"sensitive": true}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "admin_user_uuid",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	cardJSON := string(data)

	// These internal fields should NEVER appear in the A2A card
	sensitiveFields := []string{
		"system_prompt",
		"systemPrompt",
		"SECRET SYSTEM PROMPT",
		"trust_overrides",
		"trustOverrides",
		"created_by",
		"createdBy",
		"admin_user_uuid",
		"created_at",
		"createdAt",
		"updated_at",
		"updatedAt",
		"is_active",
		"isActive",
		"password",
	}

	for _, field := range sensitiveFields {
		if strings.Contains(cardJSON, field) {
			t.Errorf("A2A card should NOT contain internal field/value %q", field)
		}
	}
}

func TestA2AProtocol_WellKnown_NoSensitiveDataInSkills(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID:             "test",
		Name:           "Test Agent",
		Description:    "Test agent",
		SystemPrompt:   "You are a secret agent with credentials: password123",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{"bypass_all": true}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "secret-admin-id",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	body := w.Body.String()

	sensitiveValues := []string{
		"password123",
		"secret-admin-id",
		"bypass_all",
		"system_prompt",
		"trust_overrides",
	}

	for _, val := range sensitiveValues {
		if strings.Contains(body, val) {
			t.Errorf("well-known response should NOT contain sensitive value %q", val)
		}
	}
}

// --- URL construction ---

func TestA2AProtocol_AgentCard_URLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		externalURL string
		agentID     string
		wantURL     string
	}{
		{
			name:        "standard URL",
			externalURL: "https://registry.example.com",
			agentID:     "pmo",
			wantURL:     "https://registry.example.com/api/v1/agents/pmo",
		},
		{
			name:        "URL with trailing slash",
			externalURL: "https://registry.example.com/",
			agentID:     "pmo",
			wantURL:     "https://registry.example.com//api/v1/agents/pmo",
		},
		{
			name:        "URL with port",
			externalURL: "https://registry.example.com:8443",
			agentID:     "pmo",
			wantURL:     "https://registry.example.com:8443/api/v1/agents/pmo",
		},
		{
			name:        "URL with path prefix",
			externalURL: "https://example.com/registry",
			agentID:     "pmo",
			wantURL:     "https://example.com/registry/api/v1/agents/pmo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID:             tt.agentID,
				Name:           "Test",
				Description:    "Test",
				Tools:          json.RawMessage(`[]`),
				ExamplePrompts: json.RawMessage(`[]`),
				Version:        1,
			}

			card := agentToA2ACard(agent, tt.externalURL)

			if card.URL != tt.wantURL {
				t.Errorf("url: got %q, want %q", card.URL, tt.wantURL)
			}
		})
	}
}

// --- Router wiring compliance ---

func TestA2AProtocol_WellKnownRoute_Public(t *testing.T) {
	agentStore := newMockAgentStore()
	now := time.Now()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")
	health := &HealthHandler{}

	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    h,
	})

	// Well-known MUST be accessible without any authentication
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	// No auth headers, no cookies
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("well-known should be public: got %d, want %d; body: %s",
			w.Code, http.StatusOK, w.Body.String())
	}
}

func TestA2AProtocol_NilA2AHandler_NoRoutes(t *testing.T) {
	health := &HealthHandler{}

	// Router with nil A2A handler should not panic
	router := NewRouter(RouterConfig{
		Health: health,
		A2A:    nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get 404 (or SPA fallback), not a panic
	if w.Code == http.StatusOK {
		// Only OK if there's an SPA serving index.html
		ct := w.Header().Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			t.Error("should not get JSON response when A2A handler is nil")
		}
	}
}

// --- JSON serialization compliance ---

func TestA2AProtocol_JSONFieldNaming(t *testing.T) {
	agent := &store.Agent{
		ID:          "json_check",
		Name:        "JSON Check",
		Description: "Check JSON field naming",
		Tools: json.RawMessage(`[
			{"name": "test_tool", "source": "internal", "server_label": "", "description": "Tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Example"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")
	data, _ := json.Marshal(card)

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// A2A uses camelCase naming
	expectedFields := map[string]bool{
		"name":               true,
		"description":        true,
		"url":                true,
		"version":            true,
		"protocolVersion":    true,
		"provider":           true,
		"capabilities":       true,
		"defaultInputModes":  true,
		"defaultOutputModes": true,
		"skills":             true,
		"securitySchemes":    true,
		"security":           true,
	}

	for key := range raw {
		if !expectedFields[key] {
			t.Errorf("unexpected JSON field %q in A2A card", key)
		}
	}

	for field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("expected JSON field %q missing from A2A card", field)
		}
	}

	// Check no snake_case fields leaked
	snakeCaseFields := []string{
		"protocol_version", "default_input_modes", "default_output_modes",
		"security_schemes", "push_notifications", "server_label",
	}
	jsonStr := string(data)
	for _, field := range snakeCaseFields {
		if strings.Contains(jsonStr, `"`+field+`"`) {
			t.Errorf("A2A card should use camelCase, found snake_case field %q", field)
		}
	}
}

func TestA2AProtocol_SkillsNeverNull(t *testing.T) {
	// Even with no tools, skills should be an array, not null
	tests := []struct {
		name  string
		tools json.RawMessage
	}{
		{"empty array tools", json.RawMessage(`[]`)},
		{"nil tools", nil},
		{"null JSON tools", json.RawMessage(`null`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &store.Agent{
				ID:             "null_check",
				Name:           "Null Check",
				Description:    "Check skills never null",
				Tools:          tt.tools,
				ExamplePrompts: json.RawMessage(`[]`),
				Version:        1,
			}

			card := agentToA2ACard(agent, "https://example.com")

			if card.Skills == nil {
				t.Error("skills should never be nil")
			}

			data, _ := json.Marshal(card)
			if strings.Contains(string(data), `"skills":null`) {
				t.Error("skills should not serialize as null")
			}
		})
	}
}

func TestA2AProtocol_TagsAndExamplesNeverNull(t *testing.T) {
	agent := &store.Agent{
		ID:             "arrays_check",
		Name:           "Arrays Check",
		Description:    "Verify arrays are never null",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	data, _ := json.Marshal(card)
	cardJSON := string(data)

	// None of the array fields should be null
	if strings.Contains(cardJSON, `"tags":null`) {
		t.Error("tags should not be null")
	}
	if strings.Contains(cardJSON, `"examples":null`) {
		t.Error("examples should not be null")
	}
	if strings.Contains(cardJSON, `"defaultInputModes":null`) {
		t.Error("defaultInputModes should not be null")
	}
	if strings.Contains(cardJSON, `"defaultOutputModes":null`) {
		t.Error("defaultOutputModes should not be null")
	}
	if strings.Contains(cardJSON, `"security":null`) {
		t.Error("security should not be null")
	}
}
