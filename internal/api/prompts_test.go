package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock prompt store ---

type mockPromptStore struct {
	prompts   map[uuid.UUID]*store.Prompt
	byAgent   map[string][]uuid.UUID // agentID -> prompt IDs in order
	createErr error
}

func newMockPromptStore() *mockPromptStore {
	return &mockPromptStore{
		prompts: make(map[uuid.UUID]*store.Prompt),
		byAgent: make(map[string][]uuid.UUID),
	}
}

func (m *mockPromptStore) List(_ context.Context, agentID string, activeOnly bool, offset, limit int) ([]store.Prompt, int, error) {
	ids := m.byAgent[agentID]
	var all []store.Prompt
	for _, id := range ids {
		p := m.prompts[id]
		if activeOnly && !p.IsActive {
			continue
		}
		all = append(all, *p)
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

func (m *mockPromptStore) GetActive(_ context.Context, agentID string) (*store.Prompt, error) {
	ids := m.byAgent[agentID]
	for _, id := range ids {
		p := m.prompts[id]
		if p.IsActive {
			return p, nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: active prompt not found")
}

func (m *mockPromptStore) GetByID(_ context.Context, id uuid.UUID) (*store.Prompt, error) {
	p, ok := m.prompts[id]
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: prompt '%s' not found", id)
	}
	return p, nil
}

func (m *mockPromptStore) Create(_ context.Context, prompt *store.Prompt) error {
	if m.createErr != nil {
		return m.createErr
	}
	// Auto-assign version
	ids := m.byAgent[prompt.AgentID]
	maxVersion := 0
	for _, id := range ids {
		if m.prompts[id].Version > maxVersion {
			maxVersion = m.prompts[id].Version
		}
	}

	// Deactivate current active
	for _, id := range ids {
		m.prompts[id].IsActive = false
	}

	prompt.Version = maxVersion + 1
	prompt.IsActive = true
	prompt.ID = uuid.New()
	prompt.CreatedAt = time.Now()
	m.prompts[prompt.ID] = prompt
	m.byAgent[prompt.AgentID] = append(m.byAgent[prompt.AgentID], prompt.ID)
	return nil
}

func (m *mockPromptStore) Activate(_ context.Context, id uuid.UUID) (*store.Prompt, error) {
	target, ok := m.prompts[id]
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: prompt '%s' not found", id)
	}
	// Deactivate all for this agent
	for _, pid := range m.byAgent[target.AgentID] {
		m.prompts[pid].IsActive = false
	}
	target.IsActive = true
	return target, nil
}

func (m *mockPromptStore) Rollback(_ context.Context, agentID string, targetVersion int, actor string) (*store.Prompt, error) {
	// Find target version
	ids := m.byAgent[agentID]
	var target *store.Prompt
	for _, id := range ids {
		if m.prompts[id].Version == targetVersion {
			target = m.prompts[id]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("NOT_FOUND: prompt version %d not found", targetVersion)
	}

	// Create new prompt from target
	newPrompt := &store.Prompt{
		AgentID:      agentID,
		SystemPrompt: target.SystemPrompt,
		TemplateVars: target.TemplateVars,
		Mode:         target.Mode,
		CreatedBy:    actor,
	}
	if err := m.Create(context.Background(), newPrompt); err != nil {
		return nil, err
	}
	return newPrompt, nil
}

func (m *mockPromptStore) GetByVersion(_ context.Context, agentID string, version int) (*store.Prompt, error) {
	ids := m.byAgent[agentID]
	for _, id := range ids {
		if m.prompts[id].Version == version {
			return m.prompts[id], nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: prompt '%s/v%d' not found", agentID, version)
}

// --- Mock agent lookup for prompts handler ---

type mockAgentLookupForPrompts struct {
	agents map[string]bool
}

func (m *mockAgentLookupForPrompts) GetByID(_ context.Context, id string) (*store.Agent, error) {
	if !m.agents[id] {
		return nil, fmt.Errorf("NOT_FOUND: agent '%s' not found", id)
	}
	return &store.Agent{ID: id, Name: "Mock Agent"}, nil
}

// --- Prompts handler tests ---

func TestPromptsHandler_Create(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		body       map[string]interface{}
		wantStatus int
		wantErr    bool
	}{
		{
			name:    "valid prompt creation",
			agentID: "test_agent",
			body: map[string]interface{}{
				"system_prompt": "You are a test agent for {{workspace_name}}.",
				"template_vars": map[string]string{"workspace_name": "", "current_date": ""},
				"mode":          "toolcalling_safe",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:    "missing system_prompt",
			agentID: "test_agent",
			body: map[string]interface{}{
				"mode": "toolcalling_safe",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:    "invalid mode",
			agentID: "test_agent",
			body: map[string]interface{}{
				"system_prompt": "Some prompt",
				"mode":          "invalid_mode",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:    "agent not found",
			agentID: "nonexistent",
			body: map[string]interface{}{
				"system_prompt": "Some prompt",
				"mode":          "toolcalling_safe",
			},
			wantStatus: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptStore := newMockPromptStore()
			agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
			audit := &mockAuditStoreForAPI{}
			h := NewPromptsHandler(promptStore, agentLookup, audit)

			req := agentRequest(http.MethodPost, "/api/v1/agents/"+tt.agentID+"/prompts", tt.body, "editor")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestPromptsHandler_CreateAutoVersions(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	// Create 3 prompts sequentially
	for i := 1; i <= 3; i++ {
		body := map[string]interface{}{
			"system_prompt": fmt.Sprintf("Prompt version %d", i),
			"mode":          "toolcalling_safe",
		}
		req := agentRequest(http.MethodPost, "/api/v1/agents/test_agent/prompts", body, "editor")
		req = withChiParam(req, "agentId", "test_agent")
		w := httptest.NewRecorder()
		h.Create(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("prompt %d: expected 201, got %d; body: %s", i, w.Code, w.Body.String())
		}
	}

	// Verify only the latest is active
	activeCount := 0
	for _, p := range promptStore.prompts {
		if p.IsActive {
			activeCount++
			if p.Version != 3 {
				t.Fatalf("expected active version to be 3, got %d", p.Version)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active prompt, got %d", activeCount)
	}

	// Verify versions auto-incremented
	if len(promptStore.prompts) != 3 {
		t.Fatalf("expected 3 prompts, got %d", len(promptStore.prompts))
	}
}

func TestPromptsHandler_GetActive(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	// Create a prompt
	p := &store.Prompt{
		ID:           uuid.New(),
		AgentID:      "test_agent",
		Version:      1,
		SystemPrompt: "Active prompt",
		TemplateVars: json.RawMessage(`{}`),
		Mode:         "toolcalling_safe",
		IsActive:     true,
		CreatedBy:    "system",
		CreatedAt:    time.Now(),
	}
	promptStore.prompts[p.ID] = p
	promptStore.byAgent["test_agent"] = []uuid.UUID{p.ID}

	tests := []struct {
		name       string
		agentID    string
		wantStatus int
	}{
		{
			name:       "get active prompt",
			agentID:    "test_agent",
			wantStatus: http.StatusOK,
		},
		{
			name:       "no active prompt for unknown agent",
			agentID:    "no_prompts",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/"+tt.agentID+"/prompts/active", nil, "viewer")
			req = withChiParam(req, "agentId", tt.agentID)
			w := httptest.NewRecorder()

			h.GetActive(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestPromptsHandler_GetByID(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	promptID := uuid.New()
	promptStore.prompts[promptID] = &store.Prompt{
		ID:           promptID,
		AgentID:      "test_agent",
		Version:      1,
		SystemPrompt: "A prompt",
		TemplateVars: json.RawMessage(`{}`),
		Mode:         "toolcalling_safe",
		IsActive:     true,
		CreatedBy:    "system",
		CreatedAt:    time.Now(),
	}

	tests := []struct {
		name       string
		promptID   string
		wantStatus int
	}{
		{
			name:       "existing prompt",
			promptID:   promptID.String(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent prompt",
			promptID:   uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			promptID:   "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/test_agent/prompts/"+tt.promptID, nil, "viewer")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("agentId", "test_agent")
			rctx.URLParams.Add("promptId", tt.promptID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			w := httptest.NewRecorder()

			h.GetByID(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestPromptsHandler_Activate(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	// Create two prompts
	p1 := &store.Prompt{
		ID: uuid.New(), AgentID: "test_agent", Version: 1,
		SystemPrompt: "V1", TemplateVars: json.RawMessage(`{}`),
		Mode: "toolcalling_safe", IsActive: false, CreatedBy: "system", CreatedAt: time.Now(),
	}
	p2 := &store.Prompt{
		ID: uuid.New(), AgentID: "test_agent", Version: 2,
		SystemPrompt: "V2", TemplateVars: json.RawMessage(`{}`),
		Mode: "toolcalling_safe", IsActive: true, CreatedBy: "system", CreatedAt: time.Now(),
	}
	promptStore.prompts[p1.ID] = p1
	promptStore.prompts[p2.ID] = p2
	promptStore.byAgent["test_agent"] = []uuid.UUID{p1.ID, p2.ID}

	// Activate p1
	req := agentRequest(http.MethodPost, "/api/v1/agents/test_agent/prompts/"+p1.ID.String()+"/activate", nil, "editor")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agentId", "test_agent")
	rctx.URLParams.Add("promptId", p1.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.Activate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify p1 is active, p2 is not
	if !p1.IsActive {
		t.Fatal("expected p1 to be active")
	}
	if p2.IsActive {
		t.Fatal("expected p2 to be inactive")
	}
}

func TestPromptsHandler_Rollback(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	// Seed two versions
	p1ID := uuid.New()
	p2ID := uuid.New()
	promptStore.prompts[p1ID] = &store.Prompt{
		ID: p1ID, AgentID: "test_agent", Version: 1,
		SystemPrompt: "V1 prompt", TemplateVars: json.RawMessage(`{"key": "val"}`),
		Mode: "rag_readonly", IsActive: false, CreatedBy: "system", CreatedAt: time.Now(),
	}
	promptStore.prompts[p2ID] = &store.Prompt{
		ID: p2ID, AgentID: "test_agent", Version: 2,
		SystemPrompt: "V2 prompt", TemplateVars: json.RawMessage(`{}`),
		Mode: "toolcalling_safe", IsActive: true, CreatedBy: "system", CreatedAt: time.Now(),
	}
	promptStore.byAgent["test_agent"] = []uuid.UUID{p1ID, p2ID}

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "rollback to version 1",
			body:       map[string]interface{}{"target_version": 1},
			wantStatus: http.StatusOK,
		},
		{
			name:       "rollback to non-existent version",
			body:       map[string]interface{}{"target_version": 999},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing target_version",
			body:       map[string]interface{}{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodPost, "/api/v1/agents/test_agent/prompts/rollback", tt.body, "editor")
			req = withChiParam(req, "agentId", "test_agent")
			w := httptest.NewRecorder()

			h.Rollback(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestPromptsHandler_RollbackCreatesN1(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	// Seed two versions
	p1ID := uuid.New()
	p2ID := uuid.New()
	promptStore.prompts[p1ID] = &store.Prompt{
		ID: p1ID, AgentID: "test_agent", Version: 1,
		SystemPrompt: "V1 original", TemplateVars: json.RawMessage(`{}`),
		Mode: "rag_readonly", IsActive: false, CreatedBy: "system", CreatedAt: time.Now(),
	}
	promptStore.prompts[p2ID] = &store.Prompt{
		ID: p2ID, AgentID: "test_agent", Version: 2,
		SystemPrompt: "V2 current", TemplateVars: json.RawMessage(`{}`),
		Mode: "toolcalling_safe", IsActive: true, CreatedBy: "system", CreatedAt: time.Now(),
	}
	promptStore.byAgent["test_agent"] = []uuid.UUID{p1ID, p2ID}

	// Rollback to version 1
	req := agentRequest(http.MethodPost, "/api/v1/agents/test_agent/prompts/rollback",
		map[string]interface{}{"target_version": 1}, "editor")
	req = withChiParam(req, "agentId", "test_agent")
	w := httptest.NewRecorder()

	h.Rollback(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// The rolled-back prompt should be version 3
	version := int(data["version"].(float64))
	if version != 3 {
		t.Fatalf("expected rollback to create version 3, got %d", version)
	}

	// It should be active
	if !data["is_active"].(bool) {
		t.Fatal("expected rolled-back prompt to be active")
	}

	// It should have V1's content
	if data["system_prompt"].(string) != "V1 original" {
		t.Fatalf("expected system_prompt to be 'V1 original', got '%s'", data["system_prompt"])
	}
}

func TestPromptsHandler_List(t *testing.T) {
	promptStore := newMockPromptStore()
	agentLookup := &mockAgentLookupForPrompts{agents: map[string]bool{"test_agent": true}}
	audit := &mockAuditStoreForAPI{}
	h := NewPromptsHandler(promptStore, agentLookup, audit)

	// Seed prompts
	for i := 1; i <= 3; i++ {
		p := &store.Prompt{
			ID: uuid.New(), AgentID: "test_agent", Version: i,
			SystemPrompt: fmt.Sprintf("V%d", i), TemplateVars: json.RawMessage(`{}`),
			Mode: "toolcalling_safe", IsActive: i == 3, CreatedBy: "system", CreatedAt: time.Now(),
		}
		promptStore.prompts[p.ID] = p
		promptStore.byAgent["test_agent"] = append(promptStore.byAgent["test_agent"], p.ID)
	}

	tests := []struct {
		name       string
		query      string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "list all prompts",
			query:      "",
			wantCount:  3,
			wantStatus: http.StatusOK,
		},
		{
			name:       "list active only",
			query:      "?active_only=true",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "list with limit=1",
			query:      "?limit=1",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/agents/test_agent/prompts"+tt.query, nil, "viewer")
			req = withChiParam(req, "agentId", "test_agent")
			w := httptest.NewRecorder()

			h.List(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			env := parseEnvelope(t, w)
			data := env.Data.(map[string]interface{})
			prompts := data["prompts"].([]interface{})
			if len(prompts) != tt.wantCount {
				t.Fatalf("expected %d prompts, got %d", tt.wantCount, len(prompts))
			}
		})
	}
}
