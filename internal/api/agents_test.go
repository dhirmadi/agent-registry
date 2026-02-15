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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock agent store ---

type mockAgentStore struct {
	agents   map[string]*store.Agent
	versions map[string][]store.AgentVersion

	createErr   error
	getByIDErr  error
	listErr     error
	updateErr   error
	deleteErr   error
	rollbackErr error
}

func newMockAgentStore() *mockAgentStore {
	return &mockAgentStore{
		agents:   make(map[string]*store.Agent),
		versions: make(map[string][]store.AgentVersion),
	}
}

func (m *mockAgentStore) Create(_ context.Context, agent *store.Agent) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.agents[agent.ID]; exists {
		return apierrors.Conflict(fmt.Sprintf("agent '%s' already exists", agent.ID))
	}
	agent.Version = 1
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()
	m.agents[agent.ID] = agent
	m.versions[agent.ID] = []store.AgentVersion{
		{
			ID:             uuid.New(),
			AgentID:        agent.ID,
			Version:        1,
			Name:           agent.Name,
			Description:    agent.Description,
			SystemPrompt:   agent.SystemPrompt,
			Tools:          agent.Tools,
			TrustOverrides: agent.TrustOverrides,
			ExamplePrompts: agent.ExamplePrompts,
			IsActive:       agent.IsActive,
			CreatedBy:      agent.CreatedBy,
			CreatedAt:      agent.CreatedAt,
		},
	}
	return nil
}

func (m *mockAgentStore) GetByID(_ context.Context, id string) (*store.Agent, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	a, ok := m.agents[id]
	if !ok {
		return nil, apierrors.NotFound("agent", id)
	}
	return a, nil
}

func (m *mockAgentStore) List(_ context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var all []store.Agent
	for _, a := range m.agents {
		if activeOnly && !a.IsActive {
			continue
		}
		all = append(all, *a)
	}
	total := len(all)
	if offset >= len(all) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (m *mockAgentStore) Update(_ context.Context, agent *store.Agent, updatedAt time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.agents[agent.ID]
	if !ok {
		return apierrors.NotFound("agent", agent.ID)
	}
	if !existing.UpdatedAt.Equal(updatedAt) {
		return apierrors.Conflict("resource was modified by another client")
	}
	agent.Version = existing.Version + 1
	agent.UpdatedAt = time.Now()
	m.agents[agent.ID] = agent
	m.versions[agent.ID] = append(m.versions[agent.ID], store.AgentVersion{
		ID:             uuid.New(),
		AgentID:        agent.ID,
		Version:        agent.Version,
		Name:           agent.Name,
		Description:    agent.Description,
		SystemPrompt:   agent.SystemPrompt,
		Tools:          agent.Tools,
		TrustOverrides: agent.TrustOverrides,
		ExamplePrompts: agent.ExamplePrompts,
		IsActive:       agent.IsActive,
		CreatedBy:      agent.CreatedBy,
		CreatedAt:      time.Now(),
	})
	return nil
}

func (m *mockAgentStore) Patch(_ context.Context, id string, fields map[string]interface{}, updatedAt time.Time, actor string) (*store.Agent, error) {
	existing, ok := m.agents[id]
	if !ok {
		return nil, apierrors.NotFound("agent", id)
	}
	if !existing.UpdatedAt.Equal(updatedAt) {
		return nil, apierrors.Conflict("resource was modified by another client")
	}
	if v, ok := fields["name"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("name must be a string")
		}
		existing.Name = s
	}
	if v, ok := fields["description"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("description must be a string")
		}
		existing.Description = s
	}
	if v, ok := fields["is_active"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("is_active must be a boolean")
		}
		existing.IsActive = b
	}
	existing.Version++
	existing.UpdatedAt = time.Now()
	m.agents[id] = existing
	return existing, nil
}

func (m *mockAgentStore) Delete(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	a, ok := m.agents[id]
	if !ok {
		return apierrors.NotFound("agent", id)
	}
	a.IsActive = false
	return nil
}

