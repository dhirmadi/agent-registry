package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// VOLUME TESTS: Large-scale endpoint and version creation
// =============================================================================

func TestModelEndpoints_Volume_CreateAndPaginate1000(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create 1000 endpoints
	for i := 0; i < 1000; i++ {
		body := map[string]interface{}{
			"slug":           fmt.Sprintf("endpoint-%04d", i),
			"name":           fmt.Sprintf("Endpoint %d", i),
			"provider":       "openai",
			"endpoint_url":   "https://api.openai.com/v1",
			"is_fixed_model": true,
			"model_name":     "gpt-4o",
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
		w := httptest.NewRecorder()
		h.Create(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("endpoint %d: expected 201, got %d; body: %s", i, w.Code, w.Body.String())
		}
	}

	// Verify total count
	if len(epStore.endpoints) != 1000 {
		t.Fatalf("expected 1000 endpoints, got %d", len(epStore.endpoints))
	}

	// Test pagination: first page (default limit=50)
	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?active_only=false", nil, "viewer")
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	endpoints := data["model_endpoints"].([]interface{})
	total := int(data["total"].(float64))
	if total != 1000 {
		t.Fatalf("expected total=1000, got %d", total)
	}
	if len(endpoints) > 50 {
		t.Fatalf("default page should be <=50 items, got %d", len(endpoints))
	}

	// Test pagination: limit=200 (maximum allowed)
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints?limit=200&active_only=false", nil, "viewer")
	w = httptest.NewRecorder()
	h.List(w, req)

	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	endpoints = data["model_endpoints"].([]interface{})
	if len(endpoints) != 200 {
		t.Fatalf("expected 200 endpoints with limit=200, got %d", len(endpoints))
	}

	// Test pagination: offset=990, limit=50 (should get 10 items)
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints?offset=990&limit=50&active_only=false", nil, "viewer")
	w = httptest.NewRecorder()
	h.List(w, req)

	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	endpoints = data["model_endpoints"].([]interface{})
	if len(endpoints) != 10 {
		t.Fatalf("expected 10 endpoints at offset=990, got %d", len(endpoints))
	}

	// Test pagination: offset beyond total (should get 0 items)
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints?offset=1500&active_only=false", nil, "viewer")
	w = httptest.NewRecorder()
	h.List(w, req)

	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	if data["model_endpoints"] != nil {
		eps := data["model_endpoints"].([]interface{})
		if len(eps) != 0 {
			t.Fatalf("expected 0 endpoints at offset=1500, got %d", len(eps))
		}
	}

	// Test limit cap: requesting limit=500 should be capped to 200
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints?limit=500&active_only=false", nil, "viewer")
	w = httptest.NewRecorder()
	h.List(w, req)

	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	limit := int(data["limit"].(float64))
	if limit != 200 {
		t.Fatalf("expected limit capped at 200, got %d", limit)
	}
}

func TestModelEndpoints_Volume_100VersionsPerEndpoint(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create one endpoint
	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "versioned-ep", Name: "Versioned EP", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["versioned-ep"] = epID

	// Create 100 versions
	for i := 1; i <= 100; i++ {
		body := map[string]interface{}{
			"config":      map[string]interface{}{"temperature": float64(i) * 0.01},
			"change_note": fmt.Sprintf("Version %d config update", i),
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/versioned-ep/versions", body, "editor")
		req = withSlugParam(req, "versioned-ep")
		w := httptest.NewRecorder()
		h.CreateVersion(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("version %d: expected 201, got %d; body: %s", i, w.Code, w.Body.String())
		}
	}

	// Verify 100 versions exist
	if len(epStore.versions[epID]) != 100 {
		t.Fatalf("expected 100 versions, got %d", len(epStore.versions[epID]))
	}

	// Verify version auto-increment: each version should have unique ascending number
	seen := make(map[int]bool)
	for _, v := range epStore.versions[epID] {
		if seen[v.Version] {
			t.Fatalf("duplicate version number: %d", v.Version)
		}
		seen[v.Version] = true
	}
	for i := 1; i <= 100; i++ {
		if !seen[i] {
			t.Fatalf("missing version number: %d", i)
		}
	}

	// Verify exactly one active version (the last created)
	activeCount := 0
	var activeVersion int
	for _, v := range epStore.versions[epID] {
		if v.IsActive {
			activeCount++
			activeVersion = v.Version
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active version, got %d", activeCount)
	}
	if activeVersion != 100 {
		t.Fatalf("expected active version to be 100, got %d", activeVersion)
	}

	// Test activation of a middle version
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/versioned-ep/versions/50/activate", nil, "editor")
	req = withSlugAndVersionParams(req, "versioned-ep", "50")
	w := httptest.NewRecorder()
	h.ActivateVersion(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("activate version 50: expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify activation swap
	activeCount = 0
	for _, v := range epStore.versions[epID] {
		if v.IsActive {
			activeCount++
			if v.Version != 50 {
				t.Fatalf("expected version 50 to be active, got version %d", v.Version)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active version after activation, got %d", activeCount)
	}

	// Test listing versions with pagination
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints/versioned-ep/versions?limit=10&offset=0", nil, "viewer")
	req = withSlugParam(req, "versioned-ep")
	w = httptest.NewRecorder()
	h.ListVersions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list versions: expected 200, got %d", w.Code)
	}
	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	versions := data["versions"].([]interface{})
	totalVersions := int(data["total"].(float64))
	if totalVersions != 100 {
		t.Fatalf("expected total=100 versions, got %d", totalVersions)
	}
	if len(versions) != 10 {
		t.Fatalf("expected 10 versions in page, got %d", len(versions))
	}
}

func TestModelEndpoints_Volume_CountAll(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create 50 endpoints
	for i := 0; i < 50; i++ {
		body := map[string]interface{}{
			"slug":           fmt.Sprintf("count-ep-%03d", i),
			"name":           fmt.Sprintf("Count EP %d", i),
			"provider":       "openai",
			"endpoint_url":   "https://api.openai.com/v1",
			"is_fixed_model": true,
			"model_name":     "gpt-4o",
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
		w := httptest.NewRecorder()
		h.Create(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("endpoint %d: expected 201, got %d", i, w.Code)
		}
	}

	// Verify CountAll via the store directly
	count, err := epStore.CountAll(nil)
	if err != nil {
		t.Fatalf("CountAll error: %v", err)
	}
	if count != 50 {
		t.Fatalf("expected CountAll=50, got %d", count)
	}

	// Soft-delete 10 endpoints
	for i := 0; i < 10; i++ {
		slug := fmt.Sprintf("count-ep-%03d", i)
		req := agentRequest(http.MethodDelete, "/api/v1/model-endpoints/"+slug, nil, "editor")
		req = withSlugParam(req, slug)
		w := httptest.NewRecorder()
		h.Delete(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("delete %s: expected 204, got %d", slug, w.Code)
		}
	}

	// CountAll should still return 50 (soft-delete doesn't remove from count)
	count, err = epStore.CountAll(nil)
	if err != nil {
		t.Fatalf("CountAll error after deletes: %v", err)
	}
	if count != 50 {
		t.Fatalf("expected CountAll=50 after soft-delete, got %d", count)
	}

	// But list with active_only=true should show 40
	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?limit=200", nil, "viewer")
	w := httptest.NewRecorder()
	h.List(w, req)
	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	total := int(data["total"].(float64))
	if total != 40 {
		t.Fatalf("expected 40 active endpoints, got %d", total)
	}
}

// =============================================================================
// CONCURRENCY TESTS: Race conditions, auto-increment uniqueness, optimistic locking
// =============================================================================

func TestModelEndpoints_Concurrent_CreateVersionAutoIncrement(t *testing.T) {
	t.Parallel()

	// Use a thread-safe mock store for concurrency testing.
	// Audit store is nil to avoid race on the non-thread-safe mock.
	epStore := newConcurrentMockModelEndpointStore()
	h := NewModelEndpointsHandler(epStore, nil, nil, nil)

	epID := uuid.New()
	epStore.mu.Lock()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "concurrent-versions", Name: "Concurrent Versions", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["concurrent-versions"] = epID
	epStore.mu.Unlock()

	const numWorkers = 20
	var wg sync.WaitGroup
	var failCount int32
	var successCount int32

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := map[string]interface{}{
				"config":      map[string]interface{}{"temperature": float64(idx) * 0.05},
				"change_note": fmt.Sprintf("Concurrent version %d", idx),
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/concurrent-versions/versions", body, "editor")
			req = withSlugParam(req, "concurrent-versions")
			w := httptest.NewRecorder()
			h.CreateVersion(w, req)

			if w.Code == http.StatusCreated {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// All should succeed (mock is serialized by mutex)
	if successCount != numWorkers {
		t.Fatalf("expected %d successful creates, got %d (failures: %d)", numWorkers, successCount, failCount)
	}

	// Verify all versions have unique numbers
	epStore.mu.Lock()
	versions := epStore.versions[epID]
	epStore.mu.Unlock()

	seen := make(map[int]bool)
	for _, v := range versions {
		if seen[v.Version] {
			t.Fatalf("RACE CONDITION: duplicate version number %d detected", v.Version)
		}
		seen[v.Version] = true
	}

	if len(versions) != numWorkers {
		t.Fatalf("expected %d versions, got %d", numWorkers, len(versions))
	}
}

func TestModelEndpoints_Concurrent_ActivateVersionSingleActive(t *testing.T) {
	t.Parallel()

	epStore := newConcurrentMockModelEndpointStore()
	h := NewModelEndpointsHandler(epStore, nil, nil, nil)

	epID := uuid.New()
	epStore.mu.Lock()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "concurrent-activate", Name: "Concurrent Activate", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["concurrent-activate"] = epID

	// Pre-create 10 versions
	for i := 1; i <= 10; i++ {
		v := store.ModelEndpointVersion{
			ID: uuid.New(), EndpointID: epID, Version: i,
			Config: json.RawMessage(fmt.Sprintf(`{"temperature":%.1f}`, float64(i)*0.1)),
			IsActive: i == 1, CreatedAt: time.Now(),
		}
		epStore.versions[epID] = append(epStore.versions[epID], v)
	}
	epStore.mu.Unlock()

	// Concurrently activate different versions
	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(ver int) {
			defer wg.Done()
			req := agentRequest(http.MethodPost,
				fmt.Sprintf("/api/v1/model-endpoints/concurrent-activate/versions/%d/activate", ver), nil, "editor")
			req = withSlugAndVersionParams(req, "concurrent-activate", fmt.Sprintf("%d", ver))
			w := httptest.NewRecorder()
			h.ActivateVersion(w, req)
		}(i)
	}

	wg.Wait()

	// Verify exactly one active version (last writer wins)
	epStore.mu.Lock()
	activeCount := 0
	for _, v := range epStore.versions[epID] {
		if v.IsActive {
			activeCount++
		}
	}
	epStore.mu.Unlock()

	if activeCount != 1 {
		t.Fatalf("INVARIANT VIOLATION: expected exactly 1 active version, got %d", activeCount)
	}
}

func TestModelEndpoints_Concurrent_UpdateOptimisticConcurrency(t *testing.T) {
	t.Parallel()

	epStore := newConcurrentMockModelEndpointStore()
	h := NewModelEndpointsHandler(epStore, nil, nil, nil)

	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	epID := uuid.New()
	epStore.mu.Lock()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "concurrent-update", Name: "Concurrent Update", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: now, UpdatedAt: now,
	}
	epStore.slugs["concurrent-update"] = epID
	epStore.mu.Unlock()

	const numWorkers = 10
	var wg sync.WaitGroup
	var successCount int32
	var conflictCount int32

	// All workers use the same If-Match (original updated_at)
	// Only the first one should succeed; rest should get 409 Conflict
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := map[string]interface{}{
				"name": fmt.Sprintf("Updated by worker %d", idx),
			}
			req := agentRequest(http.MethodPut, "/api/v1/model-endpoints/concurrent-update", body, "editor")
			req.Header.Set("If-Match", now.UTC().Format(time.RFC3339Nano))
			req = withSlugParam(req, "concurrent-update")
			w := httptest.NewRecorder()
			h.Update(w, req)

			switch w.Code {
			case http.StatusOK:
				atomic.AddInt32(&successCount, 1)
			case http.StatusConflict:
				atomic.AddInt32(&conflictCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// Exactly 1 should succeed, rest should conflict
	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful update, got %d (conflicts: %d)", successCount, conflictCount)
	}
	if conflictCount != numWorkers-1 {
		t.Fatalf("expected %d conflicts, got %d", numWorkers-1, conflictCount)
	}
}

// =============================================================================
// STATE TRANSITION TESTS: Lifecycle flows and cascades
// =============================================================================

func TestModelEndpoints_Lifecycle_CreateVersionActivateMultipleGenerations(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Step 1: Create endpoint
	body := map[string]interface{}{
		"slug":           "lifecycle-ep",
		"name":           "Lifecycle Endpoint",
		"provider":       "anthropic",
		"endpoint_url":   "https://api.anthropic.com/v1",
		"is_fixed_model": true,
		"model_name":     "claude-3-5-sonnet",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	// Step 2: Create version 1 (with config)
	vBody := map[string]interface{}{
		"config":      map[string]interface{}{"temperature": 0.7, "max_tokens": 4096},
		"change_note": "Initial config",
	}
	req = agentRequest(http.MethodPost, "/api/v1/model-endpoints/lifecycle-ep/versions", vBody, "editor")
	req = withSlugParam(req, "lifecycle-ep")
	w = httptest.NewRecorder()
	h.CreateVersion(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("version 1: expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
	env := parseEnvelope(t, w)
	v1Data := env.Data.(map[string]interface{})
	if int(v1Data["version"].(float64)) != 1 {
		t.Fatalf("expected version=1, got %v", v1Data["version"])
	}

	// Step 3: Create version 2
	vBody = map[string]interface{}{
		"config":      map[string]interface{}{"temperature": 0.3, "max_tokens": 8192},
		"change_note": "Reduced temperature, increased tokens",
	}
	req = agentRequest(http.MethodPost, "/api/v1/model-endpoints/lifecycle-ep/versions", vBody, "editor")
	req = withSlugParam(req, "lifecycle-ep")
	w = httptest.NewRecorder()
	h.CreateVersion(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("version 2: expected 201, got %d", w.Code)
	}

	// Step 4: Activate version 1 (rollback scenario)
	req = agentRequest(http.MethodPost, "/api/v1/model-endpoints/lifecycle-ep/versions/1/activate", nil, "editor")
	req = withSlugAndVersionParams(req, "lifecycle-ep", "1")
	w = httptest.NewRecorder()
	h.ActivateVersion(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("activate v1: expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify v1 is active, v2 is not
	epID := epStore.slugs["lifecycle-ep"]
	activeCount := 0
	for _, v := range epStore.versions[epID] {
		if v.IsActive {
			activeCount++
			if v.Version != 1 {
				t.Fatalf("expected version 1 to be active, got %d", v.Version)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 active version, got %d", activeCount)
	}

	// Step 5: Create version 3
	vBody = map[string]interface{}{
		"config":      map[string]interface{}{"temperature": 0.5, "top_p": 0.9},
		"change_note": "New generation config",
	}
	req = agentRequest(http.MethodPost, "/api/v1/model-endpoints/lifecycle-ep/versions", vBody, "editor")
	req = withSlugParam(req, "lifecycle-ep")
	w = httptest.NewRecorder()
	h.CreateVersion(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("version 3: expected 201, got %d", w.Code)
	}

	// Step 6: Activate version 3
	req = agentRequest(http.MethodPost, "/api/v1/model-endpoints/lifecycle-ep/versions/3/activate", nil, "editor")
	req = withSlugAndVersionParams(req, "lifecycle-ep", "3")
	w = httptest.NewRecorder()
	h.ActivateVersion(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("activate v3: expected 200, got %d", w.Code)
	}

	// Verify final state: 3 versions, only v3 active
	if len(epStore.versions[epID]) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(epStore.versions[epID]))
	}
	activeCount = 0
	for _, v := range epStore.versions[epID] {
		if v.IsActive {
			activeCount++
			if v.Version != 3 {
				t.Fatalf("expected version 3 to be active, got %d", v.Version)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 active version, got %d", activeCount)
	}
}

func TestModelEndpoints_SoftDelete_GetBySlugReturnsNull(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create endpoint
	body := map[string]interface{}{
		"slug":           "delete-test-ep",
		"name":           "Delete Test EP",
		"provider":       "openai",
		"endpoint_url":   "https://api.openai.com/v1",
		"is_fixed_model": true,
		"model_name":     "gpt-4o",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// Verify it exists
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints/delete-test-ep", nil, "viewer")
	req = withSlugParam(req, "delete-test-ep")
	w = httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get before delete: expected 200, got %d", w.Code)
	}

	// Soft-delete
	req = agentRequest(http.MethodDelete, "/api/v1/model-endpoints/delete-test-ep", nil, "editor")
	req = withSlugParam(req, "delete-test-ep")
	w = httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	// GET should still work (soft-delete doesn't remove the record)
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints/delete-test-ep", nil, "viewer")
	req = withSlugParam(req, "delete-test-ep")
	w = httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get after soft-delete: expected 200, got %d", w.Code)
	}

	// But the is_active flag should be false
	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	isActive, ok := data["is_active"].(bool)
	if !ok || isActive {
		t.Fatal("expected is_active=false after soft-delete")
	}

	// List with active_only=true should exclude it
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints", nil, "viewer")
	w = httptest.NewRecorder()
	h.List(w, req)
	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	total := int(data["total"].(float64))
	if total != 0 {
		t.Fatalf("expected 0 active endpoints after delete, got %d", total)
	}

	// List with active_only=false should still include it
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints?active_only=false", nil, "viewer")
	w = httptest.NewRecorder()
	h.List(w, req)
	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	total = int(data["total"].(float64))
	if total != 1 {
		t.Fatalf("expected 1 endpoint with active_only=false, got %d", total)
	}
}

func TestModelEndpoints_DeleteEndpoint_CascadeVersions(t *testing.T) {
	// In the real DB, ON DELETE CASCADE removes versions when endpoint is deleted.
	// With soft-delete, versions should remain accessible.
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create endpoint with versions
	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "cascade-test", Name: "Cascade Test", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["cascade-test"] = epID

	// Create versions
	for i := 1; i <= 5; i++ {
		body := map[string]interface{}{
			"config":      map[string]interface{}{"temperature": float64(i) * 0.1},
			"change_note": fmt.Sprintf("Version %d", i),
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/cascade-test/versions", body, "editor")
		req = withSlugParam(req, "cascade-test")
		w := httptest.NewRecorder()
		h.CreateVersion(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("version %d: expected 201, got %d", i, w.Code)
		}
	}

	// Soft-delete the endpoint
	req := agentRequest(http.MethodDelete, "/api/v1/model-endpoints/cascade-test", nil, "editor")
	req = withSlugParam(req, "cascade-test")
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	// Versions should still be in the store (soft-delete only affects is_active)
	if len(epStore.versions[epID]) != 5 {
		t.Fatalf("expected 5 versions to remain after soft-delete, got %d", len(epStore.versions[epID]))
	}

	// Versions should still be listable
	req = agentRequest(http.MethodGet, "/api/v1/model-endpoints/cascade-test/versions", nil, "viewer")
	req = withSlugParam(req, "cascade-test")
	w = httptest.NewRecorder()
	h.ListVersions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list versions after delete: expected 200, got %d", w.Code)
	}
}

// =============================================================================
// FRONTEND INTEGRATION: API/TypeScript type alignment
// =============================================================================

func TestModelEndpoints_APIResponse_MatchesTypeScript(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create a fully populated endpoint
	body := map[string]interface{}{
		"slug":           "type-check-ep",
		"name":           "Type Check Endpoint",
		"provider":       "anthropic",
		"endpoint_url":   "https://api.anthropic.com/v1",
		"is_fixed_model": true,
		"model_name":     "claude-3-5-sonnet",
		"workspace_id":   "workspace-test",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// Verify all TypeScript ModelEndpoint fields are present
	requiredFields := []string{
		"id", "slug", "name", "provider", "endpoint_url",
		"is_fixed_model", "model_name", "allowed_models",
		"is_active", "created_by", "created_at", "updated_at",
	}
	for _, field := range requiredFields {
		if _, ok := data[field]; !ok {
			t.Errorf("MISSING FIELD: API response missing '%s' (required by TypeScript ModelEndpoint type)", field)
		}
	}

	// Verify field types match TypeScript expectations
	if _, ok := data["id"].(string); !ok {
		t.Error("id should be a string (UUID)")
	}
	if _, ok := data["slug"].(string); !ok {
		t.Error("slug should be a string")
	}
	if _, ok := data["provider"].(string); !ok {
		t.Error("provider should be a string")
	}
	if _, ok := data["is_fixed_model"].(bool); !ok {
		t.Error("is_fixed_model should be a boolean")
	}
	if _, ok := data["is_active"].(bool); !ok {
		t.Error("is_active should be a boolean")
	}

	// Verify provider is one of the valid enum values
	provider := data["provider"].(string)
	validProviders := map[string]bool{"openai": true, "azure": true, "anthropic": true, "ollama": true, "custom": true}
	if !validProviders[provider] {
		t.Errorf("provider '%s' not in TypeScript ModelProvider union type", provider)
	}

	// workspace_id should be present (nullable in TS)
	if data["workspace_id"] == nil {
		t.Error("workspace_id should be present (even if null)")
	}
}

func TestModelEndpoints_VersionResponse_MatchesTypeScript(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "version-type-test", Name: "Version Type Test", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["version-type-test"] = epID

	// Create a version with a full config
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"temperature":       0.7,
			"max_tokens":        4096,
			"max_output_tokens": 2048,
			"top_p":             0.9,
			"context_window":    128000,
		},
		"change_note": "Full config for type check",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/version-type-test/versions", body, "editor")
	req = withSlugParam(req, "version-type-test")
	w := httptest.NewRecorder()
	h.CreateVersion(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create version: expected 201, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// Verify all TypeScript ModelEndpointVersion fields
	requiredFields := []string{
		"id", "endpoint_id", "version", "config", "is_active",
		"change_note", "created_by", "created_at",
	}
	for _, field := range requiredFields {
		if _, ok := data[field]; !ok {
			t.Errorf("MISSING FIELD: version response missing '%s' (required by TypeScript ModelEndpointVersion type)", field)
		}
	}

	// Verify version is a number
	if _, ok := data["version"].(float64); !ok {
		t.Error("version should be a number")
	}

	// Verify config is an object
	config, ok := data["config"].(map[string]interface{})
	if !ok {
		t.Error("config should be a JSON object")
	} else {
		// Verify config fields match ModelEndpointConfig type
		if temp, ok := config["temperature"].(float64); !ok || temp != 0.7 {
			t.Errorf("config.temperature should be 0.7, got %v", config["temperature"])
		}
	}
}

func TestModelEndpoints_ListResponse_MatchesTypeScript(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create some endpoints
	for i := 0; i < 3; i++ {
		id := uuid.New()
		slug := fmt.Sprintf("list-type-%d", i)
		epStore.endpoints[id] = &store.ModelEndpoint{
			ID: id, Slug: slug, Name: fmt.Sprintf("EP %d", i), Provider: "openai",
			EndpointURL: "https://api.openai.com/v1", IsActive: true,
			AllowedModels: json.RawMessage(`[]`),
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		epStore.slugs[slug] = id
	}

	req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?active_only=false", nil, "viewer")
	w := httptest.NewRecorder()
	h.List(w, req)

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// List response should have model_endpoints, total, offset, limit
	if _, ok := data["model_endpoints"]; !ok {
		t.Error("list response missing 'model_endpoints' key")
	}
	if _, ok := data["total"]; !ok {
		t.Error("list response missing 'total' key")
	}
	if _, ok := data["offset"]; !ok {
		t.Error("list response missing 'offset' key")
	}
	if _, ok := data["limit"]; !ok {
		t.Error("list response missing 'limit' key")
	}

	// The key is "model_endpoints" not "endpoints"
	endpoints := data["model_endpoints"].([]interface{})
	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}
}

func TestModelEndpoints_Discovery_IncludesModelEndpoints(t *testing.T) {
	t.Parallel()

	// Use discovery-specific mocks that handle empty/nil states gracefully.
	// The standard mockModelConfigStore returns an error for missing keys,
	// but the discovery handler treats any error as fatal (HTTP 500).
	agentStore := &mockDiscoveryAgentStore{agents: []store.Agent{}}
	mcpStore := &mockDiscoveryMCPStore{servers: []store.MCPServer{}}
	trustStore := &mockDiscoveryTrustStore{defaults: []store.TrustDefault{}}
	modelConfigStore := &mockDiscoveryModelConfigStore{config: nil}
	epStore := newMockModelEndpointStore()

	// Add a model endpoint
	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "discovery-test", Name: "Discovery Test", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", ModelName: "gpt-4o",
		IsActive: true, AllowedModels: json.RawMessage(`[]`),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["discovery-test"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{
			ID: uuid.New(), EndpointID: epID, Version: 1,
			Config: json.RawMessage(`{"temperature":0.7}`),
			IsActive: true, CreatedAt: time.Now(), CreatedBy: "system",
		},
	}

	h := NewDiscoveryHandler(agentStore, mcpStore, trustStore, modelConfigStore, epStore)

	req := agentRequest(http.MethodGet, "/api/v1/discovery", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetDiscovery(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("discovery: expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// Verify model_endpoints key exists in discovery response
	modelEndpoints, ok := data["model_endpoints"]
	if !ok {
		t.Fatal("MISSING: discovery response must include 'model_endpoints' key")
	}

	eps := modelEndpoints.([]interface{})
	if len(eps) != 1 {
		t.Fatalf("expected 1 model endpoint in discovery, got %d", len(eps))
	}

	// Verify the endpoint data
	ep := eps[0].(map[string]interface{})
	if ep["slug"] != "discovery-test" {
		t.Errorf("expected slug='discovery-test', got %v", ep["slug"])
	}
	if ep["name"] != "Discovery Test" {
		t.Errorf("expected name='Discovery Test', got %v", ep["name"])
	}

	// Verify active version info is included
	if _, ok := ep["active_version"]; !ok {
		t.Error("discovery endpoint should include 'active_version' field")
	}
	if _, ok := ep["config"]; !ok {
		t.Error("discovery endpoint should include 'config' field from active version")
	}

	// Verify other expected discovery keys
	for _, key := range []string{"agents", "mcp_servers", "trust_defaults", "model_config", "fetched_at"} {
		if _, ok := data[key]; !ok {
			t.Errorf("discovery response missing '%s' key", key)
		}
	}
}

func TestModelEndpoints_Discovery_EmptyModelEndpoints(t *testing.T) {
	t.Parallel()

	agentStore := &mockDiscoveryAgentStore{agents: []store.Agent{}}
	mcpStore := &mockDiscoveryMCPStore{servers: []store.MCPServer{}}
	trustStore := &mockDiscoveryTrustStore{defaults: []store.TrustDefault{}}
	modelConfigStore := &mockDiscoveryModelConfigStore{config: nil}
	epStore := newMockModelEndpointStore()

	h := NewDiscoveryHandler(agentStore, mcpStore, trustStore, modelConfigStore, epStore)

	req := agentRequest(http.MethodGet, "/api/v1/discovery", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetDiscovery(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("discovery: expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// model_endpoints should be an empty array, not null
	modelEndpoints, ok := data["model_endpoints"]
	if !ok {
		t.Fatal("discovery response must include 'model_endpoints' key even when empty")
	}
	eps := modelEndpoints.([]interface{})
	if len(eps) != 0 {
		t.Fatalf("expected 0 model endpoints, got %d", len(eps))
	}
}

func TestModelEndpoints_DashboardCounts_Update(t *testing.T) {
	// The dashboard fetches /api/v1/discovery and shows counts.
	// Verify counts reflect created/deleted endpoints.
	t.Parallel()

	agentStore := &mockDiscoveryAgentStore{agents: []store.Agent{}}
	mcpStore := &mockDiscoveryMCPStore{servers: []store.MCPServer{}}
	trustStore := &mockDiscoveryTrustStore{defaults: []store.TrustDefault{}}
	modelConfigStore := &mockDiscoveryModelConfigStore{config: nil}
	epStore := newMockModelEndpointStore()

	discoveryH := NewDiscoveryHandler(agentStore, mcpStore, trustStore, modelConfigStore, epStore)
	epH := NewModelEndpointsHandler(epStore, &mockAuditStoreForAPI{}, nil, nil)

	// Create 3 active endpoints
	for i := 0; i < 3; i++ {
		body := map[string]interface{}{
			"slug":           fmt.Sprintf("dashboard-%d", i),
			"name":           fmt.Sprintf("Dashboard EP %d", i),
			"provider":       "openai",
			"endpoint_url":   "https://api.openai.com/v1",
			"is_fixed_model": true,
			"model_name":     "gpt-4o",
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
		w := httptest.NewRecorder()
		epH.Create(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create ep %d: expected 201, got %d", i, w.Code)
		}
	}

	// Check discovery shows 3 endpoints
	req := agentRequest(http.MethodGet, "/api/v1/discovery", nil, "viewer")
	w := httptest.NewRecorder()
	discoveryH.GetDiscovery(w, req)
	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})
	eps := data["model_endpoints"].([]interface{})
	if len(eps) != 3 {
		t.Fatalf("expected 3 endpoints in discovery, got %d", len(eps))
	}

	// Soft-delete one
	req = agentRequest(http.MethodDelete, "/api/v1/model-endpoints/dashboard-0", nil, "editor")
	req = withSlugParam(req, "dashboard-0")
	w = httptest.NewRecorder()
	epH.Delete(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	// Discovery should now show 2 active endpoints
	req = agentRequest(http.MethodGet, "/api/v1/discovery", nil, "viewer")
	w = httptest.NewRecorder()
	discoveryH.GetDiscovery(w, req)
	env = parseEnvelope(t, w)
	data = env.Data.(map[string]interface{})
	eps = data["model_endpoints"].([]interface{})
	if len(eps) != 2 {
		t.Fatalf("expected 2 active endpoints after delete, got %d", len(eps))
	}
}

// =============================================================================
// EDGE CASES
// =============================================================================

func TestModelEndpoints_ActivateVersion_NonExistent(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "no-version", Name: "No Version", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
	}
	epStore.slugs["no-version"] = epID

	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/no-version/versions/999/activate", nil, "editor")
	req = withSlugAndVersionParams(req, "no-version", "999")
	w := httptest.NewRecorder()
	h.ActivateVersion(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent version, got %d", w.Code)
	}
}

func TestModelEndpoints_ActivateVersion_AlreadyActive(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "already-active", Name: "Already Active", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
	}
	epStore.slugs["already-active"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{
			ID: uuid.New(), EndpointID: epID, Version: 1,
			Config: json.RawMessage(`{}`), IsActive: true, CreatedAt: time.Now(),
		},
	}

	// Activating an already-active version should be idempotent
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/already-active/versions/1/activate", nil, "editor")
	req = withSlugAndVersionParams(req, "already-active", "1")
	w := httptest.NewRecorder()
	h.ActivateVersion(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for re-activating already-active version, got %d; body: %s", w.Code, w.Body.String())
	}

	// Still exactly one active
	activeCount := 0
	for _, v := range epStore.versions[epID] {
		if v.IsActive {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 active version, got %d", activeCount)
	}
}

func TestModelEndpoints_CreateVersion_InvalidConfig(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "config-validation", Name: "Config Validation", Provider: "openai",
		EndpointURL: "https://api.openai.com/v1", IsActive: true,
	}
	epStore.slugs["config-validation"] = epID

	invalidConfigs := []struct {
		name   string
		config map[string]interface{}
	}{
		{"temperature too high", map[string]interface{}{"temperature": 3.0}},
		{"temperature negative", map[string]interface{}{"temperature": -1.0}},
		{"max_tokens zero", map[string]interface{}{"max_tokens": 0}},
		{"max_tokens negative", map[string]interface{}{"max_tokens": -100}},
		{"top_p too high", map[string]interface{}{"top_p": 1.5}},
		{"top_p negative", map[string]interface{}{"top_p": -0.1}},
		{"context_window zero", map[string]interface{}{"context_window": 0}},
		{"max_output_tokens zero", map[string]interface{}{"max_output_tokens": 0}},
	}

	for _, tc := range invalidConfigs {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]interface{}{
				"config":      tc.config,
				"change_note": "Invalid config test",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/config-validation/versions", body, "editor")
			req = withSlugParam(req, "config-validation")
			w := httptest.NewRecorder()
			h.CreateVersion(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d; body: %s", tc.name, w.Code, w.Body.String())
			}
		})
	}
}

func TestModelEndpoints_TypeScript_DiscoveryResponse_HasAllKeys(t *testing.T) {
	// Verify the DiscoveryResponse TypeScript type is satisfied:
	// agents, mcp_servers, trust_defaults, model_config, model_endpoints, fetched_at
	t.Parallel()

	agentStore := &mockDiscoveryAgentStore{agents: []store.Agent{}}
	mcpStore := &mockDiscoveryMCPStore{servers: []store.MCPServer{}}
	trustStore := &mockDiscoveryTrustStore{defaults: []store.TrustDefault{}}
	modelConfigStore := &mockDiscoveryModelConfigStore{config: nil}
	epStore := newMockModelEndpointStore()

	h := NewDiscoveryHandler(agentStore, mcpStore, trustStore, modelConfigStore, epStore)

	req := agentRequest(http.MethodGet, "/api/v1/discovery", nil, "viewer")
	w := httptest.NewRecorder()
	h.GetDiscovery(w, req)

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	requiredKeys := []string{
		"agents", "mcp_servers", "trust_defaults",
		"model_config", "model_endpoints", "fetched_at",
	}
	for _, key := range requiredKeys {
		if _, ok := data[key]; !ok {
			t.Errorf("MISSING KEY: DiscoveryResponse requires '%s' (TypeScript type mismatch)", key)
		}
	}

	// Verify fetched_at is a valid timestamp
	fetchedAt, ok := data["fetched_at"].(string)
	if !ok {
		t.Error("fetched_at should be a string")
	} else {
		if _, err := time.Parse(time.RFC3339, fetchedAt); err != nil {
			t.Errorf("fetched_at should be RFC3339 format, got: %s", fetchedAt)
		}
	}
}

func TestModelEndpoints_TypeScript_ModelEndpointVersion_NoExtraFields(t *testing.T) {
	// The API response should not contain fields that TypeScript doesn't expect.
	// This catches accidentally leaking internal fields.
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create endpoint and version
	body := map[string]interface{}{
		"slug":           "field-check",
		"name":           "Field Check",
		"provider":       "openai",
		"endpoint_url":   "https://api.openai.com/v1",
		"is_fixed_model": true,
		"model_name":     "gpt-4o",
	}
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
	w := httptest.NewRecorder()
	h.Create(w, req)

	env := parseEnvelope(t, w)
	data := env.Data.(map[string]interface{})

	// Verify no auth_credential, password_hash, or other internal fields leaked
	sensitiveFields := []string{"auth_credential", "password_hash", "secret", "token", "enc_key"}
	for _, field := range sensitiveFields {
		if _, ok := data[field]; ok {
			t.Errorf("SECURITY: response should not contain '%s'", field)
		}
	}

	// Also check body string for secret patterns
	bodyStr := w.Body.String()
	if strings.Contains(bodyStr, "auth_credential") {
		t.Error("SECURITY: raw response contains 'auth_credential'")
	}
}

// =============================================================================
// Thread-safe mock store for concurrency tests
// =============================================================================

type concurrentMockModelEndpointStore struct {
	mu        sync.Mutex
	endpoints map[uuid.UUID]*store.ModelEndpoint
	slugs     map[string]uuid.UUID
	versions  map[uuid.UUID][]store.ModelEndpointVersion
}

func newConcurrentMockModelEndpointStore() *concurrentMockModelEndpointStore {
	return &concurrentMockModelEndpointStore{
		endpoints: make(map[uuid.UUID]*store.ModelEndpoint),
		slugs:     make(map[string]uuid.UUID),
		versions:  make(map[uuid.UUID][]store.ModelEndpointVersion),
	}
}

func (m *concurrentMockModelEndpointStore) Create(_ context.Context, ep *store.ModelEndpoint, _ json.RawMessage, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *concurrentMockModelEndpointStore) GetBySlug(_ context.Context, slug string) (*store.ModelEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.slugs[slug]
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: model endpoint '%s' not found", slug)
	}
	ep := *m.endpoints[id]
	return &ep, nil
}

func (m *concurrentMockModelEndpointStore) List(_ context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []store.ModelEndpoint
	for _, ep := range m.endpoints {
		if activeOnly && !ep.IsActive {
			continue
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

func (m *concurrentMockModelEndpointStore) Update(_ context.Context, ep *store.ModelEndpoint, etag time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.endpoints[ep.ID]
	if !ok {
		return fmt.Errorf("NOT_FOUND: model endpoint not found")
	}
	if !existing.UpdatedAt.Equal(etag) {
		return fmt.Errorf("CONFLICT: resource was modified by another client")
	}
	// Store a copy to avoid data races
	epCopy := *ep
	epCopy.UpdatedAt = time.Now()
	ep.UpdatedAt = epCopy.UpdatedAt
	m.endpoints[ep.ID] = &epCopy
	m.slugs[ep.Slug] = ep.ID
	return nil
}

func (m *concurrentMockModelEndpointStore) Delete(_ context.Context, slug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.slugs[slug]
	if !ok {
		return fmt.Errorf("NOT_FOUND: model endpoint not found")
	}
	ep := m.endpoints[id]
	ep.IsActive = false
	return nil
}

func (m *concurrentMockModelEndpointStore) CreateVersion(_ context.Context, endpointID uuid.UUID, config json.RawMessage, changeNote, createdBy string) (*store.ModelEndpointVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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

	v := store.ModelEndpointVersion{
		ID:         uuid.New(),
		EndpointID: endpointID,
		Version:    maxVer + 1,
		Config:     config,
		IsActive:   true,
		ChangeNote: changeNote,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now(),
	}
	m.versions[endpointID] = append(m.versions[endpointID], v)
	// Return a copy to avoid data races
	result := v
	return &result, nil
}

func (m *concurrentMockModelEndpointStore) ListVersions(_ context.Context, endpointID uuid.UUID, offset, limit int) ([]store.ModelEndpointVersion, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *concurrentMockModelEndpointStore) GetVersion(_ context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions[endpointID] {
		if v.Version == version {
			result := v
			return &result, nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: version %d not found", version)
}

func (m *concurrentMockModelEndpointStore) ActivateVersion(_ context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	// Return a copy to avoid data races with concurrent JSON serialization
	result := versions[targetIdx]
	return &result, nil
}

func (m *concurrentMockModelEndpointStore) GetActiveVersion(_ context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions[endpointID] {
		if v.IsActive {
			result := v
			return &result, nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: no active version")
}

func (m *concurrentMockModelEndpointStore) CountAll(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.endpoints), nil
}
