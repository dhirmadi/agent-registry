package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock model endpoint store ---

type mockModelEndpointStore struct {
	endpoints map[uuid.UUID]*store.ModelEndpoint
	slugs     map[string]uuid.UUID
	versions  map[uuid.UUID][]store.ModelEndpointVersion

	createErr        error
	updateErr        error
	createVersionErr error
}

func newMockModelEndpointStore() *mockModelEndpointStore {
	return &mockModelEndpointStore{
		endpoints: make(map[uuid.UUID]*store.ModelEndpoint),
		slugs:     make(map[string]uuid.UUID),
		versions:  make(map[uuid.UUID][]store.ModelEndpointVersion),
	}
}

func (m *mockModelEndpointStore) Create(_ context.Context, ep *store.ModelEndpoint, _ json.RawMessage, _ string) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.slugs[ep.Slug]; exists {
		return fmt.Errorf("CONFLICT: model endpoint '%s' already exists", ep.Slug)
	}
	if ep.ID == uuid.Nil {
		ep.ID = uuid.New()
	}
	ep.CreatedAt = time.Now()
	ep.UpdatedAt = time.Now()
	m.endpoints[ep.ID] = ep
	m.slugs[ep.Slug] = ep.ID
	return nil
}

func (m *mockModelEndpointStore) GetBySlug(_ context.Context, slug string) (*store.ModelEndpoint, error) {
	id, ok := m.slugs[slug]
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: model endpoint '%s' not found", slug)
	}
	return m.endpoints[id], nil
}