func (m *mockAgentStore) ListVersions(_ context.Context, agentID string, offset, limit int) ([]store.AgentVersion, int, error) {
	versions := m.versions[agentID]
	total := len(versions)
	if offset >= len(versions) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(versions) {
		end = len(versions)
	}
	return versions[offset:end], total, nil
}

func (m *mockAgentStore) GetVersion(_ context.Context, agentID string, version int) (*store.AgentVersion, error) {
	versions := m.versions[agentID]
	for i := range versions {
		if versions[i].Version == version {
			return &versions[i], nil
		}
	}
	return nil, apierrors.NotFound("agent_version", fmt.Sprintf("%s/v%d", agentID, version))
}

func (m *mockAgentStore) Rollback(_ context.Context, agentID string, targetVersion int, actor string) (*store.Agent, error) {
	if m.rollbackErr != nil {
		return nil, m.rollbackErr
	}
	versions := m.versions[agentID]
	var target *store.AgentVersion
	for i := range versions {
		if versions[i].Version == targetVersion {
			target = &versions[i]
			break
		}
	}
	if target == nil {
		return nil, apierrors.NotFound("agent_version", fmt.Sprintf("%s/v%d", agentID, targetVersion))
	}

	existing := m.agents[agentID]
	existing.Name = target.Name
	existing.Description = target.Description
	existing.SystemPrompt = target.SystemPrompt
	existing.Tools = target.Tools
	existing.TrustOverrides = target.TrustOverrides
	existing.ExamplePrompts = target.ExamplePrompts
	existing.Version++
	existing.UpdatedAt = time.Now()
	existing.CreatedBy = actor
	m.agents[agentID] = existing
	return existing, nil
}

// --- Helper functions ---

func agentRequest(method, url string, body interface{}, role string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), role, "session")
	return req.WithContext(ctx)
}

func withChiParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// --- Agent handler tests ---

