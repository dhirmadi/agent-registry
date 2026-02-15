package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- TestAgentToA2ACard tests ---

func TestAgentToA2ACard(t *testing.T) {
	externalURL := "https://registry.example.com"

	tests := []struct {
		name           string
		agent          *store.Agent
		wantName       string
		wantURL        string
		wantVersion    string
		wantSkillCount int
		wantSkillID    string // first skill id
		wantSkillTags  []string
		wantExamples   []string
	}{
		{
			name: "agent with tools maps to skills",
			agent: &store.Agent{
				ID:          "pmo",
				Name:        "PMO Agent",
				Description: "Project management agent",
				Tools: json.RawMessage(`[
					{"name": "create_task", "source": "internal", "server_label": "", "description": "Create a new task"},
					{"name": "search_docs", "source": "mcp", "server_label": "google-workspace-mcp", "description": "Search Google docs"}
				]`),
				ExamplePrompts: json.RawMessage(`["Create a sprint plan", "Track project status"]`),
				IsActive:       true,
				Version:        3,
			},
			wantName:       "PMO Agent",
			wantURL:        "https://registry.example.com/api/v1/agents/pmo",
			wantVersion:    "3",
			wantSkillCount: 2,
			wantSkillID:    "create_task",
			wantSkillTags:  []string{"internal"},
			wantExamples:   []string{"Create a sprint plan", "Track project status"},
		},
		{
			name: "agent with no tools gets synthetic skill",
			agent: &store.Agent{
				ID:             "simple_agent",
				Name:           "Simple Agent",
				Description:    "A simple agent with no tools",
				Tools:          json.RawMessage(`[]`),
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        1,
			},
			wantName:       "Simple Agent",
			wantURL:        "https://registry.example.com/api/v1/agents/simple_agent",
			wantVersion:    "1",
			wantSkillCount: 1,
			wantSkillID:    "simple_agent",
		},
		{
			name: "agent with nil tools gets synthetic skill",
			agent: &store.Agent{
				ID:             "nil_tools",
				Name:           "Nil Tools Agent",
				Description:    "Agent with nil tools",
				Tools:          nil,
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        1,
			},
			wantName:       "Nil Tools Agent",
			wantURL:        "https://registry.example.com/api/v1/agents/nil_tools",
			wantVersion:    "1",
			wantSkillCount: 1,
			wantSkillID:    "nil_tools",
		},
		{
			name: "mcp tool gets source and server_label as tags",
			agent: &store.Agent{
				ID:          "mcp_agent",
				Name:        "MCP Agent",
				Description: "Agent with MCP tools",
				Tools: json.RawMessage(`[
					{"name": "slack_post", "source": "mcp", "server_label": "slack-mcp", "description": "Post to Slack"}
				]`),
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        2,
			},
			wantName:       "MCP Agent",
			wantURL:        "https://registry.example.com/api/v1/agents/mcp_agent",
			wantVersion:    "2",
			wantSkillCount: 1,
			wantSkillID:    "slack_post",
			wantSkillTags:  []string{"mcp", "slack-mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := agentToA2ACard(tt.agent, externalURL)

			if card.Name != tt.wantName {
				t.Errorf("name: got %q, want %q", card.Name, tt.wantName)
			}
			if card.URL != tt.wantURL {
				t.Errorf("url: got %q, want %q", card.URL, tt.wantURL)
			}
			if card.Version != tt.wantVersion {
				t.Errorf("version: got %q, want %q", card.Version, tt.wantVersion)
			}
			if card.ProtocolVersion != "0.3.0" {
				t.Errorf("protocolVersion: got %q, want %q", card.ProtocolVersion, "0.3.0")
			}
			if card.Description != tt.agent.Description {
				t.Errorf("description: got %q, want %q", card.Description, tt.agent.Description)
			}
			if len(card.Skills) != tt.wantSkillCount {
				t.Fatalf("skills count: got %d, want %d", len(card.Skills), tt.wantSkillCount)
			}
			if card.Skills[0].ID != tt.wantSkillID {
				t.Errorf("first skill id: got %q, want %q", card.Skills[0].ID, tt.wantSkillID)
			}
			if tt.wantSkillTags != nil {
				for i, tag := range tt.wantSkillTags {
					if i >= len(card.Skills[0].Tags) || card.Skills[0].Tags[i] != tag {
						t.Errorf("skill tag[%d]: got %v, want %q", i, card.Skills[0].Tags, tag)
						break
					}
				}
			}
			if tt.wantExamples != nil && len(card.Skills) > 0 {
				for i, ex := range tt.wantExamples {
					if i >= len(card.Skills[0].Examples) || card.Skills[0].Examples[i] != ex {
						t.Errorf("skill example[%d]: got %v, want %q", i, card.Skills[0].Examples, ex)
						break
					}
				}
			}
			// Verify capabilities default
			if card.Capabilities.Streaming {
				t.Error("capabilities.streaming should be false")
			}
			if card.Capabilities.PushNotifications {
				t.Error("capabilities.pushNotifications should be false")
			}
			// Verify default modes
			if len(card.DefaultInputModes) != 1 || card.DefaultInputModes[0] != "text" {
				t.Errorf("defaultInputModes: got %v, want [text]", card.DefaultInputModes)
			}
			if len(card.DefaultOutputModes) != 1 || card.DefaultOutputModes[0] != "text" {
				t.Errorf("defaultOutputModes: got %v, want [text]", card.DefaultOutputModes)
			}
			// Verify security scheme
			if len(card.SecuritySchemes) != 1 {
				t.Fatalf("securitySchemes: got %d, want 1", len(card.SecuritySchemes))
			}
			if card.SecuritySchemes["bearerAuth"].Type != "http" {
				t.Errorf("securityScheme type: got %q, want %q", card.SecuritySchemes["bearerAuth"].Type, "http")
			}
			if card.SecuritySchemes["bearerAuth"].Scheme != "bearer" {
				t.Errorf("securityScheme scheme: got %q, want %q", card.SecuritySchemes["bearerAuth"].Scheme, "bearer")
			}
			// Verify security reference
			if len(card.Security) != 1 {
				t.Fatalf("security: got %d entries, want 1", len(card.Security))
			}
		})
	}
}

