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

// --- Mock trigger rule store ---

type mockTriggerRuleStore struct {
	rules     map[uuid.UUID]*store.TriggerRule
	createErr error
	updateErr error
}

func newMockTriggerRuleStore() *mockTriggerRuleStore {
	return &mockTriggerRuleStore{
		rules: make(map[uuid.UUID]*store.TriggerRule),
	}
}

func (m *mockTriggerRuleStore) List(_ context.Context, workspaceID uuid.UUID) ([]store.TriggerRule, error) {
	var result []store.TriggerRule
	for _, r := range m.rules {
		if r.WorkspaceID == workspaceID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockTriggerRuleStore) GetByID(_ context.Context, id uuid.UUID) (*store.TriggerRule, error) {
	r, ok := m.rules[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return r, nil
}

func (m *mockTriggerRuleStore) Create(_ context.Context, rule *store.TriggerRule) error {
	if m.createErr != nil {
		return m.createErr
	}
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	m.rules[rule.ID] = rule
	return nil
}

func (m *mockTriggerRuleStore) Update(_ context.Context, rule *store.TriggerRule) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.rules[rule.ID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if !existing.UpdatedAt.Equal(rule.UpdatedAt) {
		return fmt.Errorf("conflict")
	}
	rule.UpdatedAt = time.Now()
	m.rules[rule.ID] = rule
	return nil
}

func (m *mockTriggerRuleStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.rules[id]; !ok {
		return fmt.Errorf("not found")
	}
	delete(m.rules, id)
	return nil
}

// --- Mock agent existence checker for trigger rules ---

type mockAgentExistenceChecker struct {
	agents map[string]bool
}

func (m *mockAgentExistenceChecker) AgentExists(_ context.Context, agentID string) (bool, error) {
	return m.agents[agentID], nil
}

// --- Trigger rules handler tests ---

func TestTriggerRulesHandler_Create(t *testing.T) {
	wsID := uuid.New()

	tests := []struct {
		name       string
		body       map[string]interface{}
		agents     map[string]bool
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid trigger rule",
			body: map[string]interface{}{
				"name":       "On push deploy",
				"event_type": "push",
				"agent_id":   "agent-1",
			},
			agents:     map[string]bool{"agent-1": true},
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid with schedule",
			body: map[string]interface{}{
				"name":       "Scheduled check",
				"event_type": "scheduled_tick",
				"agent_id":   "agent-2",
				"schedule":   "*/5 * * * *",
			},
			agents:     map[string]bool{"agent-2": true},
			wantStatus: http.StatusCreated,
		},
		{
			name: "invalid event_type rejected",
			body: map[string]interface{}{
				"name":       "Bad event",
				"event_type": "unknown_event",
				"agent_id":   "agent-1",
			},
			agents:     map[string]bool{"agent-1": true},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid schedule rejected",
			body: map[string]interface{}{
				"name":       "Bad cron",
				"event_type": "push",
				"agent_id":   "agent-1",
				"schedule":   "not a cron expression",
			},
			agents:     map[string]bool{"agent-1": true},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing name rejected",
			body: map[string]interface{}{
				"event_type": "push",
				"agent_id":   "agent-1",
			},
			agents:     map[string]bool{"agent-1": true},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing event_type rejected",
			body: map[string]interface{}{
				"name":     "No event",
				"agent_id": "agent-1",
			},
			agents:     map[string]bool{"agent-1": true},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing agent_id rejected",
			body: map[string]interface{}{
				"name":       "No agent",
				"event_type": "push",
			},
			agents:     map[string]bool{},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "non-existent agent_id rejected",
			body: map[string]interface{}{
				"name":       "Ghost agent",
				"event_type": "push",
				"agent_id":   "no-such-agent",
			},
			agents:     map[string]bool{"agent-1": true},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggerStore := newMockTriggerRuleStore()
			audit := &mockAuditStoreForAPI{}
			agentLookup := &mockAgentExistenceChecker{agents: tt.agents}
			h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

			req := adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules", tt.body)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			if tt.wantErr && env.Success {
				t.Fatal("expected success=false")
			}
		})
	}
}