func TestAgentsHandler_Create(t *testing.T) {
	toolsJSON := json.RawMessage(`[
		{"name": "get_config", "source": "internal", "server_label": "", "description": "Get config"},
		{"name": "git_read", "source": "mcp", "server_label": "mcp-git", "description": "Read git file"},
		{"name": "search_docs", "source": "mcp", "server_label": "google-workspace-mcp", "description": "Search docs"}
	]`)

	tests := []struct {
		name       string
		body       map[string]interface{}
		role       string
		storeErr   error
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid agent creation",
			body: map[string]interface{}{
				"id":              "test_agent",
				"name":            "Test Agent",
				"description":     "A test agent",
				"system_prompt":   "You are a test agent.",
				"tools":           toolsJSON,
				"trust_overrides": map[string]string{"git_read": "auto"},
				"example_prompts": []string{"Do something"},
			},
			role:       "editor",
			wantStatus: http.StatusCreated,
		},
		{
			name: "invalid agent ID format - uppercase",
			body: map[string]interface{}{
				"id":   "BadId",
				"name": "Bad Agent",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid agent ID format - too short",
			body: map[string]interface{}{
				"id":   "a",
				"name": "Bad Agent",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid agent ID format - starts with number",
			body: map[string]interface{}{
				"id":   "1agent",
				"name": "Bad Agent",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing name",
			body: map[string]interface{}{
				"id": "no_name",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "duplicate agent returns 409",
			body: map[string]interface{}{
				"id":   "duplicate_agent",
				"name": "Duplicate Agent",
			},
			role:       "editor",
			storeErr:   apierrors.Conflict("agent 'duplicate_agent' already exists"),
			wantStatus: http.StatusConflict,
			wantErr:    true,
		},
		{
			name: "viewer cannot create",
			body: map[string]interface{}{
				"id":   "viewer_agent",
				"name": "Viewer Agent",
			},
			role:       "viewer",
			wantStatus: http.StatusForbidden,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			agentStore.createErr = tt.storeErr
			audit := &mockAuditStoreForAPI{}
			h := NewAgentsHandler(agentStore, audit, nil)

			req := agentRequest(http.MethodPost, "/api/v1/agents", tt.body, tt.role)
			w := httptest.NewRecorder()

			// Wrap with role middleware for viewer test
			if tt.role == "viewer" {
				mw := RequireRole("editor", "admin")
				handler := mw(http.HandlerFunc(h.Create))
				handler.ServeHTTP(w, req)
			} else {
				h.Create(w, req)
			}

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if !tt.wantErr {
				env := parseEnvelope(t, w)
				if !env.Success {
					t.Fatal("expected success=true")
				}
			}
		})
	}
}

func TestAgentsHandler_Get(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	testAgent := &store.Agent{
		ID:             "test_agent",
		Name:           "Test Agent",
		Description:    "A test agent",
		SystemPrompt:   "You are a test agent.",
		Tools:          json.RawMessage(`[{"name": "get_config", "source": "internal", "server_label": "", "description": "Get config"}]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`["Do something"]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	agentStore.agents[testAgent.ID] = testAgent

	tests := []struct {
		name       string
		agentID    string
		wantStatus int
	}{
		{
			name:       "existing agent",
			agentID:    "test_agent",
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent agent",
			agentID:    "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/"+tt.agentID, nil, "viewer")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.Get(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentsHandler_List(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	// Seed agents
	for i := 0; i < 3; i++ {
		a := &store.Agent{
			ID:             fmt.Sprintf("agent_%d", i),
			Name:           fmt.Sprintf("Agent %d", i),
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       i != 2, // Third agent is inactive
			Version:        1,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		agentStore.agents[a.ID] = a
	}

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "list active only (default)",
			query:      "",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "list all (active_only=false)",
			query:      "?active_only=false",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name:       "list with limit=1",
			query:      "?limit=1&active_only=false",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.List(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			env := parseEnvelope(t, w)
			if !env.Success {
				t.Fatal("expected success=true")
			}

			data, ok := env.Data.(map[string]interface{})
			if !ok {
				t.Fatal("expected data to be a map")
			}
			agents, ok := data["agents"].([]interface{})
			if !ok {
				t.Fatal("expected data.agents to be an array")
			}
			if len(agents) != tt.wantCount {
				t.Fatalf("expected %d agents, got %d", tt.wantCount, len(agents))
			}
		})
	}
}

func TestAgentsHandler_Update(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		agentID    string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:    "valid update with If-Match",
			agentID: "test_agent",
			ifMatch: now.Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"name":          "Updated Agent",
				"description":   "Updated description",
				"system_prompt": "Updated prompt",
				"tools":         []interface{}{},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing If-Match header",
			agentID:    "test_agent",
			ifMatch:    "",
			body:       map[string]interface{}{"name": "Updated"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-existent agent",
			agentID:    "nonexistent",
			ifMatch:    now.Format(time.RFC3339Nano),
			body:       map[string]interface{}{"name": "Updated"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:    "conflict - stale etag",
			agentID: "test_agent",
			ifMatch: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"name": "Updated Agent",
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentStore := newMockAgentStore()
			audit := &mockAuditStoreForAPI{}
			h := NewAgentsHandler(agentStore, audit, nil)

			// Seed agent
			agentStore.agents["test_agent"] = &store.Agent{
				ID:             "test_agent",
				Name:           "Test Agent",
				Description:    "Original",
				SystemPrompt:   "Original prompt",
				Tools:          json.RawMessage(`[]`),
				TrustOverrides: json.RawMessage(`{}`),
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        1,
				CreatedBy:      "system",
				CreatedAt:      now,
				UpdatedAt:      now,
			}

			req := agentRequest(http.MethodPut, "/api/v1/agents/"+tt.agentID, tt.body, "editor")
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentsHandler_Patch(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["test_agent"] = &store.Agent{
		ID:             "test_agent",
		Name:           "Test Agent",
		Description:    "Original",
		SystemPrompt:   "Original prompt",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	req := agentRequest(http.MethodPatch, "/api/v1/agents/test_agent", map[string]interface{}{
		"description": "Patched description",
	}, "editor")
	req.Header.Set("If-Match", now.Format(time.RFC3339Nano))
	req = withChiParam(req, "agentId", "test_agent")
	w := httptest.NewRecorder()

	h.PatchAgent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}
}

func TestAgentsHandler_Delete(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["test_agent"] = &store.Agent{
		ID:       "test_agent",
		Name:     "Test Agent",
		IsActive: true,
	}

	tests := []struct {
		name       string
		agentID    string
		wantStatus int
	}{
		{
			name:       "soft delete existing agent",
			agentID:    "test_agent",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete non-existent agent",
			agentID:    "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodDelete, "/api/v1/agents/"+tt.agentID, nil, "editor")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.Delete(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentsHandler_ListVersions(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["test_agent"] = &store.Agent{
		ID: "test_agent", Name: "Test Agent", Version: 2,
	}
	agentStore.versions["test_agent"] = []store.AgentVersion{
		{ID: uuid.New(), AgentID: "test_agent", Version: 1, Name: "V1", CreatedAt: time.Now()},
		{ID: uuid.New(), AgentID: "test_agent", Version: 2, Name: "V2", CreatedAt: time.Now()},
	}

	req := agentRequest(http.MethodGet, "/api/v1/agents/test_agent/versions", nil, "viewer")
	req = withChiParam(req, "agentId", "test_agent")
	w := httptest.NewRecorder()

	h.ListVersions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	versions := data["versions"].([]interface{})
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
}

func TestAgentsHandler_Rollback(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["test_agent"] = &store.Agent{
		ID:             "test_agent",
		Name:           "Test Agent V2",
		Description:    "V2 desc",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        2,
		UpdatedAt:      time.Now(),
	}
	agentStore.versions["test_agent"] = []store.AgentVersion{
		{
			ID: uuid.New(), AgentID: "test_agent", Version: 1, Name: "Test Agent V1",
			Description:    "V1 desc",
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
		},
		{
			ID: uuid.New(), AgentID: "test_agent", Version: 2, Name: "Test Agent V2",
			Description:    "V2 desc",
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
		},
	}

	tests := []struct {
		name       string
		agentID    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "rollback to version 1",
			agentID:    "test_agent",
			body:       map[string]interface{}{"target_version": 1},
			wantStatus: http.StatusOK,
		},
		{
			name:       "rollback to non-existent version",
			agentID:    "test_agent",
			body:       map[string]interface{}{"target_version": 999},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing target_version",
			agentID:    "test_agent",
			body:       map[string]interface{}{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodPost, "/api/v1/agents/"+tt.agentID+"/rollback", tt.body, "editor")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.Rollback(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentsHandler_DerivedFields(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["derived_test"] = &store.Agent{
		ID:   "derived_test",
		Name: "Derived Test",
		Tools: json.RawMessage(`[
			{"name": "get_config", "source": "internal", "server_label": "", "description": "Internal tool"},
			{"name": "search_docs", "source": "internal", "server_label": "", "description": "Another internal"},
			{"name": "git_read", "source": "mcp", "server_label": "mcp-git", "description": "Git read"},
			{"name": "gws_search", "source": "mcp", "server_label": "google-workspace-mcp", "description": "Google search"},
			{"name": "slack_post", "source": "mcp", "server_label": "slack-mcp", "description": "Slack post"},
			{"name": "slack_read", "source": "mcp", "server_label": "slack-mcp", "description": "Slack read"}
		]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	req := agentRequest(http.MethodGet, "/api/v1/agents/derived_test", nil, "viewer")
	req = withChiParam(req, "agentId", "derived_test")
	w := httptest.NewRecorder()

	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// Check capabilities (only internal tool names per spec)
	caps, ok := data["capabilities"].([]interface{})
	if !ok {
		t.Fatal("expected capabilities to be an array")
	}
	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities (internal tools only), got %d", len(caps))
	}

	// Check required_connections (unique mcp server_labels, excluding mcp-git per spec)
	conns, ok := data["required_connections"].([]interface{})
	if !ok {
		t.Fatal("expected required_connections to be an array")
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 required_connections (google-workspace-mcp, slack-mcp), got %d: %v", len(conns), conns)
	}
	for _, c := range conns {
		if c.(string) == "mcp-git" {
			t.Fatal("mcp-git should be excluded from required_connections")
		}
	}
}

func TestAgentsHandler_ListIncludeTools(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["tools_test"] = &store.Agent{
		ID:             "tools_test",
		Name:           "Tools Test",
		Tools:          json.RawMessage(`[{"name": "test_tool", "source": "internal", "server_label": "", "description": "A tool"}]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	tests := []struct {
		name         string
		query        string
		expectTools  bool
	}{
		{
			name:        "include_tools=true (default)",
			query:       "",
			expectTools: true,
		},
		{
			name:        "include_tools=false omits tools",
			query:       "?include_tools=false&active_only=false",
			expectTools: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.List(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}

			env := parseEnvelope(t, w)
			data := env.Data.(map[string]interface{})
			agents := data["agents"].([]interface{})
			if len(agents) == 0 {
				t.Fatal("expected at least 1 agent")
			}

			agent := agents[0].(map[string]interface{})
			_, hasTools := agent["tools"]
			if tt.expectTools && !hasTools {
				t.Fatal("expected tools to be present in response")
			}
			if !tt.expectTools && hasTools {
				t.Fatal("expected tools to be omitted from response")
			}
		})
	}
}

// --- Input validation gap tests ---
// These tests prove that the agent handler does NOT validate tool definitions.

func TestAgentsHandler_Create_RejectsInvalidToolSource(t *testing.T) {
	t.Parallel()

	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	invalidSources := []struct {
		name   string
		source string
	}{
		{"evil source", "evil"},
		{"empty source", ""},
		{"sql injection source", "'; DROP TABLE agents; --"},
		{"unknown source", "external"},
	}

	for _, tt := range invalidSources {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := map[string]interface{}{
				"id":   "tool_src_" + tt.name,
				"name": "Tool Source Test",
				"tools": []map[string]interface{}{
					{
						"name":         "bad_tool",
						"source":       tt.source,
						"server_label": "",
						"description":  "A tool with invalid source",
					},
				},
			}
			// Need a unique ID per subtest to avoid conflict
			body["id"] = fmt.Sprintf("tool_src_%d", len(tt.name))

			req := agentRequest(http.MethodPost, "/api/v1/agents", body, "editor")
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid tool source %q, got %d; body: %s",
					tt.source, w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentsHandler_Create_RejectsToolMissingServerLabel(t *testing.T) {
	t.Parallel()

	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	body := map[string]interface{}{
		"id":   "mcp_no_label",
		"name": "MCP No Label Test",
		"tools": []map[string]interface{}{
			{
				"name":         "mcp_tool",
				"source":       "mcp",
				"server_label": "", // MCP tool MUST have a server_label
				"description":  "An MCP tool without server_label",
			},
		},
	}

	req := agentRequest(http.MethodPost, "/api/v1/agents", body, "editor")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for MCP tool without server_label, got %d; body: %s",
			w.Code, w.Body.String())
	}
}

func TestAgentsHandler_RoleAccess(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["test_agent"] = &store.Agent{
		ID:             "test_agent",
		Name:           "Test Agent",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	tests := []struct {
		name       string
		method     string
		role       string
		wantStatus int
	}{
		{"viewer can GET", http.MethodGet, "viewer", http.StatusOK},
		{"editor can GET", http.MethodGet, "editor", http.StatusOK},
		{"admin can GET", http.MethodGet, "admin", http.StatusOK},
		{"viewer cannot POST", http.MethodPost, "viewer", http.StatusForbidden},
		{"editor can POST", http.MethodPost, "editor", http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			switch tt.method {
			case http.MethodGet:
				req = agentRequest(tt.method, "/api/v1/agents/test_agent", nil, tt.role)
				req = withChiParam(req, "agentId", "test_agent")

				mw := RequireRole("viewer", "editor", "admin")
				handler := mw(http.HandlerFunc(h.Get))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				if w.Code != tt.wantStatus {
					t.Fatalf("expected %d, got %d", tt.wantStatus, w.Code)
				}
			case http.MethodPost:
				body := map[string]interface{}{
					"id":   "new_agent",
					"name": "New Agent",
				}
				req = agentRequest(tt.method, "/api/v1/agents", body, tt.role)

				mw := RequireRole("editor", "admin")
				handler := mw(http.HandlerFunc(h.Create))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				if w.Code != tt.wantStatus {
					t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
				}
			}
		})
	}
}

// --- Audit quality tests ---

func TestAgentsHandler_List_PaginationUpperBound(t *testing.T) {
	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	// Seed 5 agents
	for i := 0; i < 5; i++ {
		a := &store.Agent{
			ID:             fmt.Sprintf("agent_%d", i),
			Name:           fmt.Sprintf("Agent %d", i),
			Tools:          json.RawMessage(`[]`),
			TrustOverrides: json.RawMessage(`{}`),
			ExamplePrompts: json.RawMessage(`[]`),
			IsActive:       true,
			Version:        1,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		agentStore.agents[a.ID] = a
	}

	// Request limit=500, should be capped to 200
	req := agentRequest(http.MethodGet, "/api/v1/agents?limit=500&active_only=false", nil, "viewer")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	agents := data["agents"].([]interface{})
	// We only seeded 5, so we should get 5 back (not 500)
	if len(agents) != 5 {
		t.Fatalf("expected 5 agents (capped at 200, but only 5 exist), got %d", len(agents))
	}
}

func TestAgentsHandler_Patch_InvalidTypeReturnsError(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	agentStore.agents["test_agent"] = &store.Agent{
		ID:             "test_agent",
		Name:           "Test Agent",
		Description:    "Original",
		SystemPrompt:   "Original prompt",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      "system",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Send name as number instead of string
	req := agentRequest(http.MethodPatch, "/api/v1/agents/test_agent", map[string]interface{}{
		"name": 123,
	}, "editor")
	req.Header.Set("If-Match", now.Format(time.RFC3339Nano))
	req = withChiParam(req, "agentId", "test_agent")
	w := httptest.NewRecorder()

	h.PatchAgent(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid type in PATCH, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAgentsHandler_Get_NonNotFoundErrorReturns500(t *testing.T) {
	agentStore := newMockAgentStore()
	agentStore.getByIDErr = fmt.Errorf("database connection lost")
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	req := agentRequest(http.MethodGet, "/api/v1/agents/test_agent", nil, "viewer")
	req = withChiParam(req, "agentId", "test_agent")
	w := httptest.NewRecorder()

	h.Get(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for database error, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	errObj := env.Error
	if errObj == nil {
		t.Fatal("expected error in response")
	}
}

func TestAgentsHandler_Update_PreservesCreatedBy(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	agentStore := newMockAgentStore()
	audit := &mockAuditStoreForAPI{}
	h := NewAgentsHandler(agentStore, audit, nil)

	originalCreator := "original-creator-id"
	agentStore.agents["test_agent"] = &store.Agent{
		ID:             "test_agent",
		Name:           "Test Agent",
		Description:    "Original",
		SystemPrompt:   "Original prompt",
		Tools:          json.RawMessage(`[]`),
		TrustOverrides: json.RawMessage(`{}`),
		ExamplePrompts: json.RawMessage(`[]`),
		IsActive:       true,
		Version:        1,
		CreatedBy:      originalCreator,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	req := agentRequest(http.MethodPut, "/api/v1/agents/test_agent", map[string]interface{}{
		"name":          "Updated Agent",
		"description":   "Updated description",
		"system_prompt": "Updated prompt",
	}, "editor")
	req.Header.Set("If-Match", now.Format(time.RFC3339Nano))
	req = withChiParam(req, "agentId", "test_agent")
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify created_by was not overwritten
	agent := agentStore.agents["test_agent"]
	if agent.CreatedBy != originalCreator {
		t.Fatalf("expected created_by to remain %q, got %q", originalCreator, agent.CreatedBy)
	}
}