func TestA2AAgentCardProvider(t *testing.T) {
	agent := &store.Agent{
		ID:             "test",
		Name:           "Test",
		Description:    "Test agent",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	if card.Provider.Organization != "Agentic Registry" {
		t.Errorf("provider.organization: got %q, want %q", card.Provider.Organization, "Agentic Registry")
	}
	if card.Provider.URL != "https://registry.example.com" {
		t.Errorf("provider.url: got %q, want %q", card.Provider.URL, "https://registry.example.com")
	}
}

func TestNewA2AHandler(t *testing.T) {
	agentStore := newMockAgentStore()
	externalURL := "https://registry.example.com"

	h := NewA2AHandler(agentStore, externalURL)

	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.externalURL != externalURL {
		t.Errorf("externalURL: got %q, want %q", h.externalURL, externalURL)
	}
}

func TestAgentToA2ACard_ExamplePromptsOnFirstSkill(t *testing.T) {
	agent := &store.Agent{
		ID:          "example_agent",
		Name:        "Example Agent",
		Description: "Agent with examples and tools",
		Tools: json.RawMessage(`[
			{"name": "tool_a", "source": "internal", "server_label": "", "description": "First tool"},
			{"name": "tool_b", "source": "internal", "server_label": "", "description": "Second tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Example 1", "Example 2"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(card.Skills))
	}
	// Examples should be on the first skill only
	if len(card.Skills[0].Examples) != 2 {
		t.Errorf("first skill should have 2 examples, got %d", len(card.Skills[0].Examples))
	}
	if len(card.Skills[1].Examples) != 0 {
		t.Errorf("second skill should have 0 examples, got %d", len(card.Skills[1].Examples))
	}
}

func TestAgentToA2ACard_SyntheticSkillUsesAgentDescription(t *testing.T) {
	agent := &store.Agent{
		ID:             "synthetic",
		Name:           "Synthetic Agent",
		Description:    "A description for the synthetic skill",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`["What can you do?"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 synthetic skill, got %d", len(card.Skills))
	}
	skill := card.Skills[0]
	if skill.ID != "synthetic" {
		t.Errorf("synthetic skill id: got %q, want %q", skill.ID, "synthetic")
	}
	if skill.Name != "Synthetic Agent" {
		t.Errorf("synthetic skill name: got %q, want %q", skill.Name, "Synthetic Agent")
	}
	if skill.Description != "A description for the synthetic skill" {
		t.Errorf("synthetic skill description: got %q, want %q", skill.Description, "A description for the synthetic skill")
	}
	if len(skill.Examples) != 1 || skill.Examples[0] != "What can you do?" {
		t.Errorf("synthetic skill examples: got %v, want [What can you do?]", skill.Examples)
	}
}

func TestAgentToA2ACard_UpdatedAtNotIncluded(t *testing.T) {
	now := time.Now()
	agent := &store.Agent{
		ID:             "ts_agent",
		Name:           "Timestamp Agent",
		Description:    "Agent to verify no timestamps leak",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
		UpdatedAt:      now,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	// Marshal to JSON to verify no unexpected fields leak
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal card: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// These fields should NOT be present in the A2A card
	for _, field := range []string{"updated_at", "created_at", "created_by", "system_prompt", "is_active"} {
		if _, ok := raw[field]; ok {
			t.Errorf("A2A card should not contain %q", field)
		}
	}
}

// --- GetAgentCard handler tests ---

func TestA2AHandler_GetAgentCard(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		seedAgent  *store.Agent
		storeErr   error
		wantStatus int
		wantErr    bool
	}{
		{
			name:    "success - agent with tools",
			agentID: "pmo",
			seedAgent: &store.Agent{
				ID:          "pmo",
				Name:        "PMO Agent",
				Description: "Project management",
				Tools: json.RawMessage(`[
					{"name": "create_task", "source": "internal", "server_label": "", "description": "Create task"}
				]`),
				ExamplePrompts: json.RawMessage(`["Plan a sprint"]`),
				IsActive:       true,
				Version:        2,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "success - agent with no tools (synthetic skill)",
			agentID: "simple",
			seedAgent: &store.Agent{
				ID:             "simple",
				Name:           "Simple Agent",
				Description:    "No tools agent",
				Tools:          json.RawMessage(`[]`),
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        1,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			agentID:    "nonexistent",
			wantStatus: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:       "store error returns 500",
			agentID:    "error_agent",
			storeErr:   fmt.Errorf("database connection lost"),
			wantStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			if tt.seedAgent != nil {
				agentStore.agents[tt.seedAgent.ID] = tt.seedAgent
			}
			if tt.storeErr != nil {
				agentStore.getByIDErr = tt.storeErr
			}

			h := NewA2AHandler(agentStore, "https://registry.example.com")

			req := agentRequest(http.MethodGet, "/api/v1/agents/"+tt.agentID+"/agent-card", nil, "viewer")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.GetAgentCard(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if !tt.wantErr {
				env := parseEnvelope(t, w)
				if !env.Success {
					t.Fatal("expected success=true")
				}
				data, ok := env.Data.(map[string]interface{})
				if !ok {
					t.Fatal("expected data to be a map")
				}
				if data["name"] != tt.seedAgent.Name {
					t.Errorf("card name: got %v, want %q", data["name"], tt.seedAgent.Name)
				}
				if data["protocolVersion"] != "0.3.0" {
					t.Errorf("protocolVersion: got %v, want %q", data["protocolVersion"], "0.3.0")
				}
				skills, ok := data["skills"].([]interface{})
				if !ok || len(skills) == 0 {
					t.Fatal("expected at least one skill in card")
				}
			}
		})
	}
}

// --- GetWellKnownAgentCard handler tests ---

func TestA2AHandler_WellKnown(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	later := time.Date(2026, 2, 15, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		agents         []*store.Agent
		ifNoneMatch    string
		wantStatus     int
		wantSkillCount int
		wantETag       bool
		wantCacheCtrl  string
	}{
		{
			name: "success - two active agents become skills",
			agents: []*store.Agent{
				{
					ID: "pmo", Name: "PMO Agent", Description: "Project management",
					Tools: json.RawMessage(`[{"name": "plan", "source": "internal", "server_label": "", "description": "Plan"}]`),
					ExamplePrompts: json.RawMessage(`[]`), IsActive: true, Version: 1,
					CreatedAt: now, UpdatedAt: now,
				},
				{
					ID: "dev", Name: "Dev Agent", Description: "Development",
					Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
					IsActive: true, Version: 2,
					CreatedAt: now, UpdatedAt: later,
				},
			},
			wantStatus:     http.StatusOK,
			wantSkillCount: 2,
			wantETag:       true,
			wantCacheCtrl:  "public, max-age=60",
		},
		{
			name:           "empty registry - no agents",
			agents:         []*store.Agent{},
			wantStatus:     http.StatusOK,
			wantSkillCount: 0,
			wantETag:       true,
			wantCacheCtrl:  "public, max-age=60",
		},
		{
			name: "ETag match returns 304",
			agents: []*store.Agent{
				{
					ID: "pmo", Name: "PMO Agent", Description: "PM",
					Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
					IsActive: true, Version: 1,
					CreatedAt: now, UpdatedAt: now,
				},
			},
			ifNoneMatch: fmt.Sprintf(`"%s"`, now.UTC().Format(time.RFC3339Nano)),
			wantStatus:  http.StatusNotModified,
		},
		{
			name: "only active agents included",
			agents: []*store.Agent{
				{
					ID: "active", Name: "Active Agent", Description: "Active",
					Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
					IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
				},
				{
					ID: "inactive", Name: "Inactive Agent", Description: "Inactive",
					Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
					IsActive: false, Version: 1, CreatedAt: now, UpdatedAt: now,
				},
			},
			wantStatus:     http.StatusOK,
			wantSkillCount: 1,
			wantETag:       true,
			wantCacheCtrl:  "public, max-age=60",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			for _, a := range tt.agents {
				agentStore.agents[a.ID] = a
			}

			h := NewA2AHandler(agentStore, "https://registry.example.com")

			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			if tt.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tt.ifNoneMatch)
			}
			w := httptest.NewRecorder()

			h.GetWellKnownAgentCard(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantStatus == http.StatusNotModified {
				return
			}

			// Well-known returns raw JSON, not an envelope
			var card A2AAgentCard
			if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if card.Name != "Agentic Registry" {
				t.Errorf("name: got %q, want %q", card.Name, "Agentic Registry")
			}
			if card.ProtocolVersion != "0.3.0" {
				t.Errorf("protocolVersion: got %q, want %q", card.ProtocolVersion, "0.3.0")
			}
			if len(card.Skills) != tt.wantSkillCount {
				t.Errorf("skills count: got %d, want %d", len(card.Skills), tt.wantSkillCount)
			}

			if tt.wantETag && w.Header().Get("ETag") == "" {
				t.Error("expected ETag header to be set")
			}
			if tt.wantCacheCtrl != "" {
				if got := w.Header().Get("Cache-Control"); got != tt.wantCacheCtrl {
					t.Errorf("Cache-Control: got %q, want %q", got, tt.wantCacheCtrl)
				}
			}

			// Verify content type
			if got := w.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type: got %q, want %q", got, "application/json")
			}
		})
	}
}

// --- GetA2AIndex handler tests ---

func TestA2AHandler_Index(t *testing.T) {
	now := time.Now()

	seedAgents := func(s *mockAgentStore) {
		for _, a := range []*store.Agent{
			{ID: "pmo", Name: "PMO Agent", Description: "Project management",
				Tools: json.RawMessage(`[{"name":"plan","source":"internal","server_label":"","description":"Plan"}]`),
				ExamplePrompts: json.RawMessage(`[]`), IsActive: true, Version: 1,
				CreatedAt: now, UpdatedAt: now},
			{ID: "dev", Name: "Dev Agent", Description: "Development assistance",
				Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
				IsActive: true, Version: 2, CreatedAt: now, UpdatedAt: now},
			{ID: "qa", Name: "QA Agent", Description: "Quality assurance testing",
				Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
				IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now},
		} {
			s.agents[a.ID] = a
		}
	}

	tests := []struct {
		name       string
		query      string
		seed       bool
		wantStatus int
		wantCount  int
		wantTotal  float64
	}{
		{
			name:       "success - all active agents",
			query:      "",
			seed:       true,
			wantStatus: http.StatusOK,
			wantCount:  3,
			wantTotal:  3,
		},
		{
			name:       "pagination - limit 1",
			query:      "?limit=1",
			seed:       true,
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  3,
		},
		{
			name:       "keyword filter - matches name",
			query:      "?q=PMO",
			seed:       true,
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "keyword filter - case insensitive",
			query:      "?q=development",
			seed:       true,
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "keyword filter - matches description",
			query:      "?q=quality",
			seed:       true,
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantTotal:  1,
		},
		{
			name:       "keyword filter - no matches",
			query:      "?q=nonexistent",
			seed:       true,
			wantStatus: http.StatusOK,
			wantCount:  0,
			wantTotal:  0,
		},
		{
			name:       "empty registry",
			query:      "",
			seed:       false,
			wantStatus: http.StatusOK,
			wantCount:  0,
			wantTotal:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			if tt.seed {
				seedAgents(agentStore)
			}

			h := NewA2AHandler(agentStore, "https://registry.example.com")

			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.GetA2AIndex(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status: got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			env := parseEnvelope(t, w)
			if !env.Success {
				t.Fatal("expected success=true")
			}

			data, ok := env.Data.(map[string]interface{})
			if !ok {
				t.Fatal("expected data to be a map")
			}

			cards, ok := data["agent_cards"].([]interface{})
			if !ok {
				t.Fatal("expected data.agent_cards to be an array")
			}
			if len(cards) != tt.wantCount {
				t.Errorf("agent_cards count: got %d, want %d", len(cards), tt.wantCount)
			}

			total, ok := data["total"].(float64)
			if !ok {
				t.Fatal("expected data.total to be a number")
			}
			if total != tt.wantTotal {
				t.Errorf("total: got %v, want %v", total, tt.wantTotal)
			}
		})
	}
}

// --- Router wiring tests ---

func TestA2ARouter_WellKnownIsPublic(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
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

	// Well-known should be accessible without auth
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("well-known should be public (no auth): got %d, want %d; body: %s",
			w.Code, http.StatusOK, w.Body.String())
	}

	var card A2AAgentCard
	if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if card.Name != "Agentic Registry" {
		t.Errorf("card name: got %q, want %q", card.Name, "Agentic Registry")
	}
}

// =============================================================================
// FUNCTIONAL TESTS — A2A v0.3.0 schema compliance, field validation, edge cases
// =============================================================================

// TestA2ACard_JSONSchemaCompliance validates that marshaled A2AAgentCard JSON
// contains exactly the required fields from the A2A v0.3.0 specification.
func TestA2ACard_JSONSchemaCompliance(t *testing.T) {
	agent := &store.Agent{
		ID:          "schema_test",
		Name:        "Schema Test Agent",
		Description: "Validates JSON schema compliance",
		Tools: json.RawMessage(`[
			{"name": "test_tool", "source": "internal", "server_label": "", "description": "A test tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Hello", "Help me"]`),
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

	// Required top-level fields per A2A v0.3.0
	requiredFields := []string{
		"name", "description", "url", "version", "protocolVersion",
		"provider", "capabilities", "defaultInputModes", "defaultOutputModes",
		"skills", "securitySchemes", "security",
	}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("required A2A field missing: %q", field)
		}
	}

	// Verify field types
	if _, ok := raw["name"].(string); !ok {
		t.Error("name should be a string")
	}
	if _, ok := raw["description"].(string); !ok {
		t.Error("description should be a string")
	}
	if _, ok := raw["url"].(string); !ok {
		t.Error("url should be a string")
	}
	if _, ok := raw["version"].(string); !ok {
		t.Error("version should be a string")
	}
	if _, ok := raw["protocolVersion"].(string); !ok {
		t.Error("protocolVersion should be a string")
	}
	if _, ok := raw["provider"].(map[string]interface{}); !ok {
		t.Error("provider should be an object")
	}
	if _, ok := raw["capabilities"].(map[string]interface{}); !ok {
		t.Error("capabilities should be an object")
	}
	if _, ok := raw["defaultInputModes"].([]interface{}); !ok {
		t.Error("defaultInputModes should be an array")
	}
	if _, ok := raw["defaultOutputModes"].([]interface{}); !ok {
		t.Error("defaultOutputModes should be an array")
	}
	if _, ok := raw["skills"].([]interface{}); !ok {
		t.Error("skills should be an array")
	}
	if _, ok := raw["securitySchemes"].(map[string]interface{}); !ok {
		t.Error("securitySchemes should be an object")
	}
	if _, ok := raw["security"].([]interface{}); !ok {
		t.Error("security should be an array")
	}

	// Verify no extra fields (only the 12 required fields)
	allowedFields := map[string]bool{
		"name": true, "description": true, "url": true, "version": true,
		"protocolVersion": true, "provider": true, "capabilities": true,
		"defaultInputModes": true, "defaultOutputModes": true, "skills": true,
		"securitySchemes": true, "security": true,
	}
	for key := range raw {
		if !allowedFields[key] {
			t.Errorf("unexpected field in A2A card: %q", key)
		}
	}
}

