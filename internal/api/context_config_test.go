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

// --- Mock context config store ---

type mockContextConfigStore struct {
	configs   map[string]*store.ContextConfig // keyed by "scope/scopeID"
	updateErr error
	upsertErr error
}

func newMockContextConfigStore() *mockContextConfigStore {
	return &mockContextConfigStore{
		configs: make(map[string]*store.ContextConfig),
	}
}

func (m *mockContextConfigStore) key(scope, scopeID string) string {
	return scope + "/" + scopeID
}

func (m *mockContextConfigStore) GetByScope(_ context.Context, scope, scopeID string) (*store.ContextConfig, error) {
	c, ok := m.configs[m.key(scope, scopeID)]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	copy := *c
	return &copy, nil
}

func (m *mockContextConfigStore) GetMerged(_ context.Context, scope, scopeID string) (*store.ContextConfig, error) {
	global, ok := m.configs[m.key("global", "")]
	if !ok {
		return nil, fmt.Errorf("global config not found")
	}

	if scope == "global" {
		copy := *global
		return &copy, nil
	}

	ws, ok := m.configs[m.key(scope, scopeID)]
	if !ok {
		copy := *global
		return &copy, nil
	}

	merged := &store.ContextConfig{
		ID:             ws.ID,
		Scope:          ws.Scope,
		ScopeID:        ws.ScopeID,
		MaxTotalTokens: global.MaxTotalTokens,
		LayerBudgets:   global.LayerBudgets,
		EnabledLayers:  global.EnabledLayers,
		UpdatedAt:      ws.UpdatedAt,
	}

	if ws.MaxTotalTokens != 0 {
		merged.MaxTotalTokens = ws.MaxTotalTokens
	}
	if len(ws.LayerBudgets) > 0 && string(ws.LayerBudgets) != "null" {
		merged.LayerBudgets = ws.LayerBudgets
	}
	if len(ws.EnabledLayers) > 0 && string(ws.EnabledLayers) != "null" {
		merged.EnabledLayers = ws.EnabledLayers
	}

	return merged, nil
}

func (m *mockContextConfigStore) Update(_ context.Context, config *store.ContextConfig, etag time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.configs[m.key(config.Scope, config.ScopeID)]
	if !ok {
		return fmt.Errorf("not found")
	}
	if !existing.UpdatedAt.Equal(etag) {
		return fmt.Errorf("conflict")
	}
	config.UpdatedAt = time.Now()
	m.configs[m.key(config.Scope, config.ScopeID)] = config
	return nil
}

func (m *mockContextConfigStore) Upsert(_ context.Context, config *store.ContextConfig) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	config.ID = uuid.New()
	config.UpdatedAt = time.Now()
	m.configs[m.key(config.Scope, config.ScopeID)] = config
	return nil
}

// --- Context config handler tests ---

