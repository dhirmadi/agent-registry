package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