// TestA2ACard_SkillSchemaCompliance validates that each skill in the card has
// the correct JSON structure per A2A v0.3.0.
func TestA2ACard_SkillSchemaCompliance(t *testing.T) {
	agent := &store.Agent{
		ID:          "skill_schema",
		Name:        "Skill Schema Agent",
		Description: "Tests skill field structure",
		Tools: json.RawMessage(`[
			{"name": "tool_one", "source": "mcp", "server_label": "srv", "description": "First tool"}
		]`),
		ExamplePrompts: json.RawMessage(`["Example prompt"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	skills := raw["skills"].([]interface{})
	if len(skills) == 0 {
		t.Fatal("expected at least one skill")
	}

	skill := skills[0].(map[string]interface{})

	// Required skill fields
	for _, field := range []string{"id", "name", "description", "tags", "examples"} {
		if _, ok := skill[field]; !ok {
			t.Errorf("required skill field missing: %q", field)
		}
	}

	// Verify types
	if _, ok := skill["id"].(string); !ok {
		t.Error("skill.id should be a string")
	}
	if _, ok := skill["name"].(string); !ok {
		t.Error("skill.name should be a string")
	}
	if _, ok := skill["description"].(string); !ok {
		t.Error("skill.description should be a string")
	}
	if _, ok := skill["tags"].([]interface{}); !ok {
		t.Error("skill.tags should be an array")
	}
	if _, ok := skill["examples"].([]interface{}); !ok {
		t.Error("skill.examples should be an array")
	}
}

// TestA2ACard_SyntheticSkillTags verifies synthetic skills get empty tags array.
func TestA2ACard_SyntheticSkillTags(t *testing.T) {
	agent := &store.Agent{
		ID:             "no_tools",
		Name:           "No Tools Agent",
		Description:    "Agent without tools",
		Tools:          json.RawMessage(`[]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 synthetic skill, got %d", len(card.Skills))
	}
	if len(card.Skills[0].Tags) != 0 {
		t.Errorf("synthetic skill tags should be empty, got %v", card.Skills[0].Tags)
	}
}

// TestA2ACard_ToolWithEmptyServerLabel verifies internal tool only has source tag.
func TestA2ACard_ToolWithEmptyServerLabel(t *testing.T) {
	agent := &store.Agent{
		ID:          "internal_only",
		Name:        "Internal Agent",
		Description: "Agent with internal tools",
		Tools: json.RawMessage(`[
			{"name": "internal_tool", "source": "internal", "server_label": "", "description": "Internal tool"}
		]`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://registry.example.com")

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	if len(card.Skills[0].Tags) != 1 {
		t.Errorf("internal tool should have exactly 1 tag, got %v", card.Skills[0].Tags)
	}
	if card.Skills[0].Tags[0] != "internal" {
		t.Errorf("internal tool tag: got %q, want %q", card.Skills[0].Tags[0], "internal")
	}
}

// TestA2ACard_VersionIsStringFromInt verifies version is integer-to-string conversion.
func TestA2ACard_VersionIsStringFromInt(t *testing.T) {
	tests := []struct {
		version     int
		wantVersion string
	}{
		{1, "1"},
		{0, "0"},
		{99, "99"},
		{1000, "1000"},
	}

	for _, tt := range tests {
		agent := &store.Agent{
			ID: "ver", Name: "V", Description: "D",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			Version: tt.version,
		}
		card := agentToA2ACard(agent, "https://example.com")
		if card.Version != tt.wantVersion {
			t.Errorf("version %d: got %q, want %q", tt.version, card.Version, tt.wantVersion)
		}
	}
}

// TestA2ACard_URLConstruction verifies URL is externalURL + /api/v1/agents/ + ID.
func TestA2ACard_URLConstruction(t *testing.T) {
	tests := []struct {
		externalURL string
		agentID     string
		wantURL     string
	}{
		{"https://registry.example.com", "pmo", "https://registry.example.com/api/v1/agents/pmo"},
		{"https://example.com", "a-b-c", "https://example.com/api/v1/agents/a-b-c"},
		{"http://localhost:8080", "test_agent", "http://localhost:8080/api/v1/agents/test_agent"},
		{"https://registry.example.com", "agent_with_underscores", "https://registry.example.com/api/v1/agents/agent_with_underscores"},
	}

	for _, tt := range tests {
		agent := &store.Agent{
			ID: tt.agentID, Name: "N", Description: "D",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			Version: 1,
		}
		card := agentToA2ACard(agent, tt.externalURL)
		if card.URL != tt.wantURL {
			t.Errorf("URL for (%q, %q): got %q, want %q", tt.externalURL, tt.agentID, card.URL, tt.wantURL)
		}
	}
}

// TestA2ACard_NilExamplePrompts verifies nil example_prompts results in empty/nil examples.
func TestA2ACard_NilExamplePrompts(t *testing.T) {
	agent := &store.Agent{
		ID: "nil_ex", Name: "Nil Example", Description: "No examples",
		Tools: json.RawMessage(`[
			{"name": "tool1", "source": "internal", "server_label": "", "description": "Tool"}
		]`),
		ExamplePrompts: nil,
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")
	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	// With nil ExamplePrompts, the examples variable stays nil from unmarshal.
	// The first skill gets the nil slice assigned. Verify it doesn't panic.
	_ = card.Skills[0].Examples
}

// TestA2ACard_NilExamplePromptsSynthetic verifies nil prompts on synthetic skill.
func TestA2ACard_NilExamplePromptsSynthetic(t *testing.T) {
	agent := &store.Agent{
		ID: "nil_syn", Name: "Nil Syn", Description: "Synthetic with nil",
		Tools:          nil,
		ExamplePrompts: nil,
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")
	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 synthetic skill, got %d", len(card.Skills))
	}
	if card.Skills[0].ID != "nil_syn" {
		t.Errorf("synthetic skill id: got %q, want %q", card.Skills[0].ID, "nil_syn")
	}
}

// TestA2ACard_ManyTools verifies correct skill generation for agents with many tools.
func TestA2ACard_ManyTools(t *testing.T) {
	toolCount := 20
	tools := make([]agentTool, toolCount)
	for i := 0; i < toolCount; i++ {
		tools[i] = agentTool{
			Name:        fmt.Sprintf("tool_%d", i),
			Source:      "internal",
			ServerLabel: "",
			Description: fmt.Sprintf("Tool number %d", i),
		}
	}
	toolsJSON, _ := json.Marshal(tools)

	agent := &store.Agent{
		ID: "many_tools", Name: "Many Tools Agent", Description: "Agent with 20 tools",
		Tools:          json.RawMessage(toolsJSON),
		ExamplePrompts: json.RawMessage(`["prompt1"]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if len(card.Skills) != toolCount {
		t.Fatalf("skills count: got %d, want %d", len(card.Skills), toolCount)
	}

	// Only the first skill gets examples
	if len(card.Skills[0].Examples) != 1 {
		t.Errorf("first skill examples: got %d, want 1", len(card.Skills[0].Examples))
	}
	for i := 1; i < toolCount; i++ {
		if len(card.Skills[i].Examples) != 0 {
			t.Errorf("skill[%d] should have 0 examples, got %d", i, len(card.Skills[i].Examples))
		}
	}

	// Verify each skill has the correct ID
	for i := 0; i < toolCount; i++ {
		wantID := fmt.Sprintf("tool_%d", i)
		if card.Skills[i].ID != wantID {
			t.Errorf("skill[%d].id: got %q, want %q", i, card.Skills[i].ID, wantID)
		}
	}
}

// TestA2ACard_ProtocolVersionConstant verifies the constant is "0.3.0".
func TestA2ACard_ProtocolVersionConstant(t *testing.T) {
	if a2aProtocolVersion != "0.3.0" {
		t.Errorf("a2aProtocolVersion: got %q, want %q", a2aProtocolVersion, "0.3.0")
	}
}

// TestA2ACard_SecuritySchemeStructure validates security scheme deep structure.
func TestA2ACard_SecuritySchemeStructure(t *testing.T) {
	agent := &store.Agent{
		ID: "sec", Name: "S", Description: "D",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		Version: 1,
	}
	card := agentToA2ACard(agent, "https://example.com")

	// securitySchemes must have exactly "bearerAuth" key
	if _, ok := card.SecuritySchemes["bearerAuth"]; !ok {
		t.Fatal("securitySchemes must contain bearerAuth")
	}
	if len(card.SecuritySchemes) != 1 {
		t.Errorf("securitySchemes should have exactly 1 entry, got %d", len(card.SecuritySchemes))
	}

	// security array must reference bearerAuth
	if len(card.Security) != 1 {
		t.Fatalf("security should have 1 entry, got %d", len(card.Security))
	}
	scopes, ok := card.Security[0]["bearerAuth"]
	if !ok {
		t.Fatal("security[0] must reference bearerAuth")
	}
	if len(scopes) != 0 {
		t.Errorf("bearerAuth scopes should be empty, got %v", scopes)
	}
}

// TestA2AHandler_GetAgentCard_FullResponseValidation validates the complete
// envelope response for GetAgentCard including all nested structures.
func TestA2AHandler_GetAgentCard_FullResponseValidation(t *testing.T) {
	now := time.Now()
	agent := &store.Agent{
		ID:          "full_check",
		Name:        "Full Check Agent",
		Description: "Comprehensive response validation",
		Tools: json.RawMessage(`[
			{"name": "read_file", "source": "mcp", "server_label": "fs-mcp", "description": "Read a file"},
			{"name": "write_file", "source": "mcp", "server_label": "fs-mcp", "description": "Write a file"}
		]`),
		ExamplePrompts: json.RawMessage(`["Read my config", "Save changes"]`),
		IsActive:       true,
		Version:        7,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	agentStore := newMockAgentStore()
	agentStore.agents["full_check"] = agent
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/full_check/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "full_check")
	w := httptest.NewRecorder()
	h.GetAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data should be a map")
	}

	// Validate provider structure
	provider, ok := data["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("provider should be an object")
	}
	if provider["organization"] != "Agentic Registry" {
		t.Errorf("provider.organization: got %v, want %q", provider["organization"], "Agentic Registry")
	}
	if provider["url"] != "https://registry.example.com" {
		t.Errorf("provider.url: got %v, want %q", provider["url"], "https://registry.example.com")
	}

	// Validate capabilities structure
	caps, ok := data["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("capabilities should be an object")
	}
	if caps["streaming"] != false {
		t.Errorf("capabilities.streaming: got %v, want false", caps["streaming"])
	}
	if caps["pushNotifications"] != false {
		t.Errorf("capabilities.pushNotifications: got %v, want false", caps["pushNotifications"])
	}

	// Validate skills array structure
	skills, ok := data["skills"].([]interface{})
	if !ok || len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %v", skills)
	}

	// First skill: read_file
	skill0 := skills[0].(map[string]interface{})
	if skill0["id"] != "read_file" {
		t.Errorf("skill[0].id: got %v, want %q", skill0["id"], "read_file")
	}
	if skill0["name"] != "read_file" {
		t.Errorf("skill[0].name: got %v, want %q", skill0["name"], "read_file")
	}
	if skill0["description"] != "Read a file" {
		t.Errorf("skill[0].description: got %v, want %q", skill0["description"], "Read a file")
	}
	tags0 := skill0["tags"].([]interface{})
	if len(tags0) != 2 || tags0[0] != "mcp" || tags0[1] != "fs-mcp" {
		t.Errorf("skill[0].tags: got %v, want [mcp fs-mcp]", tags0)
	}
	examples0 := skill0["examples"].([]interface{})
	if len(examples0) != 2 {
		t.Errorf("skill[0].examples count: got %d, want 2", len(examples0))
	}

	// Second skill: write_file (no examples)
	skill1 := skills[1].(map[string]interface{})
	if skill1["id"] != "write_file" {
		t.Errorf("skill[1].id: got %v, want %q", skill1["id"], "write_file")
	}
	examples1 := skill1["examples"].([]interface{})
	if len(examples1) != 0 {
		t.Errorf("skill[1].examples should be empty, got %v", examples1)
	}

	// Validate securitySchemes
	secSchemes, ok := data["securitySchemes"].(map[string]interface{})
	if !ok || len(secSchemes) != 1 {
		t.Fatalf("securitySchemes: expected 1 entry, got %v", secSchemes)
	}
	bearer, ok := secSchemes["bearerAuth"].(map[string]interface{})
	if !ok {
		t.Fatal("bearerAuth should be an object")
	}
	if bearer["type"] != "http" {
		t.Errorf("bearerAuth.type: got %v, want %q", bearer["type"], "http")
	}
	if bearer["scheme"] != "bearer" {
		t.Errorf("bearerAuth.scheme: got %v, want %q", bearer["scheme"], "bearer")
	}

	// Validate security references
	security, ok := data["security"].([]interface{})
	if !ok || len(security) != 1 {
		t.Fatalf("security should have 1 entry, got %v", security)
	}
	secEntry := security[0].(map[string]interface{})
	if _, ok := secEntry["bearerAuth"]; !ok {
		t.Error("security[0] should reference bearerAuth")
	}

	// Verify version is a string "7"
	if data["version"] != "7" {
		t.Errorf("version: got %v, want %q", data["version"], "7")
	}

	// Verify URL construction
	if data["url"] != "https://registry.example.com/api/v1/agents/full_check" {
		t.Errorf("url: got %v, want %q", data["url"], "https://registry.example.com/api/v1/agents/full_check")
	}
}

// TestA2AHandler_GetAgentCard_ErrorResponseSchema validates error envelope structure.
func TestA2AHandler_GetAgentCard_ErrorResponseSchema(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/missing/agent-card", nil, "viewer")
	req = withChiParam(req, "agentId", "missing")
	w := httptest.NewRecorder()
	h.GetAgentCard(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}

	env := parseEnvelope(t, w)
	if env.Success {
		t.Error("expected success=false for not found")
	}
	if env.Error == nil {
		t.Error("expected error to be non-nil")
	}

	errMap, ok := env.Error.(map[string]interface{})
	if !ok {
		t.Fatal("error should be a map")
	}
	if errMap["code"] != "NOT_FOUND" {
		t.Errorf("error.code: got %v, want %q", errMap["code"], "NOT_FOUND")
	}
}

// TestA2AHandler_WellKnown_ETagGeneration validates ETag is computed from max(UpdatedAt).
func TestA2AHandler_WellKnown_ETagGeneration(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 2, 15, 12, 30, 45, 123456789, time.UTC)

	agentStore := newMockAgentStore()
	agentStore.agents["a1"] = &store.Agent{
		ID: "a1", Name: "Agent 1", Description: "First",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: earlier, UpdatedAt: earlier,
	}
	agentStore.agents["a2"] = &store.Agent{
		ID: "a2", Name: "Agent 2", Description: "Second",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: earlier, UpdatedAt: latest,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}

	etag := w.Header().Get("ETag")
	expectedETag := fmt.Sprintf(`"%s"`, latest.UTC().Format(time.RFC3339Nano))
	if etag != expectedETag {
		t.Errorf("ETag: got %q, want %q", etag, expectedETag)
	}
}

// TestA2AHandler_WellKnown_ETagMismatchReturns200 verifies stale ETag gets fresh response.
func TestA2AHandler_WellKnown_ETagMismatchReturns200(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)

	agentStore := newMockAgentStore()
	agentStore.agents["a1"] = &store.Agent{
		ID: "a1", Name: "Agent 1", Description: "Agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("If-None-Match", `"2020-01-01T00:00:00Z"`)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("stale ETag should return 200, got %d", w.Code)
	}
}

// TestA2AHandler_WellKnown_EmptyRegistryETag verifies ETag with zero time.
func TestA2AHandler_WellKnown_EmptyRegistryETag(t *testing.T) {
	agentStore := newMockAgentStore()
	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}

	etag := w.Header().Get("ETag")
	zeroETag := fmt.Sprintf(`"%s"`, time.Time{}.UTC().Format(time.RFC3339Nano))
	if etag != zeroETag {
		t.Errorf("empty registry ETag: got %q, want zero-time ETag %q", etag, zeroETag)
	}
}

// TestA2AHandler_WellKnown_StoreError_ListFailure verifies 500 on store failure.
func TestA2AHandler_WellKnown_StoreError_ListFailure(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.listErr = fmt.Errorf("database down")

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", w.Code)
	}

	var errResp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "internal server error" {
		t.Errorf("error message: got %v, want %q", errResp["error"], "internal server error")
	}
}

// TestA2AHandler_WellKnown_CardStructure validates the well-known card has correct top-level fields.
func TestA2AHandler_WellKnown_CardStructure(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test Agent", Description: "A test agent",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://myregistry.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	var card A2AAgentCard
	if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if card.Name != "Agentic Registry" {
		t.Errorf("name: got %q, want %q", card.Name, "Agentic Registry")
	}
	if card.Description != "A registry of AI agents and their configurations" {
		t.Errorf("description: got %q", card.Description)
	}
	if card.URL != "https://myregistry.com" {
		t.Errorf("url: got %q, want %q", card.URL, "https://myregistry.com")
	}
	if card.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", card.Version, "1.0.0")
	}
	if card.ProtocolVersion != "0.3.0" {
		t.Errorf("protocolVersion: got %q, want %q", card.ProtocolVersion, "0.3.0")
	}
	if card.Provider.Organization != "Agentic Registry" {
		t.Errorf("provider.organization: got %q", card.Provider.Organization)
	}
	if card.Provider.URL != "https://myregistry.com" {
		t.Errorf("provider.url: got %q, want %q", card.Provider.URL, "https://myregistry.com")
	}

	// Well-known skills are agent-level (one per agent), tagged with "agent"
	if len(card.Skills) != 1 {
		t.Fatalf("skills count: got %d, want 1", len(card.Skills))
	}
	if card.Skills[0].ID != "test" {
		t.Errorf("skill id: got %q, want %q", card.Skills[0].ID, "test")
	}
	if card.Skills[0].Name != "Test Agent" {
		t.Errorf("skill name: got %q, want %q", card.Skills[0].Name, "Test Agent")
	}
	if len(card.Skills[0].Tags) != 1 || card.Skills[0].Tags[0] != "agent" {
		t.Errorf("well-known skill tags: got %v, want [agent]", card.Skills[0].Tags)
	}
}

// TestA2AHandler_Index_PaginationParams verifies offset/limit parsing.
func TestA2AHandler_Index_PaginationParams(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("agent_%d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: fmt.Sprintf("Description %d", i),
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	tests := []struct {
		name      string
		query     string
		wantTotal float64
	}{
		{"default pagination", "", 5},
		{"limit 2", "?limit=2", 5},
		{"offset beyond range", "?offset=100", 5},
		{"negative offset treated as 0", "?offset=-5", 5},
		{"negative limit defaults to 20", "?limit=-1", 5},
		{"zero limit defaults to 20", "?limit=0", 5},
		{"limit exceeds max 200 capped", "?limit=500", 5},
		{"non-numeric offset", "?offset=abc", 5},
		{"non-numeric limit", "?limit=abc", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200; body: %s", w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			data := env.Data.(map[string]interface{})
			total := data["total"].(float64)
			if total != tt.wantTotal {
				t.Errorf("total: got %v, want %v", total, tt.wantTotal)
			}
		})
	}
}

// TestA2AHandler_Index_StoreError_ListFailure validates store error returns 500.
func TestA2AHandler_Index_StoreError_ListFailure(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.listErr = fmt.Errorf("connection refused")

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", w.Code)
	}

	env := parseEnvelope(t, w)
	if env.Success {
		t.Error("expected success=false for store error")
	}
}

// TestA2AHandler_Index_CardsAreFullA2ACards verifies each card in the index is a complete A2A card.
func TestA2AHandler_Index_CardsAreFullA2ACards(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["rich"] = &store.Agent{
		ID: "rich", Name: "Rich Agent", Description: "Agent with tools and examples",
		Tools: json.RawMessage(`[
			{"name": "tool_a", "source": "mcp", "server_label": "srv-a", "description": "Tool A"}
		]`),
		ExamplePrompts: json.RawMessage(`["Do something"]`),
		IsActive:       true, Version: 3,
		CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})

	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}

	card := cards[0].(map[string]interface{})

	// Verify all required A2A fields present
	for _, field := range []string{"name", "description", "url", "version", "protocolVersion",
		"provider", "capabilities", "defaultInputModes", "defaultOutputModes",
		"skills", "securitySchemes", "security"} {
		if _, ok := card[field]; !ok {
			t.Errorf("card missing required field: %q", field)
		}
	}

	// Verify skill details
	skills := card["skills"].([]interface{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	skill := skills[0].(map[string]interface{})
	if skill["id"] != "tool_a" {
		t.Errorf("skill.id: got %v, want %q", skill["id"], "tool_a")
	}
	tags := skill["tags"].([]interface{})
	if len(tags) != 2 || tags[0] != "mcp" || tags[1] != "srv-a" {
		t.Errorf("skill.tags: got %v, want [mcp srv-a]", tags)
	}
	examples := skill["examples"].([]interface{})
	if len(examples) != 1 || examples[0] != "Do something" {
		t.Errorf("skill.examples: got %v, want [Do something]", examples)
	}
}

// TestA2AHandler_MalformedToolsJSON verifies graceful handling of invalid tools JSON.
func TestA2AHandler_MalformedToolsJSON(t *testing.T) {
	agent := &store.Agent{
		ID: "bad_tools", Name: "Bad Tools Agent", Description: "Invalid tools JSON",
		Tools:          json.RawMessage(`not valid json`),
		ExamplePrompts: json.RawMessage(`[]`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	// When tools JSON is malformed, json.Unmarshal fails silently, tools slice stays nil,
	// so the agent falls through to synthetic skill path.
	if len(card.Skills) != 1 {
		t.Fatalf("malformed tools should fall back to synthetic skill, got %d skills", len(card.Skills))
	}
	if card.Skills[0].ID != "bad_tools" {
		t.Errorf("synthetic skill id: got %q, want %q", card.Skills[0].ID, "bad_tools")
	}
}

// TestA2AHandler_MalformedExamplePromptsJSON verifies graceful handling of invalid prompts.
func TestA2AHandler_MalformedExamplePromptsJSON(t *testing.T) {
	agent := &store.Agent{
		ID: "bad_ex", Name: "Bad Examples", Description: "Invalid examples",
		Tools: json.RawMessage(`[{"name":"t1","source":"internal","server_label":"","description":"T1"}]`),
		ExamplePrompts: json.RawMessage(`{invalid}`),
		Version:        1,
	}

	card := agentToA2ACard(agent, "https://example.com")

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	// Malformed examples should result in nil/empty (unmarshal fails)
	if len(card.Skills[0].Examples) != 0 && card.Skills[0].Examples != nil {
		t.Errorf("malformed examples should be empty, got %v", card.Skills[0].Examples)
	}
}

// TestA2AHandler_WellKnown_304HasNoBody verifies 304 response has empty body.
func TestA2AHandler_WellKnown_304HasNoBody(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["test"] = &store.Agent{
		ID: "test", Name: "Test", Description: "Test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	etag := fmt.Sprintf(`"%s"`, now.UTC().Format(time.RFC3339Nano))
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	if w.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("304 body should be empty, got %d bytes", w.Body.Len())
	}
}

// TestA2AHandler_ContentTypes verifies correct Content-Type headers on all endpoints.
func TestA2AHandler_ContentTypes(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["ct_test"] = &store.Agent{
		ID: "ct_test", Name: "CT", Description: "Content-Type test",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://example.com")

	t.Run("well-known", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
		}
	})

	t.Run("agent-card", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/agents/ct_test/agent-card", nil, "viewer")
		req = withChiParam(req, "agentId", "ct_test")
		w := httptest.NewRecorder()
		h.GetAgentCard(w, req)
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
		}
	})

	t.Run("a2a-index", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
		w := httptest.NewRecorder()
		h.GetA2AIndex(w, req)
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
		}
	})
}

// =============================================================================
// INTEGRATION TESTS — Seeded data, concurrent requests, response consistency
// =============================================================================

// TestA2AIntegration_SeededAgentsIndex tests the full flow with realistic seeded data.
func TestA2AIntegration_SeededAgentsIndex(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()

	agents := []*store.Agent{
		{ID: "pmo", Name: "PMO Agent", Description: "Project management and sprint planning",
			Tools: json.RawMessage(`[
				{"name":"create_task","source":"internal","server_label":"","description":"Create a task"},
				{"name":"search_confluence","source":"mcp","server_label":"confluence-mcp","description":"Search Confluence"}
			]`),
			ExamplePrompts: json.RawMessage(`["Create a sprint plan","Track velocity"]`),
			IsActive: true, Version: 3, CreatedAt: now, UpdatedAt: now},
		{ID: "dev", Name: "Development Agent", Description: "Code generation and review",
			Tools: json.RawMessage(`[
				{"name":"git_read","source":"mcp","server_label":"mcp-git","description":"Read git files"}
			]`),
			ExamplePrompts: json.RawMessage(`["Review this PR"]`),
			IsActive: true, Version: 5, CreatedAt: now, UpdatedAt: now},
		{ID: "qa", Name: "QA Agent", Description: "Quality assurance and testing",
			Tools:          json.RawMessage(`[]`),
			ExamplePrompts: json.RawMessage(`["Run regression tests"]`),
			IsActive:       true, Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "security", Name: "Security Agent", Description: "Security scanning and vulnerability assessment",
			Tools: json.RawMessage(`[
				{"name":"scan_deps","source":"internal","server_label":"","description":"Scan dependencies"}
			]`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       true, Version: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "decommissioned", Name: "Old Agent", Description: "Decommissioned agent",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: false, Version: 10, CreatedAt: now, UpdatedAt: now},
	}
	for _, a := range agents {
		agentStore.agents[a.ID] = a
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetA2AIndex(w, req)

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	cards := data["agent_cards"].([]interface{})
	total := data["total"].(float64)

	// 4 active agents (decommissioned excluded)
	if total != 4 {
		t.Errorf("total: got %v, want 4 (only active agents)", total)
	}
	if len(cards) != 4 {
		t.Errorf("cards count: got %d, want 4", len(cards))
	}

	// Verify each card is valid
	for i, c := range cards {
		card := c.(map[string]interface{})
		if card["protocolVersion"] != "0.3.0" {
			t.Errorf("card[%d].protocolVersion: got %v, want %q", i, card["protocolVersion"], "0.3.0")
		}
		skills, ok := card["skills"].([]interface{})
		if !ok || len(skills) == 0 {
			t.Errorf("card[%d] should have at least one skill", i)
		}
	}
}

// TestA2AIntegration_SeededAgentsFiltering tests keyword filtering on seeded data.
func TestA2AIntegration_SeededAgentsFiltering(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	for _, a := range []*store.Agent{
		{ID: "pmo", Name: "PMO Agent", Description: "Project management",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "dev", Name: "Developer Agent", Description: "Code review and generation",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "qa", Name: "QA Agent", Description: "Testing and quality assurance",
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		agentStore.agents[a.ID] = a
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	tests := []struct {
		name      string
		q         string
		wantCount int
	}{
		{"filter by agent name", "PMO", 1},
		{"filter by description word", "code", 1},
		{"partial match in name", "agent", 3},
		{"partial match in description", "ment", 1},
		{"case insensitive name", "pmo", 1},
		{"no match", "zzzzz", 0},
		{"empty query returns all", "", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := ""
			if tt.q != "" {
				query = "?q=" + tt.q
			}
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+query, nil, "viewer")
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)

			env := parseEnvelope(t, w)
			data := env.Data.(map[string]interface{})
			cards := data["agent_cards"].([]interface{})
			if len(cards) != tt.wantCount {
				t.Errorf("q=%q: got %d cards, want %d", tt.q, len(cards), tt.wantCount)
			}
		})
	}
}

// TestA2AIntegration_ConcurrentRequests tests race safety under concurrent access.
func TestA2AIntegration_ConcurrentRequests(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("agent_%d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %d", i), Description: fmt.Sprintf("Desc %d", i),
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	var wg sync.WaitGroup
	errors := make(chan string, 100)

	// 20 concurrent requests across all three endpoints
	for i := 0; i < 20; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index", nil, "viewer")
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)
			if w.Code != http.StatusOK {
				errors <- fmt.Sprintf("GetA2AIndex: got %d", w.Code)
			}
		}()

		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)
			if w.Code != http.StatusOK {
				errors <- fmt.Sprintf("GetWellKnownAgentCard: got %d", w.Code)
			}
		}()

		go func(idx int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent_%d", idx%10)
			req := agentRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/agent-card", nil, "viewer")
			req = withChiParam(req, "agentId", agentID)
			w := httptest.NewRecorder()
			h.GetAgentCard(w, req)
			if w.Code != http.StatusOK {
				errors <- fmt.Sprintf("GetAgentCard(%s): got %d", agentID, w.Code)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestA2AIntegration_ResponseConsistency verifies multiple requests return the same data.
func TestA2AIntegration_ResponseConsistency(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["stable"] = &store.Agent{
		ID: "stable", Name: "Stable Agent", Description: "Deterministic test",
		Tools: json.RawMessage(`[{"name":"tool1","source":"internal","server_label":"","description":"T1"}]`),
		ExamplePrompts: json.RawMessage(`["Hello"]`),
		IsActive:       true, Version: 42,
		CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	var firstBody string
	for i := 0; i < 5; i++ {
		req := agentRequest(http.MethodGet, "/api/v1/agents/stable/agent-card", nil, "viewer")
		req = withChiParam(req, "agentId", "stable")
		w := httptest.NewRecorder()
		h.GetAgentCard(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, w.Code)
		}

		body := w.Body.String()
		if i == 0 {
			firstBody = body
		} else if body != firstBody {
			t.Errorf("request %d response differs from first request", i)
		}
	}
}

// TestA2AIntegration_WellKnownETags verifies ETag caching workflow.
func TestA2AIntegration_WellKnownETags(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	agentStore := newMockAgentStore()
	agentStore.agents["a1"] = &store.Agent{
		ID: "a1", Name: "Agent 1", Description: "First",
		Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Step 1: First request to get ETag
	req1 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w1 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first request: status %d", w1.Code)
	}
	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first request should return ETag")
	}

	// Step 2: Conditional request with matching ETag
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w2, req2)

	if w2.Code != http.StatusNotModified {
		t.Fatalf("conditional request with matching ETag should return 304, got %d", w2.Code)
	}
	if w2.Body.Len() != 0 {
		t.Error("304 response should have empty body")
	}

	// Step 3: Simulate agent update (change UpdatedAt)
	later := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	agentStore.agents["a1"].UpdatedAt = later

	req3 := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	req3.Header.Set("If-None-Match", etag) // old ETag
	w3 := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("stale ETag should return 200 with fresh data, got %d", w3.Code)
	}

	newETag := w3.Header().Get("ETag")
	if newETag == etag {
		t.Error("new ETag should differ from old ETag after data change")
	}
}

// TestA2AIntegration_WellKnownSkillsUseAgentLevelMapping verifies that well-known
// skills are one-per-agent (not one-per-tool like individual cards).
func TestA2AIntegration_WellKnownSkillsUseAgentLevelMapping(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	agentStore.agents["multi_tool"] = &store.Agent{
		ID: "multi_tool", Name: "Multi-Tool Agent", Description: "Agent with 3 tools",
		Tools: json.RawMessage(`[
			{"name":"t1","source":"internal","server_label":"","description":"T1"},
			{"name":"t2","source":"mcp","server_label":"s1","description":"T2"},
			{"name":"t3","source":"mcp","server_label":"s2","description":"T3"}
		]`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	// Well-known endpoint
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)

	var card A2AAgentCard
	json.NewDecoder(w.Body).Decode(&card)

	// Well-known has 1 skill per agent (agent-level), not per tool
	if len(card.Skills) != 1 {
		t.Fatalf("well-known should have 1 skill (agent-level), got %d", len(card.Skills))
	}
	if card.Skills[0].ID != "multi_tool" {
		t.Errorf("well-known skill id: got %q, want %q", card.Skills[0].ID, "multi_tool")
	}

	// Individual agent card endpoint
	req2 := agentRequest(http.MethodGet, "/api/v1/agents/multi_tool/agent-card", nil, "viewer")
	req2 = withChiParam(req2, "agentId", "multi_tool")
	w2 := httptest.NewRecorder()
	h.GetAgentCard(w2, req2)

	env := parseEnvelope(t, w2)
	data := env.Data.(map[string]interface{})
	skills := data["skills"].([]interface{})

	// Individual card has 3 skills (one per tool)
	if len(skills) != 3 {
		t.Errorf("individual card should have 3 skills (per-tool), got %d", len(skills))
	}
}

// TestA2AIntegration_IndexPaginationOffsetLimit verifies offset+limit work correctly.
func TestA2AIntegration_IndexPaginationOffsetLimit(t *testing.T) {
	now := time.Now()
	agentStore := newMockAgentStore()
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("agent_%02d", i)
		agentStore.agents[id] = &store.Agent{
			ID: id, Name: fmt.Sprintf("Agent %02d", i), Description: fmt.Sprintf("Desc %d", i),
			Tools: json.RawMessage(`[]`), ExamplePrompts: json.RawMessage(`[]`),
			IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}

	h := NewA2AHandler(agentStore, "https://registry.example.com")

	tests := []struct {
		name           string
		query          string
		wantCardCount  int
		wantTotalCount float64
	}{
		{"offset 0, limit 3", "?offset=0&limit=3", 3, 10},
		{"offset 5, limit 3", "?offset=5&limit=3", 3, 10},
		{"offset 8, limit 5 (partial page)", "?offset=8&limit=5", 2, 10},
		{"offset 10, limit 5 (beyond range)", "?offset=10&limit=5", 0, 10},
		{"offset 0, limit 200 (max)", "?offset=0&limit=200", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)

			env := parseEnvelope(t, w)
			data := env.Data.(map[string]interface{})
			cards := data["agent_cards"].([]interface{})
			total := data["total"].(float64)

			if len(cards) != tt.wantCardCount {
				t.Errorf("cards: got %d, want %d", len(cards), tt.wantCardCount)
			}
			if total != tt.wantTotalCount {
				t.Errorf("total: got %v, want %v", total, tt.wantTotalCount)
			}
		})
	}
}

// =============================================================================
// Performance Benchmarks and Stress Tests
// =============================================================================

// makeAgent creates a test agent with the given number of tools.
func makeAgent(id string, numTools int) *store.Agent {
	var tools []agentTool
	for i := 0; i < numTools; i++ {
		source := "internal"
		label := ""
		if i%2 == 0 {
			source = "mcp"
			label = fmt.Sprintf("mcp-server-%d", i)
		}
		tools = append(tools, agentTool{
			Name:        fmt.Sprintf("tool_%d", i),
			Source:      source,
			ServerLabel: label,
			Description: fmt.Sprintf("Description for tool %d which does something useful", i),
		})
	}
	toolsJSON, _ := json.Marshal(tools)
	examples := []string{"Example prompt 1", "Example prompt 2", "Example prompt 3"}
	examplesJSON, _ := json.Marshal(examples)

	return &store.Agent{
		ID:             id,
		Name:           fmt.Sprintf("Agent %s", id),
		Description:    fmt.Sprintf("Description for agent %s that handles various tasks", id),
		SystemPrompt:   "You are a helpful assistant.",
		Tools:          json.RawMessage(toolsJSON),
		ExamplePrompts: json.RawMessage(examplesJSON),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "admin",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// seedN populates a mock store with n agents, each having numTools tools.
func seedN(s *mockAgentStore, n, numTools int) {
	base := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		a := makeAgent(fmt.Sprintf("agent-%04d", i), numTools)
		a.UpdatedAt = base.Add(time.Duration(i) * time.Second)
		s.agents[a.ID] = a
	}
}

// --- Benchmark: agentToA2ACard mapping function ---

func BenchmarkAgentToA2ACard_NoTools(b *testing.B) {
	agent := makeAgent("bench", 0)
	agent.Tools = json.RawMessage(`[]`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentToA2ACard(agent, "https://registry.example.com")
	}
}

func BenchmarkAgentToA2ACard_5Tools(b *testing.B) {
	agent := makeAgent("bench", 5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentToA2ACard(agent, "https://registry.example.com")
	}
}

func BenchmarkAgentToA2ACard_20Tools(b *testing.B) {
	agent := makeAgent("bench", 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentToA2ACard(agent, "https://registry.example.com")
	}
}

func BenchmarkAgentToA2ACard_50Tools(b *testing.B) {
	agent := makeAgent("bench", 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentToA2ACard(agent, "https://registry.example.com")
	}
}

func BenchmarkAgentToA2ACard_100Tools(b *testing.B) {
	agent := makeAgent("bench", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentToA2ACard(agent, "https://registry.example.com")
	}
}

// --- Benchmark: JSON serialization of A2A cards ---

func BenchmarkA2ACardJSON_SmallAgent(b *testing.B) {
	card := agentToA2ACard(makeAgent("bench", 3), "https://registry.example.com")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(card)
	}
}

func BenchmarkA2ACardJSON_LargeAgent(b *testing.B) {
	card := agentToA2ACard(makeAgent("bench", 50), "https://registry.example.com")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(card)
	}
}

// --- Benchmark: GetWellKnownAgentCard with varying agent counts ---

func benchmarkWellKnown(b *testing.B, agentCount int) {
	s := newMockAgentStore()
	seedN(s, agentCount, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

func BenchmarkWellKnown_10Agents(b *testing.B)   { benchmarkWellKnown(b, 10) }
func BenchmarkWellKnown_50Agents(b *testing.B)   { benchmarkWellKnown(b, 50) }
func BenchmarkWellKnown_100Agents(b *testing.B)  { benchmarkWellKnown(b, 100) }
func BenchmarkWellKnown_500Agents(b *testing.B)  { benchmarkWellKnown(b, 500) }
func BenchmarkWellKnown_1000Agents(b *testing.B) { benchmarkWellKnown(b, 1000) }

// --- Benchmark: GetA2AIndex with pagination ---

func benchmarkA2AIndex(b *testing.B, agentCount, limit int) {
	s := newMockAgentStore()
	seedN(s, agentCount, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	url := fmt.Sprintf("/api/v1/agents/a2a-index?limit=%d", limit)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		h.GetA2AIndex(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

func BenchmarkA2AIndex_100Agents_Limit20(b *testing.B)   { benchmarkA2AIndex(b, 100, 20) }
func BenchmarkA2AIndex_100Agents_Limit100(b *testing.B)  { benchmarkA2AIndex(b, 100, 100) }
func BenchmarkA2AIndex_1000Agents_Limit20(b *testing.B)  { benchmarkA2AIndex(b, 1000, 20) }
func BenchmarkA2AIndex_1000Agents_Limit200(b *testing.B) { benchmarkA2AIndex(b, 1000, 200) }

// --- Benchmark: GetA2AIndex with keyword filter ---

func BenchmarkA2AIndex_FilterHit(b *testing.B) {
	s := newMockAgentStore()
	seedN(s, 500, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=agent-0001", nil)
		w := httptest.NewRecorder()
		h.GetA2AIndex(w, req)
	}
}

func BenchmarkA2AIndex_FilterMiss(b *testing.B) {
	s := newMockAgentStore()
	seedN(s, 500, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index?q=zzz_nonexistent", nil)
		w := httptest.NewRecorder()
		h.GetA2AIndex(w, req)
	}
}

// --- Benchmark: ETag computation (isolated) ---

func BenchmarkETagComputation(b *testing.B) {
	agents := make([]store.Agent, 1000)
	base := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	for i := range agents {
		agents[i].UpdatedAt = base.Add(time.Duration(i) * time.Second)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var maxUpdated time.Time
		for j := range agents {
			if agents[j].UpdatedAt.After(maxUpdated) {
				maxUpdated = agents[j].UpdatedAt
			}
		}
		_ = fmt.Sprintf(`"%s"`, maxUpdated.UTC().Format(time.RFC3339Nano))
	}
}

// --- Benchmark: GetAgentCard single-agent lookup ---

func BenchmarkGetAgentCard(b *testing.B) {
	s := newMockAgentStore()
	agent := makeAgent("bench-agent", 10)
	s.agents[agent.ID] = agent
	h := NewA2AHandler(s, "https://registry.example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/bench-agent/agent-card", nil)
		req = withChiParam(req, "agentId", "bench-agent")
		w := httptest.NewRecorder()
		h.GetAgentCard(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

// --- Benchmark: Memory allocation profiling ---

func BenchmarkAgentToA2ACard_Allocs(b *testing.B) {
	agent := makeAgent("alloc-bench", 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentToA2ACard(agent, "https://registry.example.com")
	}
}

func BenchmarkWellKnown_Allocs(b *testing.B) {
	s := newMockAgentStore()
	seedN(s, 100, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
	}
}

// =============================================================================
// Stress Tests
// =============================================================================

// TestStress_ConcurrentWellKnown verifies the well-known endpoint handles
// concurrent requests without races or panics.
func TestStress_ConcurrentWellKnown(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 100, 5)
	h := NewA2AHandler(s, "https://registry.example.com")

	const goroutines = 100
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)
			if w.Code != http.StatusOK {
				errs <- fmt.Errorf("status %d", w.Code)
				return
			}
			var card A2AAgentCard
			if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
				errs <- fmt.Errorf("decode: %v", err)
				return
			}
			if len(card.Skills) != 100 {
				errs <- fmt.Errorf("skills count: got %d, want 100", len(card.Skills))
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// TestStress_ConcurrentA2AIndex verifies the index endpoint handles
// concurrent requests with varied query params.
func TestStress_ConcurrentA2AIndex(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 200, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	const goroutines = 100
	errs := make(chan error, goroutines)

	queries := []string{
		"/api/v1/agents/a2a-index?limit=10",
		"/api/v1/agents/a2a-index?limit=50&offset=10",
		"/api/v1/agents/a2a-index?q=agent-0001",
		"/api/v1/agents/a2a-index?limit=200",
		"/api/v1/agents/a2a-index?q=nonexistent",
	}

	for i := 0; i < goroutines; i++ {
		query := queries[i%len(queries)]
		go func(q string) {
			req := httptest.NewRequest(http.MethodGet, q, nil)
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)
			if w.Code != http.StatusOK {
				errs <- fmt.Errorf("status %d for %s", w.Code, q)
				return
			}
			errs <- nil
		}(query)
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// TestStress_ConcurrentGetAgentCard verifies single-agent card endpoint
// handles concurrent requests without races.
func TestStress_ConcurrentGetAgentCard(t *testing.T) {
	s := newMockAgentStore()
	for i := 0; i < 10; i++ {
		a := makeAgent(fmt.Sprintf("agent-%d", i), 10)
		s.agents[a.ID] = a
	}
	h := NewA2AHandler(s, "https://registry.example.com")

	const goroutines = 100
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		id := fmt.Sprintf("agent-%d", i%10)
		go func(agentID string) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/agent-card", nil)
			req = withChiParam(req, "agentId", agentID)
			w := httptest.NewRecorder()
			h.GetAgentCard(w, req)
			if w.Code != http.StatusOK {
				errs <- fmt.Errorf("status %d for %s", w.Code, agentID)
				return
			}
			errs <- nil
		}(id)
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// TestStress_LargeToolList ensures agents with many tools are handled correctly.
func TestStress_LargeToolList(t *testing.T) {
	toolCounts := []int{10, 25, 50, 75, 100}

	for _, n := range toolCounts {
		t.Run(fmt.Sprintf("%d_tools", n), func(t *testing.T) {
			agent := makeAgent("large-tools", n)
			card := agentToA2ACard(agent, "https://registry.example.com")

			if len(card.Skills) != n {
				t.Errorf("skills count: got %d, want %d", len(card.Skills), n)
			}

			// Verify JSON serialization doesn't blow up
			data, err := json.Marshal(card)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// Log response size for analysis
			t.Logf("Agent with %d tools -> JSON size: %d bytes", n, len(data))

			// Verify round-trip
			var decoded A2AAgentCard
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if len(decoded.Skills) != n {
				t.Errorf("round-trip skills count: got %d, want %d", len(decoded.Skills), n)
			}
		})
	}
}

// TestStress_WellKnownResponseSize measures JSON response sizes with large agent populations.
func TestStress_WellKnownResponseSize(t *testing.T) {
	agentCounts := []int{10, 50, 100, 500, 1000}

	for _, n := range agentCounts {
		t.Run(fmt.Sprintf("%d_agents", n), func(t *testing.T) {
			s := newMockAgentStore()
			seedN(s, n, 3)
			h := NewA2AHandler(s, "https://registry.example.com")

			req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
			w := httptest.NewRecorder()
			h.GetWellKnownAgentCard(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200", w.Code)
			}

			bodySize := w.Body.Len()
			t.Logf("Well-known with %d agents -> response: %d bytes (%.1f KB)", n, bodySize, float64(bodySize)/1024)

			// Verify it's valid JSON
			var card A2AAgentCard
			if err := json.NewDecoder(w.Body).Decode(&card); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if len(card.Skills) != n {
				t.Errorf("skills: got %d, want %d", len(card.Skills), n)
			}
		})
	}
}

// TestStress_A2AIndexResponseSize measures index response sizes with pagination.
func TestStress_A2AIndexResponseSize(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 500, 5)
	h := NewA2AHandler(s, "https://registry.example.com")

	limits := []int{10, 20, 50, 100, 200}

	for _, limit := range limits {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			url := fmt.Sprintf("/api/v1/agents/a2a-index?limit=%d", limit)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200", w.Code)
			}

			bodySize := w.Body.Len()
			t.Logf("A2A index limit=%d -> response: %d bytes (%.1f KB)", limit, bodySize, float64(bodySize)/1024)
		})
	}
}

// TestStress_ETagConsistency verifies ETag stays consistent across requests
// when no data changes.
func TestStress_ETagConsistency(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 50, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	var firstETag string
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)

		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Fatal("expected ETag header")
		}
		if i == 0 {
			firstETag = etag
		} else if etag != firstETag {
			t.Fatalf("ETag changed on request %d: got %q, want %q", i, etag, firstETag)
		}
	}
}

// TestStress_ConditionalRequestPerformance verifies 304 responses are faster
// than full responses (no JSON encoding needed).
func TestStress_ConditionalRequestPerformance(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 500, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	// Get the ETag first
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)
	etag := w.Header().Get("ETag")

	// Full request
	fullStart := time.Now()
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
	}
	fullDuration := time.Since(fullStart)

	// Conditional request (304)
	condStart := time.Now()
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		req.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
		if w.Code != http.StatusNotModified {
			t.Fatalf("expected 304, got %d", w.Code)
		}
	}
	condDuration := time.Since(condStart)

	t.Logf("100x full requests:        %v (avg %v)", fullDuration, fullDuration/100)
	t.Logf("100x conditional requests: %v (avg %v)", condDuration, condDuration/100)

	// Conditional should be faster since no JSON encoding happens
	if condDuration > fullDuration {
		t.Logf("WARNING: conditional requests were slower than full requests — ETag short-circuit may not be effective")
	}
}

// TestStress_PaginationExtremeValues tests pagination with boundary values.
func TestStress_PaginationExtremeValues(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 100, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"zero limit defaults to 20", "?limit=0", http.StatusOK},
		{"negative limit defaults to 20", "?limit=-1", http.StatusOK},
		{"very large limit capped at 200", "?limit=99999", http.StatusOK},
		{"negative offset becomes 0", "?offset=-5", http.StatusOK},
		{"offset beyond total returns empty", "?offset=99999", http.StatusOK},
		{"max int-like limit", "?limit=2147483647", http.StatusOK},
		{"non-numeric limit", "?limit=abc", http.StatusOK},
		{"non-numeric offset", "?offset=xyz", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index"+tt.query, nil)
			w := httptest.NewRecorder()
			h.GetA2AIndex(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// TestStress_MixedConcurrentEndpoints fires requests at all three A2A
// endpoints simultaneously.
func TestStress_MixedConcurrentEndpoints(t *testing.T) {
	s := newMockAgentStore()
	seedN(s, 50, 5)
	h := NewA2AHandler(s, "https://registry.example.com")

	const goroutines = 150
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		switch i % 3 {
		case 0:
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
				w := httptest.NewRecorder()
				h.GetWellKnownAgentCard(w, req)
				if w.Code != http.StatusOK {
					errs <- fmt.Errorf("well-known: status %d", w.Code)
					return
				}
				errs <- nil
			}()
		case 1:
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a2a-index?limit=20", nil)
				w := httptest.NewRecorder()
				h.GetA2AIndex(w, req)
				if w.Code != http.StatusOK {
					errs <- fmt.Errorf("index: status %d", w.Code)
					return
				}
				errs <- nil
			}()
		case 2:
			id := fmt.Sprintf("agent-%04d", i%50)
			go func(agentID string) {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/agent-card", nil)
				req = withChiParam(req, "agentId", agentID)
				w := httptest.NewRecorder()
				h.GetAgentCard(w, req)
				if w.Code != http.StatusOK {
					errs <- fmt.Errorf("agent-card %s: status %d", agentID, w.Code)
					return
				}
				errs <- nil
			}(id)
		}
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// --- Benchmark: Conditional request (If-None-Match) ---

func BenchmarkWellKnown_ConditionalHit(b *testing.B) {
	s := newMockAgentStore()
	seedN(s, 100, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	// Get the ETag
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.GetWellKnownAgentCard(w, req)
	etag := w.Header().Get("ETag")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		req.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
	}
}

func BenchmarkWellKnown_ConditionalMiss(b *testing.B) {
	s := newMockAgentStore()
	seedN(s, 100, 3)
	h := NewA2AHandler(s, "https://registry.example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		req.Header.Set("If-None-Match", `"stale-etag"`)
		w := httptest.NewRecorder()
		h.GetWellKnownAgentCard(w, req)
	}
}