func TestTriggerRulesHandler_Get(t *testing.T) {
	wsID := uuid.New()
	triggerStore := newMockTriggerRuleStore()
	audit := &mockAuditStoreForAPI{}
	agentLookup := &mockAgentExistenceChecker{agents: map[string]bool{}}
	h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

	triggerID := uuid.New()
	triggerStore.rules[triggerID] = &store.TriggerRule{
		ID:          triggerID,
		WorkspaceID: wsID,
		Name:        "Test trigger",
		EventType:   "push",
		AgentID:     "agent-1",
		Condition:   json.RawMessage(`{}`),
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	tests := []struct {
		name       string
		triggerID  string
		wantStatus int
	}{
		{
			name:       "existing trigger",
			triggerID:  triggerID.String(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent trigger",
			triggerID:  uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			triggerID:  "bad-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodGet, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules/"+tt.triggerID, nil)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			rctx.URLParams.Add("triggerId", tt.triggerID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Get(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestTriggerRulesHandler_List(t *testing.T) {
	wsID := uuid.New()
	otherWsID := uuid.New()
	triggerStore := newMockTriggerRuleStore()
	audit := &mockAuditStoreForAPI{}
	agentLookup := &mockAgentExistenceChecker{agents: map[string]bool{}}
	h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

	// Add triggers in different workspaces
	for i := 0; i < 3; i++ {
		id := uuid.New()
		triggerStore.rules[id] = &store.TriggerRule{
			ID:          id,
			WorkspaceID: wsID,
			Name:        fmt.Sprintf("Trigger %d", i),
			EventType:   "push",
			AgentID:     "agent-1",
			Condition:   json.RawMessage(`{}`),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}
	otherID := uuid.New()
	triggerStore.rules[otherID] = &store.TriggerRule{
		ID:          otherID,
		WorkspaceID: otherWsID,
		Name:        "Other workspace trigger",
		EventType:   "push",
		AgentID:     "agent-1",
		Condition:   json.RawMessage(`{}`),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wsID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	rules := data["rules"].([]interface{})
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
}

func TestTriggerRulesHandler_Update(t *testing.T) {
	wsID := uuid.New()
	triggerStore := newMockTriggerRuleStore()
	audit := &mockAuditStoreForAPI{}
	agentLookup := &mockAgentExistenceChecker{agents: map[string]bool{"agent-1": true, "agent-2": true}}
	h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

	triggerID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	triggerStore.rules[triggerID] = &store.TriggerRule{
		ID:          triggerID,
		WorkspaceID: wsID,
		Name:        "Original",
		EventType:   "push",
		AgentID:     "agent-1",
		Condition:   json.RawMessage(`{}`),
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   updatedAt,
	}

	tests := []struct {
		name       string
		triggerID  string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:      "valid update",
			triggerID: triggerID.String(),
			ifMatch:   updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"name":       "Updated trigger",
				"event_type": "webhook",
				"agent_id":   "agent-2",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing If-Match",
			triggerID:  triggerID.String(),
			ifMatch:    "",
			body:       map[string]interface{}{"name": "x"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-existent trigger",
			triggerID:  uuid.New().String(),
			ifMatch:    time.Now().UTC().Format(time.RFC3339Nano),
			body:       map[string]interface{}{"name": "x", "event_type": "push", "agent_id": "agent-1"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "invalid event_type on update",
			triggerID: triggerID.String(),
			ifMatch:   updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"name":       "Bad update",
				"event_type": "invalid_event",
				"agent_id":   "agent-1",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodPut, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules/"+tt.triggerID, tt.body)
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			rctx.URLParams.Add("triggerId", tt.triggerID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestTriggerRulesHandler_Delete(t *testing.T) {
	wsID := uuid.New()
	triggerStore := newMockTriggerRuleStore()
	audit := &mockAuditStoreForAPI{}
	agentLookup := &mockAgentExistenceChecker{agents: map[string]bool{}}
	h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

	triggerID := uuid.New()
	triggerStore.rules[triggerID] = &store.TriggerRule{
		ID:          triggerID,
		WorkspaceID: wsID,
		Name:        "To delete",
		EventType:   "push",
		AgentID:     "agent-1",
		Condition:   json.RawMessage(`{}`),
	}

	tests := []struct {
		name       string
		triggerID  string
		wantStatus int
	}{
		{
			name:       "delete existing",
			triggerID:  triggerID.String(),
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete non-existent",
			triggerID:  uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			triggerID:  "bad-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodDelete, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules/"+tt.triggerID, nil)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			rctx.URLParams.Add("triggerId", tt.triggerID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Delete(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestTriggerRulesHandler_ValidEventTypes(t *testing.T) {
	wsID := uuid.New()

	validTypes := []string{
		"push", "scheduled_tick", "webhook", "email.ingested",
		"calendar.change", "transcript.available", "drive.file_changed",
		"slack.message", "file_changed", "nudge_created",
	}

	for _, eventType := range validTypes {
		t.Run("event_"+eventType, func(t *testing.T) {
			triggerStore := newMockTriggerRuleStore()
			audit := &mockAuditStoreForAPI{}
			agentLookup := &mockAgentExistenceChecker{agents: map[string]bool{"agent-1": true}}
			h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

			body := map[string]interface{}{
				"name":       "Test " + eventType,
				"event_type": eventType,
				"agent_id":   "agent-1",
			}

			req := adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules", body)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Create(w, req)

			if w.Code != http.StatusCreated {
				t.Fatalf("event_type %s: expected 201, got %d; body: %s", eventType, w.Code, w.Body.String())
			}
		})
	}
}

func TestTriggerRulesHandler_ValidCronExpressions(t *testing.T) {
	wsID := uuid.New()

	validCrons := []string{
		"*/5 * * * *",
		"0 * * * *",
		"0 0 * * *",
		"0 0 * * MON",
		"30 4 1 * *",
	}

	for _, cron := range validCrons {
		t.Run("cron_"+cron, func(t *testing.T) {
			triggerStore := newMockTriggerRuleStore()
			audit := &mockAuditStoreForAPI{}
			agentLookup := &mockAgentExistenceChecker{agents: map[string]bool{"agent-1": true}}
			h := NewTriggerRulesHandler(triggerStore, audit, agentLookup, nil)

			body := map[string]interface{}{
				"name":       "Scheduled " + cron,
				"event_type": "scheduled_tick",
				"agent_id":   "agent-1",
				"schedule":   cron,
			}

			req := adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trigger-rules", body)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Create(w, req)

			if w.Code != http.StatusCreated {
				t.Fatalf("cron %s: expected 201, got %d; body: %s", cron, w.Code, w.Body.String())
			}
		})
	}
}
