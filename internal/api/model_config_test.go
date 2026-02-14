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

// --- Mock model config store ---

type mockModelConfigStore struct {
	configs   map[string]*store.ModelConfig // keyed by "scope/scopeID"
	updateErr error
	upsertErr error
}

func newMockModelConfigStore() *mockModelConfigStore {
	return &mockModelConfigStore{
		configs: make(map[string]*store.ModelConfig),
	}
}

func (m *mockModelConfigStore) key(scope, scopeID string) string {
	return scope + "/" + scopeID
}

func (m *mockModelConfigStore) GetByScope(_ context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	c, ok := m.configs[m.key(scope, scopeID)]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	copy := *c
	return &copy, nil
}

func (m *mockModelConfigStore) GetMerged(_ context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	global, ok := m.configs[m.key("global", "")]
	if !ok {
		return nil, fmt.Errorf("global config not found")
	}
	result := *global

	if scope == "workspace" || scope == "user" {
		if ws, ok := m.configs[m.key("workspace", scopeID)]; ok {
			if ws.DefaultModel != "" {
				result.DefaultModel = ws.DefaultModel
			}
			if ws.Temperature != 0.0 {
				result.Temperature = ws.Temperature
			}
			if ws.MaxTokens != 0 {
				result.MaxTokens = ws.MaxTokens
			}
			if ws.MaxToolRounds != 0 {
				result.MaxToolRounds = ws.MaxToolRounds
			}
			if ws.DefaultContextWindow != 0 {
				result.DefaultContextWindow = ws.DefaultContextWindow
			}
			if ws.DefaultMaxOutputTokens != 0 {
				result.DefaultMaxOutputTokens = ws.DefaultMaxOutputTokens
			}
			if ws.HistoryTokenBudget != 0 {
				result.HistoryTokenBudget = ws.HistoryTokenBudget
			}
			if ws.MaxHistoryMessages != 0 {
				result.MaxHistoryMessages = ws.MaxHistoryMessages
			}
			if ws.EmbeddingModel != "" {
				result.EmbeddingModel = ws.EmbeddingModel
			}
		}
	}

	return &result, nil
}

func (m *mockModelConfigStore) Update(_ context.Context, config *store.ModelConfig, etag time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	k := m.key(config.Scope, config.ScopeID)
	existing, ok := m.configs[k]
	if !ok {
		return fmt.Errorf("not found")
	}
	if !existing.UpdatedAt.Equal(etag) {
		return fmt.Errorf("conflict")
	}
	config.UpdatedAt = time.Now()
	m.configs[k] = config
	return nil
}

func (m *mockModelConfigStore) Upsert(_ context.Context, config *store.ModelConfig) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	k := m.key(config.Scope, config.ScopeID)
	config.ID = uuid.New()
	config.UpdatedAt = time.Now()
	m.configs[k] = config
	return nil
}

// --- Model config handler tests ---

func TestModelConfigHandler_GetGlobal_Success(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ModelConfig{
		ID:                     uuid.New(),
		Scope:                  "global",
		ScopeID:                "",
		DefaultModel:           "qwen3:8b",
		Temperature:            0.7,
		MaxTokens:              8192,
		MaxToolRounds:          10,
		DefaultContextWindow:   128000,
		DefaultMaxOutputTokens: 8192,
		HistoryTokenBudget:     4000,
		MaxHistoryMessages:     20,
		EmbeddingModel:         "nomic-embed-text:latest",
		UpdatedAt:              updatedAt,
	}

	req := adminRequest(http.MethodGet, "/api/v1/model-config", nil)
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
	if data["default_model"] != "qwen3:8b" {
		t.Fatalf("expected default_model=qwen3:8b, got %v", data["default_model"])
	}
}