func (m *mockModelEndpointStore) List(_ context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error) {
	var all []store.ModelEndpoint
	for _, ep := range m.endpoints {
		if activeOnly && !ep.IsActive {
			continue
		}
		if workspaceID != nil {
			if ep.WorkspaceID == nil || *ep.WorkspaceID != *workspaceID {
				continue
			}
		}
		all = append(all, *ep)
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

func (m *mockModelEndpointStore) Update(_ context.Context, ep *store.ModelEndpoint, etag time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.endpoints[ep.ID]
	if !ok {
		return fmt.Errorf("NOT_FOUND: model endpoint not found")
	}
	if !existing.UpdatedAt.Equal(etag) {
		return fmt.Errorf("CONFLICT: resource was modified by another client")
	}
	if existingID, exists := m.slugs[ep.Slug]; exists && existingID != ep.ID {
		return fmt.Errorf("CONFLICT: slug '%s' already exists", ep.Slug)
	}
	delete(m.slugs, existing.Slug)
	ep.UpdatedAt = time.Now()
	m.endpoints[ep.ID] = ep
	m.slugs[ep.Slug] = ep.ID
	return nil
}

func (m *mockModelEndpointStore) Delete(_ context.Context, slug string) error {
	id, ok := m.slugs[slug]
	if !ok {
		return fmt.Errorf("NOT_FOUND: model endpoint not found")
	}
	ep := m.endpoints[id]
	ep.IsActive = false
	return nil
}

func (m *mockModelEndpointStore) CreateVersion(_ context.Context, endpointID uuid.UUID, config json.RawMessage, changeNote, createdBy string) (*store.ModelEndpointVersion, error) {
	if m.createVersionErr != nil {
		return nil, m.createVersionErr
	}
	if _, ok := m.endpoints[endpointID]; !ok {
		return nil, fmt.Errorf("NOT_FOUND: model endpoint not found")
	}

	maxVer := 0
	for _, ver := range m.versions[endpointID] {
		if ver.Version > maxVer {
			maxVer = ver.Version
		}
	}

	versions := m.versions[endpointID]
	for i := range versions {
		versions[i].IsActive = false
	}
	m.versions[endpointID] = versions

	v := &store.ModelEndpointVersion{
		ID:         uuid.New(),
		EndpointID: endpointID,
		Version:    maxVer + 1,
		Config:     config,
		IsActive:   true,
		ChangeNote: changeNote,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now(),
	}
	m.versions[endpointID] = append(m.versions[endpointID], *v)
	return v, nil
}

func (m *mockModelEndpointStore) ListVersions(_ context.Context, endpointID uuid.UUID, offset, limit int) ([]store.ModelEndpointVersion, int, error) {
	versions := m.versions[endpointID]
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

func (m *mockModelEndpointStore) GetVersion(_ context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error) {
	for _, v := range m.versions[endpointID] {
		if v.Version == version {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: version %d not found", version)
}

func (m *mockModelEndpointStore) ActivateVersion(_ context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error) {
	versions := m.versions[endpointID]
	var targetIdx int = -1
	for i := range versions {
		if versions[i].Version == version {
			targetIdx = i
		}
	}
	if targetIdx == -1 {
		return nil, fmt.Errorf("NOT_FOUND: version %d not found", version)
	}
	for i := range versions {
		versions[i].IsActive = false
	}
	versions[targetIdx].IsActive = true
	m.versions[endpointID] = versions
	return &versions[targetIdx], nil
}

func (m *mockModelEndpointStore) GetActiveVersion(_ context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error) {
	for _, v := range m.versions[endpointID] {
		if v.IsActive {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: no active version")
}

func (m *mockModelEndpointStore) CountAll(_ context.Context) (int, error) {
	return len(m.endpoints), nil
}

// --- Helper functions ---

func withSlugParam(req *http.Request, slug string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func withSlugAndVersionParams(req *http.Request, slug string, version string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	rctx.URLParams.Add("version", version)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// --- CRUD tests ---

func TestModelEndpointsHandler_Create(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]interface{}
		role       string
		storeErr   error
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid fixed-model endpoint",
			body: map[string]interface{}{
				"slug":           "openai-gpt4o-prod",
				"name":           "GPT-4o Production",
				"provider":       "openai",
				"endpoint_url":   "https://api.openai.com/v1",
				"is_fixed_model": true,
				"model_name":     "gpt-4o-2024-08-06",
			},
			role:       "editor",
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid flexible-model endpoint",
			body: map[string]interface{}{
				"slug":           "azure-east-flex",
				"name":           "Azure East Flexible",
				"provider":       "azure",
				"endpoint_url":   "https://myorg-east.openai.azure.com",
				"is_fixed_model": false,
				"model_name":     "gpt-4o",
				"allowed_models": []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"},
			},
			role:       "editor",
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid custom provider",
			body: map[string]interface{}{
				"slug":           "custom-local",
				"name":           "Custom Provider",
				"provider":       "custom",
				"endpoint_url":   "https://custom.example.com/v1",
				"is_fixed_model": true,
				"model_name":     "my-model",
			},
			role:       "editor",
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing slug",
			body: map[string]interface{}{
				"name":         "No Slug",
				"provider":     "openai",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing name",
			body: map[string]interface{}{
				"slug":         "no-name",
				"provider":     "openai",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing provider",
			body: map[string]interface{}{
				"slug":         "no-provider",
				"name":         "No Provider",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid provider",
			body: map[string]interface{}{
				"slug":         "bad-provider",
				"name":         "Bad Provider",
				"provider":     "deepseek",
				"endpoint_url": "https://api.example.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing endpoint_url",
			body: map[string]interface{}{
				"slug":     "no-url",
				"name":     "No URL",
				"provider": "openai",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid slug format - uppercase",
			body: map[string]interface{}{
				"slug":         "BadSlug",
				"name":         "Bad Slug",
				"provider":     "openai",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid slug format - spaces",
			body: map[string]interface{}{
				"slug":         "bad slug",
				"name":         "Bad Slug",
				"provider":     "openai",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid slug format - too short",
			body: map[string]interface{}{
				"slug":         "ab",
				"name":         "Short Slug",
				"provider":     "openai",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "editor",
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "duplicate slug returns 409",
			body: map[string]interface{}{
				"slug":           "dup-slug",
				"name":           "Duplicate",
				"provider":       "openai",
				"endpoint_url":   "https://api.openai.com/v1",
				"is_fixed_model": true,
				"model_name":     "gpt-4o",
			},
			role:       "editor",
			storeErr:   fmt.Errorf("CONFLICT: model endpoint 'dup-slug' already exists"),
			wantStatus: http.StatusConflict,
			wantErr:    true,
		},
		{
			name: "viewer cannot create",
			body: map[string]interface{}{
				"slug":         "viewer-test",
				"name":         "Viewer Test",
				"provider":     "openai",
				"endpoint_url": "https://api.openai.com/v1",
			},
			role:       "viewer",
			wantStatus: http.StatusForbidden,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epStore := newMockModelEndpointStore()
			epStore.createErr = tt.storeErr
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", tt.body, tt.role)
			w := httptest.NewRecorder()

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

func TestModelEndpointsHandler_Get(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID:          epID,
		Slug:        "openai-gpt4o",
		Name:        "GPT-4o",
		Provider:    "openai",
		EndpointURL: "https://api.openai.com/v1",
		ModelName:   "gpt-4o",
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	epStore.slugs["openai-gpt4o"] = epID

	tests := []struct {
		name       string
		slug       string
		wantStatus int
	}{
		{"existing endpoint", "openai-gpt4o", http.StatusOK},
		{"non-existent endpoint", "nonexistent", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/"+tt.slug, nil, "viewer")
			req = withSlugParam(req, tt.slug)
			w := httptest.NewRecorder()

			h.Get(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpointsHandler_List(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	for i := 0; i < 3; i++ {
		id := uuid.New()
		slug := fmt.Sprintf("endpoint-%d", i)
		ep := &store.ModelEndpoint{
			ID: id, Slug: slug, Name: fmt.Sprintf("Endpoint %d", i), Provider: "openai",
			EndpointURL: "https://api.openai.com/v1", ModelName: "gpt-4o",
			IsActive: i != 2, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		epStore.endpoints[id] = ep
		epStore.slugs[slug] = id
	}

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCount  int
	}{
		{"list active only (default)", "", http.StatusOK, 2},
		{"list all (active_only=false)", "?active_only=false", http.StatusOK, 3},
		{"list with limit=1", "?limit=1&active_only=false", http.StatusOK, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/model-endpoints"+tt.query, nil, "viewer")
			w := httptest.NewRecorder()

			h.List(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			env := parseEnvelope(t, w)
			if !env.Success {
				t.Fatal("expected success=true")
			}
			data := env.Data.(map[string]interface{})
			endpoints := data["model_endpoints"].([]interface{})
			if len(endpoints) != tt.wantCount {
				t.Fatalf("expected %d endpoints, got %d", tt.wantCount, len(endpoints))
			}
		})
	}
}

func TestModelEndpointsHandler_Update(t *testing.T) {
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		slug       string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name: "valid update with If-Match", slug: "openai-gpt4o",
			ifMatch: now.Format(time.RFC3339Nano),
			body:    map[string]interface{}{"name": "Updated GPT-4o", "is_active": true},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing If-Match header", slug: "openai-gpt4o",
			body: map[string]interface{}{"name": "Updated"}, wantStatus: http.StatusBadRequest,
		},
		{
			name: "non-existent endpoint", slug: "nonexistent",
			ifMatch: now.Format(time.RFC3339Nano),
			body:    map[string]interface{}{"name": "Updated"}, wantStatus: http.StatusNotFound,
		},
		{
			name: "conflict - stale etag", slug: "openai-gpt4o",
			ifMatch: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			body:    map[string]interface{}{"name": "Updated GPT-4o"}, wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			epID := uuid.New()
			epStore.endpoints[epID] = &store.ModelEndpoint{
				ID: epID, Slug: "openai-gpt4o", Name: "GPT-4o", Provider: "openai",
				EndpointURL: "https://api.openai.com/v1", ModelName: "gpt-4o",
				IsActive: true, CreatedAt: now, UpdatedAt: now,
			}
			epStore.slugs["openai-gpt4o"] = epID

			req := agentRequest(http.MethodPut, "/api/v1/model-endpoints/"+tt.slug, tt.body, "editor")
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			req = withSlugParam(req, tt.slug)
			w := httptest.NewRecorder()

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpointsHandler_Delete(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "to-delete", Name: "Delete Me", IsActive: true,
	}
	epStore.slugs["to-delete"] = epID

	tests := []struct {
		name       string
		slug       string
		wantStatus int
	}{
		{"soft delete existing endpoint", "to-delete", http.StatusNoContent},
		{"delete non-existent endpoint", "nonexistent", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodDelete, "/api/v1/model-endpoints/"+tt.slug, nil, "editor")
			req = withSlugParam(req, tt.slug)
			w := httptest.NewRecorder()

			h.Delete(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// --- Version tests ---

func TestModelEndpointsHandler_CreateVersion(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "openai-gpt4o", Name: "GPT-4o", Provider: "openai", IsActive: true,
	}
	epStore.slugs["openai-gpt4o"] = epID

	tests := []struct {
		name       string
		slug       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name: "valid version creation", slug: "openai-gpt4o",
			body: map[string]interface{}{
				"config":      map[string]interface{}{"temperature": 0.3, "max_tokens": 4096},
				"change_note": "Lowered temperature",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "version with full config", slug: "openai-gpt4o",
			body: map[string]interface{}{
				"config": map[string]interface{}{
					"temperature": 0.7, "max_tokens": 8192, "max_output_tokens": 4096,
					"context_window": 128000, "top_p": 0.95,
				},
				"change_note": "Full config",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing config", slug: "openai-gpt4o",
			body: map[string]interface{}{}, wantStatus: http.StatusBadRequest,
		},
		{
			name: "non-existent endpoint", slug: "nonexistent",
			body: map[string]interface{}{
				"config": map[string]interface{}{"temperature": 0.5},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/"+tt.slug+"/versions", tt.body, "editor")
			req = withSlugParam(req, tt.slug)
			w := httptest.NewRecorder()

			h.CreateVersion(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpointsHandler_CreateVersion_AutoIncrement(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "openai-gpt4o", Name: "GPT-4o", Provider: "openai", IsActive: true,
	}
	epStore.slugs["openai-gpt4o"] = epID

	for i := 1; i <= 3; i++ {
		body := map[string]interface{}{
			"config":      map[string]interface{}{"temperature": float64(i) * 0.1},
			"change_note": fmt.Sprintf("Version %d", i),
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/openai-gpt4o/versions", body, "editor")
		req = withSlugParam(req, "openai-gpt4o")
		w := httptest.NewRecorder()
		h.CreateVersion(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("version %d: expected 201, got %d; body: %s", i, w.Code, w.Body.String())
		}
	}

	versions := epStore.versions[epID]
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}

	activeCount := 0
	for _, v := range versions {
		if v.IsActive {
			activeCount++
			if v.Version != 3 {
				t.Fatalf("expected active version to be 3, got %d", v.Version)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active version, got %d", activeCount)
	}
}

func TestModelEndpointsHandler_ListVersions(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "openai-gpt4o", Name: "GPT-4o", IsActive: true,
	}
	epStore.slugs["openai-gpt4o"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{ID: uuid.New(), EndpointID: epID, Version: 1, Config: json.RawMessage(`{"temperature":0.7}`), IsActive: false, CreatedAt: time.Now()},
		{ID: uuid.New(), EndpointID: epID, Version: 2, Config: json.RawMessage(`{"temperature":0.3}`), IsActive: true, CreatedAt: time.Now()},
	}

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/openai-gpt4o/versions", nil, "viewer")
	req = withSlugParam(req, "openai-gpt4o")
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

func TestModelEndpointsHandler_GetVersion(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "openai-gpt4o", Name: "GPT-4o", IsActive: true,
	}
	epStore.slugs["openai-gpt4o"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{ID: uuid.New(), EndpointID: epID, Version: 1, Config: json.RawMessage(`{"temperature":0.7}`), IsActive: true, CreatedAt: time.Now()},
	}

	tests := []struct {
		name       string
		slug       string
		version    string
		wantStatus int
	}{
		{"existing version", "openai-gpt4o", "1", http.StatusOK},
		{"non-existent version", "openai-gpt4o", "999", http.StatusNotFound},
		{"invalid version number", "openai-gpt4o", "abc", http.StatusBadRequest},
		{"non-existent endpoint", "nonexistent", "1", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/"+tt.slug+"/versions/"+tt.version, nil, "viewer")
			req = withSlugAndVersionParams(req, tt.slug, tt.version)
			w := httptest.NewRecorder()

			h.GetVersion(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpointsHandler_ActivateVersion(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "openai-gpt4o", Name: "GPT-4o", IsActive: true,
	}
	epStore.slugs["openai-gpt4o"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{ID: uuid.New(), EndpointID: epID, Version: 1, Config: json.RawMessage(`{"temperature":0.7}`), IsActive: false, CreatedAt: time.Now()},
		{ID: uuid.New(), EndpointID: epID, Version: 2, Config: json.RawMessage(`{"temperature":0.3}`), IsActive: true, CreatedAt: time.Now()},
	}

	tests := []struct {
		name       string
		slug       string
		version    string
		wantStatus int
	}{
		{"activate existing version", "openai-gpt4o", "1", http.StatusOK},
		{"activate non-existent version", "openai-gpt4o", "999", http.StatusNotFound},
		{"invalid version number", "openai-gpt4o", "abc", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/"+tt.slug+"/versions/"+tt.version+"/activate", nil, "editor")
			req = withSlugAndVersionParams(req, tt.slug, tt.version)
			w := httptest.NewRecorder()

			h.ActivateVersion(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}

	// Verify activation swap
	versions := epStore.versions[epID]
	for _, v := range versions {
		if v.Version == 1 && !v.IsActive {
			t.Fatal("expected version 1 to be active after activation")
		}
		if v.Version == 2 && v.IsActive {
			t.Fatal("expected version 2 to be inactive after activating version 1")
		}
	}
}

// --- Security: SSRF validation ---

func TestModelEndpointsHandler_Create_RejectsInternalEndpoint(t *testing.T) {
	t.Parallel()

	ssrfEndpoints := []struct {
		name        string
		endpointURL string
	}{
		{"AWS metadata v1", "http://169.254.169.254/latest/meta-data/"},
		{"AWS metadata v2", "http://169.254.169.254/latest/api/token"},
		{"localhost", "http://localhost:8080/admin"},
		{"loopback IP", "http://127.0.0.1:9090/internal"},
		{"private 10.x", "http://10.0.0.1/secrets"},
		{"private 172.16.x", "http://172.16.0.1/admin"},
		{"private 192.168.x", "http://192.168.1.1/config"},
		{"IPv6 loopback", "http://[::1]:8080/admin"},
		{"link-local", "http://169.254.1.1/metadata"},
	}

	for _, tt := range ssrfEndpoints {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug":           "ssrf-" + strings.ReplaceAll(strings.ToLower(tt.name), " ", "-"),
				"name":           "SSRF Test",
				"provider":       "custom",
				"endpoint_url":   tt.endpointURL,
				"is_fixed_model": true,
				"model_name":     "test",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for SSRF endpoint %q, got %d; body: %s",
					tt.endpointURL, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpointsHandler_Create_RejectsInvalidScheme(t *testing.T) {
	t.Parallel()

	invalidSchemes := []struct {
		name        string
		endpointURL string
	}{
		{"file scheme", "file:///etc/passwd"},
		{"ftp scheme", "ftp://evil.com/payload"},
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:text/html,<script>alert(1)</script>"},
		{"no scheme", "example.com/api"},
	}

	for _, tt := range invalidSchemes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug":           "scheme-" + strings.ReplaceAll(strings.ToLower(tt.name), " ", "-"),
				"name":           "Scheme Test",
				"provider":       "openai",
				"endpoint_url":   tt.endpointURL,
				"is_fixed_model": true,
				"model_name":     "test",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid scheme %q, got %d; body: %s",
					tt.endpointURL, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpointsHandler_Update_ValidatesEndpoint(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	ssrfEndpoints := []struct {
		name        string
		endpointURL string
	}{
		{"AWS metadata on update", "http://169.254.169.254/latest/meta-data/"},
		{"localhost on update", "http://localhost:8080/admin"},
		{"file scheme on update", "file:///etc/passwd"},
		{"private IP on update", "http://10.0.0.1/internal"},
	}

	for _, tt := range ssrfEndpoints {
		t.Run(tt.name, func(t *testing.T) {
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			epID := uuid.New()
			epStore.endpoints[epID] = &store.ModelEndpoint{
				ID: epID, Slug: "ssrf-update-test", Name: "SSRF Update Test",
				Provider: "openai", EndpointURL: "https://api.openai.com/v1",
				IsActive: true, CreatedAt: now, UpdatedAt: now,
			}
			epStore.slugs["ssrf-update-test"] = epID

			body := map[string]interface{}{"endpoint_url": tt.endpointURL}
			req := agentRequest(http.MethodPut, "/api/v1/model-endpoints/ssrf-update-test", body, "editor")
			req.Header.Set("If-Match", now.UTC().Format(time.RFC3339Nano))
			req = withSlugParam(req, "ssrf-update-test")
			w := httptest.NewRecorder()

			h.Update(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for SSRF endpoint on update %q, got %d; body: %s",
					tt.endpointURL, w.Code, w.Body.String())
			}
		})
	}
}

// --- Security: Config size validation ---

func TestModelEndpointsHandler_CreateVersion_RejectsOversizedConfig(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "config-size-test", Name: "Config Size Test", Provider: "openai", IsActive: true,
	}
	epStore.slugs["config-size-test"] = epID

	largeData := make([]byte, 100*1024)
	for i := range largeData {
		largeData[i] = 'A'
	}
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"temperature": 0.5,
			"metadata":    map[string]string{"padding": string(largeData)},
		},
		"change_note": "Oversized config",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/config-size-test/versions", body, "editor")
	req = withSlugParam(req, "config-size-test")
	w := httptest.NewRecorder()

	h.CreateVersion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized config, got %d; body: %s", w.Code, w.Body.String())
	}
}

// --- Security: Headers redaction ---

func TestModelEndpointsHandler_GetVersion_RedactsHeaders(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "redact-test", Name: "Redact Test", IsActive: true,
	}
	epStore.slugs["redact-test"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{
			ID: uuid.New(), EndpointID: epID, Version: 1,
			Config: json.RawMessage(`{"temperature":0.7,"headers":{"Authorization":"Bearer sk-secret-key","X-Custom":"value"}}`),
			IsActive: true, CreatedAt: time.Now(),
		},
	}

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/redact-test/versions/1", nil, "viewer")
	req = withSlugAndVersionParams(req, "redact-test", "1")
	w := httptest.NewRecorder()

	h.GetVersion(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "sk-secret-key") {
		t.Fatal("response must NOT contain raw header secret values")
	}
	if !strings.Contains(body, "REDACTED") {
		t.Fatal("response should contain REDACTED placeholder for header values")
	}
}

func TestModelEndpointsHandler_ListVersions_RedactsHeaders(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "redact-list-test", Name: "Redact List Test", IsActive: true,
	}
	epStore.slugs["redact-list-test"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{
			ID: uuid.New(), EndpointID: epID, Version: 1,
			Config:   json.RawMessage(`{"headers":{"Authorization":"Bearer secret123"}}`),
			IsActive: true, CreatedAt: time.Now(),
		},
	}

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/redact-list-test/versions", nil, "viewer")
	req = withSlugParam(req, "redact-list-test")
	w := httptest.NewRecorder()

	h.ListVersions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "secret123") {
		t.Fatal("list response must NOT contain raw header secret values")
	}
}

// --- Provider validation ---

func TestModelEndpointsHandler_Create_ValidatesProvider(t *testing.T) {
	t.Parallel()

	valid := []string{"openai", "azure", "anthropic", "ollama", "custom"}
	invalid := []string{"deepseek", "", "OPENAI", "google", "huggingface", "'; DROP TABLE;--"}

	for _, p := range valid {
		t.Run("valid_"+p, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug": "provider-" + p, "name": "Provider " + p, "provider": p,
				"endpoint_url": "https://api.example.com/v1", "is_fixed_model": true, "model_name": "test-model",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201 for valid provider %q, got %d; body: %s", p, w.Code, w.Body.String())
			}
		})
	}

	for _, p := range invalid {
		t.Run("invalid_"+p, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug": "bad-provider-test", "name": "Bad Provider", "provider": p,
				"endpoint_url": "https://api.example.com/v1", "is_fixed_model": true, "model_name": "test-model",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid provider %q, got %d; body: %s", p, w.Code, w.Body.String())
			}
		})
	}
}

// --- Workspace scoping ---

func TestModelEndpointsHandler_Create_WorkspaceScoped(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	body := map[string]interface{}{
		"slug": "ws-scoped-ep", "name": "Workspace Scoped", "provider": "openai",
		"endpoint_url": "https://api.openai.com/v1", "model_name": "gpt-4o",
		"is_fixed_model": true, "workspace_id": "workspace-123",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	wsID, ok := data["workspace_id"].(string)
	if !ok || wsID != "workspace-123" {
		t.Fatalf("expected workspace_id='workspace-123', got %v", data["workspace_id"])
	}
}

func TestModelEndpointsHandler_List_FilterByWorkspace(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	wsA := "workspace-A"
	wsB := "workspace-B"
	entries := []struct {
		slug string
		wsID *string
	}{
		{"ws-a-1", &wsA}, {"ws-a-2", &wsA},
		{"ws-b-1", &wsB}, {"global-1", nil},
	}
	for _, e := range entries {
		id := uuid.New()
		epStore.endpoints[id] = &store.ModelEndpoint{
			ID: id, Slug: e.slug, Name: e.slug, Provider: "openai",
			EndpointURL: "https://api.openai.com/v1", WorkspaceID: e.wsID,
			IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		epStore.slugs[e.slug] = id
	}

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?workspace_id=workspace-A&active_only=false", nil, "viewer")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	endpoints := data["model_endpoints"].([]interface{})
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints for workspace-A, got %d", len(endpoints))
	}
}

// --- Duplicate slug ---

func TestModelEndpointsHandler_DuplicateSlug(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	body := map[string]interface{}{
		"slug": "dup-slug-test", "name": "Duplicate Slug", "provider": "openai",
		"endpoint_url": "https://api.openai.com/v1", "model_name": "gpt-4o",
		"is_fixed_model": true,
	}

	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w.Code)
	}

	req = agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w = httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate slug: expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

// --- Pagination upper bound ---

func TestModelEndpointsHandler_List_PaginationUpperBound(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	for i := 0; i < 5; i++ {
		id := uuid.New()
		slug := fmt.Sprintf("ep-%d", i)
		epStore.endpoints[id] = &store.ModelEndpoint{
			ID: id, Slug: slug, Name: fmt.Sprintf("EP %d", i), Provider: "openai",
			EndpointURL: "https://api.openai.com/v1", IsActive: true,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		epStore.slugs[slug] = id
	}

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?limit=500&active_only=false", nil, "viewer")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	endpoints := data["model_endpoints"].([]interface{})
	if len(endpoints) != 5 {
		t.Fatalf("expected 5 endpoints (capped at 200, only 5 exist), got %d", len(endpoints))
	}
}

// --- Role access ---

func TestModelEndpointsHandler_RoleAccess(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "role-test", Name: "Role Test", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["role-test"] = epID

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
			switch tt.method {
			case http.MethodGet:
				req := agentRequest(tt.method, "/api/v1/model-endpoints/role-test", nil, tt.role)
				req = withSlugParam(req, "role-test")
				mw := RequireRole("viewer", "editor", "admin")
				handler := mw(http.HandlerFunc(h.Get))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				if w.Code != tt.wantStatus {
					t.Fatalf("expected %d, got %d", tt.wantStatus, w.Code)
				}
			case http.MethodPost:
				body := map[string]interface{}{
					"slug": "new-ep", "name": "New Endpoint", "provider": "openai",
					"endpoint_url": "https://api.openai.com/v1", "model_name": "gpt-4o",
					"is_fixed_model": true,
				}
				req := agentRequest(tt.method, "/api/v1/model-endpoints", body, tt.role)
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

// --- Fixed vs Flexible model validation ---

func TestModelEndpointsHandler_Create_FixedModelRequiresModelName(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	body := map[string]interface{}{
		"slug": "fixed-no-model", "name": "Fixed No Model", "provider": "openai",
		"endpoint_url": "https://api.openai.com/v1", "is_fixed_model": true,
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for fixed model without model_name, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestModelEndpointsHandler_Create_FlexibleModelRequiresAllowedModels(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	body := map[string]interface{}{
		"slug": "flex-no-allowed", "name": "Flex No Allowed Models", "provider": "azure",
		"endpoint_url": "https://myorg.openai.azure.com",
		"is_fixed_model": false, "model_name": "gpt-4o",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for flexible model without allowed_models, got %d; body: %s", w.Code, w.Body.String())
	}
}

// --- Error response structure ---

func TestModelEndpointsHandler_Get_NotFoundErrorStructure(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/nonexistent", nil, "viewer")
	req = withSlugParam(req, "nonexistent")
	w := httptest.NewRecorder()

	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	if env.Success {
		t.Fatal("expected success=false")
	}
	if env.Error == nil {
		t.Fatal("expected error in response")
	}
}
