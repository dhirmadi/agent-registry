package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock trust rule store ---

type mockTrustRuleStore struct {
	rules map[uuid.UUID]*store.TrustRule
	// Index by workspace_id+tool_pattern for upsert
	index map[string]uuid.UUID
}

func newMockTrustRuleStore() *mockTrustRuleStore {
	return &mockTrustRuleStore{
		rules: make(map[uuid.UUID]*store.TrustRule),
		index: make(map[string]uuid.UUID),
	}
}

func (m *mockTrustRuleStore) List(_ context.Context, workspaceID uuid.UUID) ([]store.TrustRule, error) {
	var result []store.TrustRule
	for _, r := range m.rules {
		if r.WorkspaceID == workspaceID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockTrustRuleStore) Upsert(_ context.Context, rule *store.TrustRule) error {
	key := rule.WorkspaceID.String() + ":" + rule.ToolPattern
	if existingID, exists := m.index[key]; exists {
		// Update existing
		existing := m.rules[existingID]
		existing.Tier = rule.Tier
		existing.UpdatedAt = time.Now()
		rule.ID = existingID
		rule.CreatedAt = existing.CreatedAt
		rule.UpdatedAt = existing.UpdatedAt
		return nil
	}
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	m.rules[rule.ID] = rule
	m.index[key] = rule.ID
	return nil
}

func (m *mockTrustRuleStore) Delete(_ context.Context, id uuid.UUID) error {
	r, ok := m.rules[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	key := r.WorkspaceID.String() + ":" + r.ToolPattern
	delete(m.index, key)
	delete(m.rules, id)
	return nil
}

// --- Trust rules handler tests ---

func TestTrustRulesHandler_Create(t *testing.T) {
	wsID := uuid.New()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid trust rule",
			body: map[string]interface{}{
				"tool_pattern": "read_*",
				"tier":         "auto",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "invalid tier rejected",
			body: map[string]interface{}{
				"tool_pattern": "read_*",
				"tier":         "custom",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing tool_pattern rejected",
			body: map[string]interface{}{
				"tier": "auto",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing tier rejected",
			body: map[string]interface{}{
				"tool_pattern": "read_*",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleStore := newMockTrustRuleStore()
			audit := &mockAuditStoreForAPI{}
			h := NewTrustRulesHandler(ruleStore, audit, nil)

			req := adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trust-rules", tt.body)
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

func TestTrustRulesHandler_Upsert(t *testing.T) {
	wsID := uuid.New()
	ruleStore := newMockTrustRuleStore()
	audit := &mockAuditStoreForAPI{}
	h := NewTrustRulesHandler(ruleStore, audit, nil)

	body := map[string]interface{}{
		"tool_pattern": "read_*",
		"tier":         "auto",
	}

	// First create
	req := adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trust-rules", body)
	w := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wsID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w.Code)
	}

	// Upsert same pattern with different tier
	body["tier"] = "review"
	req = adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trust-rules", body)
	w = httptest.NewRecorder()
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wsID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("upsert: expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify only one rule exists for this workspace
	rules, _ := ruleStore.List(context.Background(), wsID)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after upsert, got %d", len(rules))
	}
	if rules[0].Tier != "review" {
		t.Fatalf("expected tier 'review' after upsert, got %s", rules[0].Tier)
	}
}

func TestTrustRulesHandler_List(t *testing.T) {
	wsID := uuid.New()
	otherWsID := uuid.New()
	ruleStore := newMockTrustRuleStore()
	audit := &mockAuditStoreForAPI{}
	h := NewTrustRulesHandler(ruleStore, audit, nil)

	// Add rules in different workspaces
	ruleStore.Upsert(context.Background(), &store.TrustRule{
		WorkspaceID: wsID,
		ToolPattern: "read_*",
		Tier:        "auto",
	})
	ruleStore.Upsert(context.Background(), &store.TrustRule{
		WorkspaceID: wsID,
		ToolPattern: "write_*",
		Tier:        "review",
	})
	ruleStore.Upsert(context.Background(), &store.TrustRule{
		WorkspaceID: otherWsID,
		ToolPattern: "delete_*",
		Tier:        "block",
	})

	req := adminRequest(http.MethodGet, "/api/v1/workspaces/"+wsID.String()+"/trust-rules", nil)
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
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules for workspace, got %d", len(rules))
	}
}

// --- Input validation gap tests ---
// These tests prove that the trust rules handler does NOT validate tool_pattern.

func TestTrustRulesHandler_Create_RejectsInvalidPattern(t *testing.T) {
	t.Parallel()

	wsID := uuid.New()

	invalidPatterns := []struct {
		name    string
		pattern string
	}{
		{"control characters", "read_\x00*"},
		{"shell injection", "read_*; rm -rf /"},
		{"path traversal", "../../../etc/passwd"},
		{"extremely long pattern", string(make([]byte, 1024))}, // 1KB pattern
		{"null bytes", "read\x00_*"},
		{"regex DoS pattern", "((((((((((((((((((((a*)*)*)*)*)*)*)*)*)*)*)*)*)*)*)*)*)*)*)*)*"},
	}

	for _, tt := range invalidPatterns {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ruleStore := newMockTrustRuleStore()
			audit := &mockAuditStoreForAPI{}
			h := NewTrustRulesHandler(ruleStore, audit, nil)

			body := map[string]interface{}{
				"tool_pattern": tt.pattern,
				"tier":         "auto",
			}
			req := adminRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/trust-rules", body)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid tool_pattern %q, got %d; body: %s",
					tt.name, w.Code, w.Body.String())
			}
		})
	}
}

func TestTrustRulesHandler_Delete(t *testing.T) {
	wsID := uuid.New()
	ruleStore := newMockTrustRuleStore()
	audit := &mockAuditStoreForAPI{}
	h := NewTrustRulesHandler(ruleStore, audit, nil)

	rule := &store.TrustRule{
		WorkspaceID: wsID,
		ToolPattern: "read_*",
		Tier:        "auto",
	}
	ruleStore.Upsert(context.Background(), rule)

	tests := []struct {
		name       string
		ruleID     string
		wantStatus int
	}{
		{
			name:       "delete existing",
			ruleID:     rule.ID.String(),
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete non-existent",
			ruleID:     uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			ruleID:     "bad-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodDelete, "/api/v1/workspaces/"+wsID.String()+"/trust-rules/"+tt.ruleID, nil)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workspaceId", wsID.String())
			rctx.URLParams.Add("ruleId", tt.ruleID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Delete(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}