func TestModelConfigHandler_GetGlobal_NotFound(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	req := adminRequest(http.MethodGet, "/api/v1/model-config", nil)
	w := httptest.NewRecorder()

	h.GetGlobal(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestModelConfigHandler_UpdateGlobal_Success(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ModelConfig{
		ID:                     uuid.New(),
		Scope:                  "global",
		ScopeID:                "",
		DefaultModel:           "qwen3:8b",
		Temperature:            0.7,
		MaxTokens:              8192,
		MaxToolRounds:          10,
		DefaultContextWindow:   128000,
		DefaultMaxOutputTokens: 8192,
		HistoryTokenBudget:     4000,
		MaxHistoryMessages:     20,
		EmbeddingModel:         "nomic-embed-text:latest",
		UpdatedAt:              updatedAt,
	}

	req := adminRequest(http.MethodPut, "/api/v1/model-config", map[string]interface{}{
		"default_model": "llama3:70b",
		"temperature":   0.9,
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
	if data["default_model"] != "llama3:70b" {
		t.Fatalf("expected default_model=llama3:70b, got %v", data["default_model"])
	}

	if len(audit.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.entries))
	}
}

func TestModelConfigHandler_UpdateGlobal_MissingIfMatch(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	req := adminRequest(http.MethodPut, "/api/v1/model-config", map[string]interface{}{
		"default_model": "llama3:70b",
	})
	// No If-Match header
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestModelConfigHandler_UpdateGlobal_StaleEtag(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ModelConfig{
		ID:                     uuid.New(),
		Scope:                  "global",
		ScopeID:                "",
		DefaultModel:           "qwen3:8b",
		Temperature:            0.7,
		MaxTokens:              8192,
		MaxToolRounds:          10,
		DefaultContextWindow:   128000,
		DefaultMaxOutputTokens: 8192,
		HistoryTokenBudget:     4000,
		MaxHistoryMessages:     20,
		EmbeddingModel:         "nomic-embed-text:latest",
		UpdatedAt:              updatedAt,
	}

	// Use a stale etag
	staleEtag := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UTC().Format(time.RFC3339Nano)

	req := adminRequest(http.MethodPut, "/api/v1/model-config", map[string]interface{}{
		"default_model": "llama3:70b",
	})
	req.Header.Set("If-Match", staleEtag)
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestModelConfigHandler_UpdateGlobal_InvalidBody(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	configStore.configs["global/"] = &store.ModelConfig{
		ID:        uuid.New(),
		Scope:     "global",
		ScopeID:   "",
		UpdatedAt: updatedAt,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/model-config", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", updatedAt.UTC().Format(time.RFC3339Nano))

	// Inject auth context
	ctx := req.Context()
	ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
	uid := uuid.New()
	ctxWithAuth := context.WithValue(ctx, contextKeyForTest("user_id"), uid)
	_ = ctxWithAuth
	// Use adminRequest helper but override body to be nil (empty body triggers EOF decode error)
	req2 := adminRequest(http.MethodPut, "/api/v1/model-config", nil)
	req2.Header.Set("If-Match", updatedAt.UTC().Format(time.RFC3339Nano))

	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req2)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

type contextKeyForTest string

func TestModelConfigHandler_GetWorkspace_Merged(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	// Seed global config
	configStore.configs["global/"] = &store.ModelConfig{
		ID:                     uuid.New(),
		Scope:                  "global",
		ScopeID:                "",
		DefaultModel:           "qwen3:8b",
		Temperature:            0.7,
		MaxTokens:              8192,
		MaxToolRounds:          10,
		DefaultContextWindow:   128000,
		DefaultMaxOutputTokens: 8192,
		HistoryTokenBudget:     4000,
		MaxHistoryMessages:     20,
		EmbeddingModel:         "nomic-embed-text:latest",
		UpdatedAt:              time.Now(),
	}

	// Seed workspace override
	wsID := "ws-123"
	configStore.configs["workspace/"+wsID] = &store.ModelConfig{
		ID:           uuid.New(),
		Scope:        "workspace",
		ScopeID:      wsID,
		DefaultModel: "llama3:70b",
		Temperature:  0.5,
		UpdatedAt:    time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/workspaces/"+wsID+"/model-config", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wsID)
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
	// Workspace override should take effect
	if data["default_model"] != "llama3:70b" {
		t.Fatalf("expected default_model=llama3:70b, got %v", data["default_model"])
	}
	// Temperature should be workspace override
	if temp, ok := data["temperature"].(float64); !ok || temp != 0.5 {
		t.Fatalf("expected temperature=0.5, got %v", data["temperature"])
	}
	// MaxTokens should fall through from global
	if mt, ok := data["max_tokens"].(float64); !ok || int(mt) != 8192 {
		t.Fatalf("expected max_tokens=8192, got %v", data["max_tokens"])
	}
}

func TestModelConfigHandler_UpdateWorkspace_CreatesNew(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	wsID := "ws-456"

	req := adminRequest(http.MethodPut, "/api/v1/workspaces/"+wsID+"/model-config", map[string]interface{}{
		"default_model": "gpt-4",
		"temperature":   0.8,
	})
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wsID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.UpdateWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	// Verify config was stored
	stored, ok := configStore.configs["workspace/"+wsID]
	if !ok {
		t.Fatal("expected workspace config to be stored")
	}
	if stored.DefaultModel != "gpt-4" {
		t.Fatalf("expected default_model=gpt-4, got %s", stored.DefaultModel)
	}

	if len(audit.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.entries))
	}
}

func TestModelConfigHandler_UpdateWorkspace_UpdatesExisting(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	wsID := "ws-789"
	configStore.configs["workspace/"+wsID] = &store.ModelConfig{
		ID:           uuid.New(),
		Scope:        "workspace",
		ScopeID:      wsID,
		DefaultModel: "old-model",
		Temperature:  0.5,
		UpdatedAt:    time.Now(),
	}

	req := adminRequest(http.MethodPut, "/api/v1/workspaces/"+wsID+"/model-config", map[string]interface{}{
		"default_model": "new-model",
		"max_tokens":    16384,
	})
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wsID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.UpdateWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	stored := configStore.configs["workspace/"+wsID]
	if stored.DefaultModel != "new-model" {
		t.Fatalf("expected default_model=new-model, got %s", stored.DefaultModel)
	}
	if stored.MaxTokens != 16384 {
		t.Fatalf("expected max_tokens=16384, got %d", stored.MaxTokens)
	}
}

func TestModelConfigHandler_UpdateGlobal_InvalidIfMatch(t *testing.T) {
	configStore := newMockModelConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelConfigHandler(configStore, audit, nil)

	req := adminRequest(http.MethodPut, "/api/v1/model-config", map[string]interface{}{
		"default_model": "llama3:70b",
	})
	req.Header.Set("If-Match", "not-a-timestamp")
	w := httptest.NewRecorder()

	h.UpdateGlobal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}