func TestContextConfigHandler_GetGlobal(t *testing.T) {
	configStore := newMockContextConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewContextConfigHandler(configStore, audit, nil)

	globalID := uuid.New()
	configStore.configs["global/"] = &store.ContextConfig{
		ID:             globalID,
		Scope:          "global",
		ScopeID:        "",
		MaxTotalTokens: 18000,
		LayerBudgets:   json.RawMessage(`{"workspace_structure":500,"file_content":8000}`),
		EnabledLayers:  json.RawMessage(`["workspace_structure","file_content"]`),
		UpdatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	req := adminRequest(http.MethodGet, "/api/v1/context-config", nil)
	w := httptest.NewRecorder()

	h.GetGlobal(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	if data["scope"] != "global" {
		t.Fatalf("expected scope=global, got %v", data["scope"])
	}
	if int(data["max_total_tokens"].(float64)) != 18000 {
		t.Fatalf("expected max_total_tokens=18000, got %v", data["max_total_tokens"])
	}
}

func TestContextConfigHandler_UpdateGlobal(t *testing.T) {
	configStore := newMockContextConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewContextConfigHandler(configStore, audit, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ContextConfig{
		ID:             uuid.New(),
		Scope:          "global",
		ScopeID:        "",
		MaxTotalTokens: 18000,
		LayerBudgets:   json.RawMessage(`{"workspace_structure":500}`),
		EnabledLayers:  json.RawMessage(`["workspace_structure"]`),
		UpdatedAt:      updatedAt,
	}

	req := adminRequest(http.MethodPut, "/api/v1/context-config", map[string]interface{}{
		"max_total_tokens": 20000,
		"layer_budgets":    map[string]int{"workspace_structure": 600, "file_content": 9000},
		"enabled_layers":   []string{"workspace_structure", "file_content"},
	})
	req.Header.Set("If-Match", updatedAt.UTC().Format(time.RFC3339Nano))
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	if int(data["max_total_tokens"].(float64)) != 20000 {
		t.Fatalf("expected max_total_tokens=20000, got %v", data["max_total_tokens"])
	}

	if len(audit.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.entries))
	}
	if audit.entries[0].Action != "context_config_update" {
		t.Fatalf("expected action=context_config_update, got %s", audit.entries[0].Action)
	}
}

func TestContextConfigHandler_UpdateGlobal_MissingIfMatch(t *testing.T) {
	configStore := newMockContextConfigStore()
	h := NewContextConfigHandler(configStore, nil, nil)

	req := adminRequest(http.MethodPut, "/api/v1/context-config", map[string]interface{}{
		"max_total_tokens": 20000,
	})
	// No If-Match header
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestContextConfigHandler_UpdateGlobal_StaleEtag(t *testing.T) {
	configStore := newMockContextConfigStore()
	h := NewContextConfigHandler(configStore, nil, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ContextConfig{
		ID:             uuid.New(),
		Scope:          "global",
		ScopeID:        "",
		MaxTotalTokens: 18000,
		LayerBudgets:   json.RawMessage(`{}`),
		EnabledLayers:  json.RawMessage(`[]`),
		UpdatedAt:      updatedAt,
	}

	staleEtag := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UTC().Format(time.RFC3339Nano)
	req := adminRequest(http.MethodPut, "/api/v1/context-config", map[string]interface{}{
		"max_total_tokens": 20000,
	})
	req.Header.Set("If-Match", staleEtag)
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestContextConfigHandler_UpdateGlobal_InvalidBody(t *testing.T) {
	configStore := newMockContextConfigStore()
	h := NewContextConfigHandler(configStore, nil, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ContextConfig{
		ID:             uuid.New(),
		Scope:          "global",
		ScopeID:        "",
		MaxTotalTokens: 18000,
		LayerBudgets:   json.RawMessage(`{}`),
		EnabledLayers:  json.RawMessage(`[]`),
		UpdatedAt:      updatedAt,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/context-config", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", updatedAt.UTC().Format(time.RFC3339Nano))
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestContextConfigHandler_GetWorkspace_Merged(t *testing.T) {
	configStore := newMockContextConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewContextConfigHandler(configStore, audit, nil)

	// Global config
	configStore.configs["global/"] = &store.ContextConfig{
		ID:             uuid.New(),
		Scope:          "global",
		ScopeID:        "",
		MaxTotalTokens: 18000,
		LayerBudgets:   json.RawMessage(`{"workspace_structure":500,"file_content":8000}`),
		EnabledLayers:  json.RawMessage(`["workspace_structure","file_content"]`),
		UpdatedAt:      time.Now(),
	}

	// Workspace override with custom tokens
	wsID := uuid.New()
	configStore.configs["workspace/ws-123"] = &store.ContextConfig{
		ID:             wsID,
		Scope:          "workspace",
		ScopeID:        "ws-123",
		MaxTotalTokens: 25000,
		UpdatedAt:      time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/workspaces/ws-123/context-config", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", "ws-123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.GetWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	// MaxTotalTokens should be overridden by workspace
	if int(data["max_total_tokens"].(float64)) != 25000 {
		t.Fatalf("expected merged max_total_tokens=25000, got %v", data["max_total_tokens"])
	}
	// LayerBudgets should come from global since workspace didn't override
	budgets := data["layer_budgets"].(map[string]interface{})
	if int(budgets["workspace_structure"].(float64)) != 500 {
		t.Fatalf("expected global layer_budgets preserved, got %v", budgets)
	}
}

func TestContextConfigHandler_UpdateWorkspace_Creates(t *testing.T) {
	configStore := newMockContextConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewContextConfigHandler(configStore, audit, nil)

	req := adminRequest(http.MethodPut, "/api/v1/workspaces/ws-new/context-config", map[string]interface{}{
		"max_total_tokens": 12000,
		"layer_budgets":    map[string]int{"file_content": 6000},
		"enabled_layers":   []string{"file_content"},
	})
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", "ws-new")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.UpdateWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	if data["scope"] != "workspace" {
		t.Fatalf("expected scope=workspace, got %v", data["scope"])
	}
	if data["scope_id"] != "ws-new" {
		t.Fatalf("expected scope_id=ws-new, got %v", data["scope_id"])
	}
	if int(data["max_total_tokens"].(float64)) != 12000 {
		t.Fatalf("expected max_total_tokens=12000, got %v", data["max_total_tokens"])
	}

	// Verify config was stored
	stored, ok := configStore.configs["workspace/ws-new"]
	if !ok {
		t.Fatal("expected workspace config to be stored")
	}
	if stored.MaxTotalTokens != 12000 {
		t.Fatalf("expected stored max_total_tokens=12000, got %d", stored.MaxTotalTokens)
	}

	if len(audit.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.entries))
	}
}

func TestContextConfigHandler_UpdateWorkspace_Updates(t *testing.T) {
	configStore := newMockContextConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewContextConfigHandler(configStore, audit, nil)

	// Pre-existing workspace config
	configStore.configs["workspace/ws-exist"] = &store.ContextConfig{
		ID:             uuid.New(),
		Scope:          "workspace",
		ScopeID:        "ws-exist",
		MaxTotalTokens: 10000,
		LayerBudgets:   json.RawMessage(`{"file_content":5000}`),
		EnabledLayers:  json.RawMessage(`["file_content"]`),
		UpdatedAt:      time.Now(),
	}

	req := adminRequest(http.MethodPut, "/api/v1/workspaces/ws-exist/context-config", map[string]interface{}{
		"max_total_tokens": 15000,
		"layer_budgets":    map[string]int{"file_content": 7000},
		"enabled_layers":   []string{"file_content", "workspace_structure"},
	})
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", "ws-exist")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.UpdateWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	if int(data["max_total_tokens"].(float64)) != 15000 {
		t.Fatalf("expected max_total_tokens=15000, got %v", data["max_total_tokens"])
	}

	// Verify stored value was updated via upsert
	stored := configStore.configs["workspace/ws-exist"]
	if stored.MaxTotalTokens != 15000 {
		t.Fatalf("expected stored max_total_tokens=15000, got %d", stored.MaxTotalTokens)
	}
}
