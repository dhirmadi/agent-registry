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

// --- Mock signal config store ---

type mockSignalConfigStore struct {
	configs   map[uuid.UUID]*store.SignalConfig
	updateErr error
}

func newMockSignalConfigStore() *mockSignalConfigStore {
	return &mockSignalConfigStore{
		configs: make(map[uuid.UUID]*store.SignalConfig),
	}
}

func (m *mockSignalConfigStore) List(_ context.Context) ([]store.SignalConfig, error) {
	var all []store.SignalConfig
	for _, c := range m.configs {
		all = append(all, *c)
	}
	return all, nil
}

func (m *mockSignalConfigStore) GetByID(_ context.Context, id uuid.UUID) (*store.SignalConfig, error) {
	c, ok := m.configs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	copy := *c
	return &copy, nil
}

func (m *mockSignalConfigStore) Update(_ context.Context, c *store.SignalConfig, etag time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.configs[c.ID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if !existing.UpdatedAt.Equal(etag) {
		return fmt.Errorf("conflict")
	}
	c.UpdatedAt = time.Now()
	m.configs[c.ID] = c
	return nil
}

// --- Signal config handler tests ---

func TestSignalConfigHandler_List(t *testing.T) {
	sigStore := newMockSignalConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewSignalConfigHandler(sigStore, audit, nil)

	// Seed configs
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	sigStore.configs[ids[0]] = &store.SignalConfig{
		ID:           ids[0],
		Source:       "gmail",
		PollInterval: "15m",
		IsEnabled:    true,
		UpdatedAt:    time.Now(),
	}
	sigStore.configs[ids[1]] = &store.SignalConfig{
		ID:           ids[1],
		Source:       "calendar",
		PollInterval: "1h",
		IsEnabled:    true,
		UpdatedAt:    time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/signal-config", nil)
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
	signals := data["signals"].([]interface{})
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}
}

func TestSignalConfigHandler_ListEmpty(t *testing.T) {
	sigStore := newMockSignalConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewSignalConfigHandler(sigStore, audit, nil)

	req := adminRequest(http.MethodGet, "/api/v1/signal-config", nil)
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
	signals := data["signals"].([]interface{})
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(signals))
	}
}

func TestSignalConfigHandler_Update(t *testing.T) {
	sigStore := newMockSignalConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewSignalConfigHandler(sigStore, audit, nil)

	signalID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	sigStore.configs[signalID] = &store.SignalConfig{
		ID:           signalID,
		Source:       "gmail",
		PollInterval: "15m",
		IsEnabled:    true,
		UpdatedAt:    updatedAt,
	}

	tests := []struct {
		name       string
		signalID   string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:     "valid update",
			signalID: signalID.String(),
			ifMatch:  updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"poll_interval": "10m",
				"is_enabled":    true,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "missing If-Match",
			signalID: signalID.String(),
			ifMatch:  "",
			body: map[string]interface{}{
				"poll_interval": "10m",
				"is_enabled":    true,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "invalid signal ID",
			signalID: "not-a-uuid",
			ifMatch:  updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"poll_interval": "10m",
				"is_enabled":    true,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "invalid poll_interval format",
			signalID: signalID.String(),
			ifMatch:  updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"poll_interval": "not-a-duration",
				"is_enabled":    true,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "poll_interval too short",
			signalID: signalID.String(),
			ifMatch:  updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"poll_interval": "500ms",
				"is_enabled":    true,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "non-existent signal",
			signalID: uuid.New().String(),
			ifMatch:  updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"poll_interval": "10m",
				"is_enabled":    true,
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodPut, "/api/v1/signal-config/"+tt.signalID, tt.body)
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("signalId", tt.signalID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestSignalConfigHandler_Update_ConflictOnStaleEtag(t *testing.T) {
	sigStore := newMockSignalConfigStore()
	audit := &mockAuditStoreForAPI{}
	h := NewSignalConfigHandler(sigStore, audit, nil)

	signalID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	sigStore.configs[signalID] = &store.SignalConfig{
		ID:           signalID,
		Source:       "gmail",
		PollInterval: "15m",
		IsEnabled:    true,
		UpdatedAt:    updatedAt,
	}

	// Use a stale etag
	staleEtag := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UTC().Format(time.RFC3339Nano)

	req := adminRequest(http.MethodPut, "/api/v1/signal-config/"+signalID.String(), map[string]interface{}{
		"poll_interval": "10m",
		"is_enabled":    true,
	})
	req.Header.Set("If-Match", staleEtag)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("signalId", signalID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.Update(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for stale etag, got %d; body: %s", w.Code, w.Body.String())
	}
}
