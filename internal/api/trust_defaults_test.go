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

// --- Mock trust default store ---

type mockTrustDefaultStore struct {
	defaults  map[uuid.UUID]*store.TrustDefault
	updateErr error
}

func newMockTrustDefaultStore() *mockTrustDefaultStore {
	return &mockTrustDefaultStore{
		defaults: make(map[uuid.UUID]*store.TrustDefault),
	}
}

func (m *mockTrustDefaultStore) List(_ context.Context) ([]store.TrustDefault, error) {
	var all []store.TrustDefault
	for _, d := range m.defaults {
		all = append(all, *d)
	}
	return all, nil
}

func (m *mockTrustDefaultStore) GetByID(_ context.Context, id uuid.UUID) (*store.TrustDefault, error) {
	d, ok := m.defaults[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	// Return a copy so the handler's modifications don't affect the stored object
	copy := *d
	return &copy, nil
}

func (m *mockTrustDefaultStore) Update(_ context.Context, d *store.TrustDefault) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.defaults[d.ID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if !existing.UpdatedAt.Equal(d.UpdatedAt) {
		return fmt.Errorf("conflict")
	}
	d.UpdatedAt = time.Now()
	m.defaults[d.ID] = d
	return nil
}

// --- Trust defaults handler tests ---

func TestTrustDefaultsHandler_List(t *testing.T) {
	defaultStore := newMockTrustDefaultStore()
	audit := &mockAuditStoreForAPI{}
	h := NewTrustDefaultsHandler(defaultStore, audit, nil)

	// Seed defaults
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	defaultStore.defaults[ids[0]] = &store.TrustDefault{
		ID:        ids[0],
		Tier:      "auto",
		Patterns:  json.RawMessage(`["_read", "_list"]`),
		Priority:  1,
		UpdatedAt: time.Now(),
	}
	defaultStore.defaults[ids[1]] = &store.TrustDefault{
		ID:        ids[1],
		Tier:      "block",
		Patterns:  json.RawMessage(`["_delete", "_send"]`),
		Priority:  2,
		UpdatedAt: time.Now(),
	}
	defaultStore.defaults[ids[2]] = &store.TrustDefault{
		ID:        ids[2],
		Tier:      "review",
		Patterns:  json.RawMessage(`["_write", "_create"]`),
		Priority:  3,
		UpdatedAt: time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/trust-defaults", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	data := env.Data.(map[string]interface{})
	defaults := data["defaults"].([]interface{})
	if len(defaults) != 3 {
		t.Fatalf("expected 3 defaults, got %d", len(defaults))
	}
}

func TestTrustDefaultsHandler_Update(t *testing.T) {
	defaultStore := newMockTrustDefaultStore()
	audit := &mockAuditStoreForAPI{}
	h := NewTrustDefaultsHandler(defaultStore, audit, nil)

	defaultID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	defaultStore.defaults[defaultID] = &store.TrustDefault{
		ID:        defaultID,
		Tier:      "auto",
		Patterns:  json.RawMessage(`["_read", "_list"]`),
		Priority:  1,
		UpdatedAt: updatedAt,
	}

	tests := []struct {
		name       string
		defaultID  string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:      "valid update",
			defaultID: defaultID.String(),
			ifMatch:   updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"patterns": []string{"_read", "_list", "_search", "_get"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing If-Match",
			defaultID:  defaultID.String(),
			ifMatch:    "",
			body:       map[string]interface{}{"patterns": []string{"_read"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-existent default",
			defaultID:  uuid.New().String(),
			ifMatch:    time.Now().UTC().Format(time.RFC3339Nano),
			body:       map[string]interface{}{"patterns": []string{"_read"}},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodPut, "/api/v1/trust-defaults/"+tt.defaultID, tt.body)
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("defaultId", tt.defaultID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestTrustDefaultsHandler_Update_ConflictOnStaleEtag(t *testing.T) {
	defaultStore := newMockTrustDefaultStore()
	audit := &mockAuditStoreForAPI{}
	h := NewTrustDefaultsHandler(defaultStore, audit, nil)

	defaultID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	defaultStore.defaults[defaultID] = &store.TrustDefault{
		ID:        defaultID,
		Tier:      "auto",
		Patterns:  json.RawMessage(`["_read"]`),
		Priority:  1,
		UpdatedAt: updatedAt,
	}

	// Use a stale etag
	staleEtag := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UTC().Format(time.RFC3339Nano)

	req := adminRequest(http.MethodPut, "/api/v1/trust-defaults/"+defaultID.String(), map[string]interface{}{
		"patterns": []string{"_read", "_list"},
	})
	req.Header.Set("If-Match", staleEtag)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("defaultId", defaultID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Update(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for stale etag, got %d; body: %s", w.Code, w.Body.String())
	}
}
