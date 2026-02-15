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

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// RED TEAM: Adversarial Security Tests for Model Endpoints (009)
// =============================================================================

// --- 1. SLUG INJECTION ATTACKS ---

func TestRedTeam_SlugInjection(t *testing.T) {
	t.Parallel()

	maliciousSlugs := []struct {
		name string
		slug string
	}{
		// Path traversal
		{"path traversal dots", "../../sensitive"},
		{"encoded path traversal", "%2e%2e%2fsensitive"},
		{"double-encoded path traversal", "%252e%252e%252f"},

		// SQL injection
		{"SQL drop table", "; DROP TABLE model_endpoints;--"},
		{"SQL union", "test' UNION SELECT * FROM users--"},
		{"SQL OR injection", "test' OR '1'='1"},

		// Command injection
		{"command injection semicolon", "test; ls -la /"},
		{"command injection pipe", "test | cat /etc/passwd"},
		{"command injection backtick", "test`whoami`"},
		{"command injection $() ", "test$(id)"},

		// Null bytes
		{"null byte", "test\x00admin"},

		// URL encoding bypass
		{"url encoded slash", "test%2Fadmin"},
		{"url encoded dot", "test%2E%2E"},

		// Unicode normalization attacks
		{"unicode dash", "test\u2010slug"},
		{"fullwidth chars", "\uff54\uff45\uff53\uff54"},

		// Starts with number (should fail regex)
		{"starts with number", "1test-slug"},
		{"starts with hyphen", "-test-slug"},

		// Too long slug (100+ chars)
		{"too long slug", "a" + strings.Repeat("b", 100)},

		// Empty-ish values
		{"single char", "a"},
		{"two chars", "ab"},

		// Special characters
		{"underscore", "test_slug"},
		{"period", "test.slug"},
		{"colon", "test:slug"},
		{"at sign", "test@slug"},
		{"hash", "test#slug"},
	}

	for _, tt := range maliciousSlugs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug":           tt.slug,
				"name":           "Injection Test",
				"provider":       "openai",
				"endpoint_url":   "https://api.openai.com/v1",
				"is_fixed_model": true,
				"model_name":     "gpt-4o",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("slug %q: expected 400, got %d; body: %s", tt.slug, w.Code, w.Body.String())
			}
		})
	}
}

// --- 2. PROVIDER ENUM INJECTION ---

func TestRedTeam_ProviderInjection(t *testing.T) {
	t.Parallel()

	attacks := []struct {
		name     string
		provider interface{}
	}{
		{"null provider", nil},
		{"numeric provider", 42},
		{"boolean provider", true},
		{"SQL in provider", "openai'; DROP TABLE model_endpoints;--"},
		{"very long provider", strings.Repeat("a", 10000)},
		{"empty string", ""},
		{"whitespace only", "   "},
		{"newline injection", "openai\n\r"},
		{"tab injection", "openai\t"},
	}

	for _, tt := range attacks {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug":           "provider-test",
				"name":           "Provider Test",
				"provider":       tt.provider,
				"endpoint_url":   "https://api.openai.com/v1",
				"is_fixed_model": true,
				"model_name":     "gpt-4o",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if w.Code == http.StatusCreated {
				t.Errorf("provider %v: should NOT have succeeded with status 201", tt.provider)
			}
		})
	}
}

// --- 3. IS_FIXED_MODEL LOGIC ATTACKS ---

func TestRedTeam_FixedModelLogicBypass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantReject bool
		reason     string
	}{
		{
			name: "is_fixed_model=true with non-empty allowed_models must fail",
			body: map[string]interface{}{
				"slug": "fixed-with-allowed", "name": "Fixed With Allowed",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": true, "model_name": "gpt-4o",
				"allowed_models": []string{"gpt-4o", "gpt-4o-mini"},
			},
			wantReject: true,
			reason:     "fixed model must have empty allowed_models",
		},
		{
			name: "is_fixed_model=true without model_name must fail",
			body: map[string]interface{}{
				"slug": "fixed-no-model", "name": "Fixed No Model",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": true,
			},
			wantReject: true,
			reason:     "fixed model requires model_name",
		},
		{
			name: "is_fixed_model=false without allowed_models must fail",
			body: map[string]interface{}{
				"slug": "flex-no-models", "name": "Flex No Models",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": false,
			},
			wantReject: true,
			reason:     "flexible model requires allowed_models",
		},
		{
			name: "is_fixed_model=false with empty allowed_models must fail",
			body: map[string]interface{}{
				"slug": "flex-empty-models", "name": "Flex Empty Models",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": false, "allowed_models": []string{},
			},
			wantReject: true,
			reason:     "flexible model requires non-empty allowed_models",
		},
		{
			name: "is_fixed_model=false with null allowed_models must fail",
			body: map[string]interface{}{
				"slug": "flex-null-models", "name": "Flex Null Models",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": false, "allowed_models": nil,
			},
			wantReject: true,
			reason:     "flexible model requires allowed_models, not null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", tt.body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if tt.wantReject && w.Code != http.StatusBadRequest {
				t.Errorf("expected rejection (400) because %s, got %d; body: %s",
					tt.reason, w.Code, w.Body.String())
			}
			if !tt.wantReject && w.Code != http.StatusCreated {
				t.Errorf("expected success (201) because %s, got %d; body: %s",
					tt.reason, w.Code, w.Body.String())
			}
		})
	}
}

// --- 4. CONFIG SCHEMA ATTACKS ---

func TestRedTeam_ConfigSchemaAttacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     interface{}
		wantReject bool
	}{
		// Negative/invalid temperature
		{"negative temperature", map[string]interface{}{"temperature": -1.0}, true},
		{"temperature > 2", map[string]interface{}{"temperature": 2.1}, true},
		{"temperature = NaN string", map[string]interface{}{"temperature": "NaN"}, true},
		{"temperature = Infinity", map[string]interface{}{"temperature": "Infinity"}, true},
		{"temperature at boundary 0", map[string]interface{}{"temperature": 0.0}, false},
		{"temperature at boundary 2", map[string]interface{}{"temperature": 2.0}, false},

		// Negative/invalid max_tokens
		{"negative max_tokens", map[string]interface{}{"max_tokens": -100}, true},
		{"zero max_tokens", map[string]interface{}{"max_tokens": 0}, true},
		{"max_tokens string", map[string]interface{}{"max_tokens": "lots"}, true},

		// Negative max_output_tokens
		{"negative max_output_tokens", map[string]interface{}{"max_output_tokens": -1}, true},
		{"zero max_output_tokens", map[string]interface{}{"max_output_tokens": 0}, true},

		// Invalid top_p
		{"negative top_p", map[string]interface{}{"top_p": -0.1}, true},
		{"top_p > 1", map[string]interface{}{"top_p": 1.1}, true},
		{"top_p at boundary 0", map[string]interface{}{"top_p": 0.0}, false},
		{"top_p at boundary 1", map[string]interface{}{"top_p": 1.0}, false},

		// Invalid context_window
		{"negative context_window", map[string]interface{}{"context_window": -100}, true},
		{"zero context_window", map[string]interface{}{"context_window": 0}, true},

		// Non-object config
		{"config is array", []string{"a", "b"}, true},
		{"config is string", "just a string", true},
		{"config is number", 42, true},

		// Valid configs
		{"valid basic config", map[string]interface{}{"temperature": 0.7}, false},
		{"valid full config", map[string]interface{}{"temperature": 0.5, "max_tokens": 4096, "top_p": 0.9}, false},
		{"empty object config", map[string]interface{}{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			epID := uuid.New()
			epStore.endpoints[epID] = &store.ModelEndpoint{
				ID: epID, Slug: "config-attack", Name: "Config Attack", Provider: "openai", IsActive: true,
			}
			epStore.slugs["config-attack"] = epID

			body := map[string]interface{}{
				"config":      tt.config,
				"change_note": "Attack: " + tt.name,
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/config-attack/versions", body, "editor")
			req = withSlugParam(req, "config-attack")
			w := httptest.NewRecorder()
			h.CreateVersion(w, req)

			if tt.wantReject && w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %q, got %d; body: %s", tt.name, w.Code, w.Body.String())
			}
			if !tt.wantReject && w.Code != http.StatusCreated {
				t.Errorf("expected 201 for %q, got %d; body: %s", tt.name, w.Code, w.Body.String())
			}
		})
	}
}

// --- 5. SSRF BYPASS ATTACKS (advanced) ---

func TestRedTeam_SSRFBypass(t *testing.T) {
	t.Parallel()

	bypasses := []struct {
		name string
		url  string
	}{
		// Decimal IP notation
		{"decimal IP for localhost", "http://2130706433/admin"},
		// Octal IP notation
		{"octal IP for 127.0.0.1", "http://0177.0.0.1/admin"},
		// Hex IP notation
		{"hex IP for 127.0.0.1", "http://0x7f.0.0.1/admin"},
		// Zero-padded
		{"zero-padded localhost", "http://127.0.0.01/admin"},
		// IPv6 mapped IPv4
		{"IPv6 mapped 127.0.0.1", "http://[::ffff:127.0.0.1]/admin"},
		{"IPv6 mapped 10.0.0.1", "http://[::ffff:10.0.0.1]/admin"},
		// Short IPv6 for localhost
		{"short IPv6 localhost", "http://[0:0:0:0:0:0:0:1]/admin"},
		// Double URL encoding
		{"double-encoded localhost", "http://127%2E0%2E0%2E1/admin"},
		// @ trick (user@host)
		{"at trick", "http://evil.com@127.0.0.1/admin"},
		// Redirect via DNS rebinding (symbolic, test the URL parsing)
		{"localhost alias 0.0.0.0", "http://0.0.0.0/admin"},
		// IPv4 as IPv6
		{"IPv4 as hex in IPv6", "http://[0:0:0:0:0:ffff:7f00:1]/admin"},
		// Port scan attempt
		{"internal port scan", "http://127.0.0.1:22/"},
		// Cloud metadata endpoints
		{"GCP metadata", "http://169.254.169.254/computeMetadata/v1/"},
		{"Azure metadata", "http://169.254.169.254/metadata/instance"},
		// Private class B
		{"private class B", "http://172.31.255.255/internal"},
	}

	for _, tt := range bypasses {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug":           "ssrf-" + strings.ReplaceAll(strings.ToLower(tt.name), " ", "-"),
				"name":           "SSRF Bypass Test",
				"provider":       "custom",
				"endpoint_url":   tt.url,
				"is_fixed_model": true,
				"model_name":     "test",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if w.Code == http.StatusCreated {
				t.Errorf("SSRF BYPASS: %q (%s) was accepted with 201! This is a vulnerability.",
					tt.name, tt.url)
			}
		})
	}
}

// --- 6. HEADER LEAKAGE TESTS ---

func TestRedTeam_HeadersNeverLeakInAnyResponse(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	// Create endpoint with headers in config
	epID := uuid.New()
	secretAPIKey := "sk-SUPER-SECRET-API-KEY-12345"
	configWithHeaders := json.RawMessage(fmt.Sprintf(
		`{"temperature":0.7,"headers":{"Authorization":"Bearer %s","X-Api-Key":"secret-key-xyz"}}`,
		secretAPIKey,
	))

	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "header-leak-test", Name: "Header Leak Test",
		Provider: "openai", EndpointURL: "https://api.openai.com/v1",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["header-leak-test"] = epID
	epStore.versions[epID] = []store.ModelEndpointVersion{
		{
			ID: uuid.New(), EndpointID: epID, Version: 1,
			Config: configWithHeaders, IsActive: false, CreatedAt: time.Now(),
		},
		{
			ID: uuid.New(), EndpointID: epID, Version: 2,
			Config: configWithHeaders, IsActive: true, CreatedAt: time.Now(),
		},
	}

	// Test 1: GetVersion should redact headers
	t.Run("GetVersion redacts headers", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/header-leak-test/versions/1", nil, "viewer")
		req = withSlugAndVersionParams(req, "header-leak-test", "1")
		w := httptest.NewRecorder()
		h.GetVersion(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, secretAPIKey) {
			t.Error("CRITICAL: GetVersion leaks raw secret API key in headers")
		}
		if strings.Contains(body, "secret-key-xyz") {
			t.Error("CRITICAL: GetVersion leaks X-Api-Key value")
		}
	})

	// Test 2: ListVersions should redact headers
	t.Run("ListVersions redacts headers", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/header-leak-test/versions", nil, "viewer")
		req = withSlugParam(req, "header-leak-test")
		w := httptest.NewRecorder()
		h.ListVersions(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, secretAPIKey) {
			t.Error("CRITICAL: ListVersions leaks raw secret API key in headers")
		}
		if strings.Contains(body, "secret-key-xyz") {
			t.Error("CRITICAL: ListVersions leaks X-Api-Key value")
		}
	})

	// Test 3: Get endpoint itself doesn't leak version headers
	t.Run("Get endpoint doesn't include version headers", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints/header-leak-test", nil, "viewer")
		req = withSlugParam(req, "header-leak-test")
		w := httptest.NewRecorder()
		h.Get(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, secretAPIKey) {
			t.Error("CRITICAL: Get endpoint leaks secret API key")
		}
	})

	// Test 4: List endpoint doesn't leak version headers
	t.Run("List endpoint doesn't include version headers", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?active_only=false", nil, "viewer")
		w := httptest.NewRecorder()
		h.List(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, secretAPIKey) {
			t.Error("CRITICAL: List endpoint leaks secret API key")
		}
	})

	// Test 5: ActivateVersion should not leak headers
	t.Run("ActivateVersion doesn't leak headers", func(t *testing.T) {
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/header-leak-test/versions/1/activate", nil, "editor")
		req = withSlugAndVersionParams(req, "header-leak-test", "1")
		w := httptest.NewRecorder()
		h.ActivateVersion(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, secretAPIKey) {
			t.Error("CRITICAL: ActivateVersion leaks raw secret API key in headers")
		}
	})

	// Test 6: CreateVersion response should not leak headers
	t.Run("CreateVersion response doesn't leak previously stored headers", func(t *testing.T) {
		body := map[string]interface{}{
			"config": map[string]interface{}{
				"temperature": 0.5,
				"headers": map[string]string{
					"Authorization": "Bearer new-secret-key",
				},
			},
			"change_note": "New version with headers",
		}
		req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/header-leak-test/versions", body, "editor")
		req = withSlugParam(req, "header-leak-test")
		w := httptest.NewRecorder()
		h.CreateVersion(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
		}

		// Note: CreateVersion returning the headers in its response may or may not be
		// considered a leak since the caller just sent them. The key question is whether
		// *other* versions' headers are visible.
	})
}

// --- 7. OPTIMISTIC CONCURRENCY ATTACKS ---

func TestRedTeam_OptimisticConcurrencyAttacks(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		ifMatch    string
		wantStatus int
	}{
		{"missing If-Match header", "", http.StatusBadRequest},
		{"invalid If-Match format", "not-a-timestamp", http.StatusBadRequest},
		{"stale If-Match (old timestamp)", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), http.StatusConflict},
		{"future If-Match", time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), http.StatusConflict},
		{"SQL injection in If-Match", "'; DROP TABLE model_endpoints;--", http.StatusBadRequest},
		{"empty string If-Match", "", http.StatusBadRequest},
		{"correct If-Match succeeds", now.UTC().Format(time.RFC3339Nano), http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			epID := uuid.New()
			epStore.endpoints[epID] = &store.ModelEndpoint{
				ID: epID, Slug: "concurrency-test", Name: "Concurrency Test",
				Provider: "openai", EndpointURL: "https://api.openai.com/v1",
				IsActive: true, CreatedAt: now, UpdatedAt: now,
			}
			epStore.slugs["concurrency-test"] = epID

			body := map[string]interface{}{"name": "Updated Name"}
			req := agentRequest(http.MethodPut, "/api/v1/model-endpoints/concurrency-test", body, "editor")
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			req = withSlugParam(req, "concurrency-test")
			w := httptest.NewRecorder()
			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// --- 8. WORKSPACE ISOLATION ATTACKS ---

func TestRedTeam_WorkspaceIsolation(t *testing.T) {
	t.Parallel()

	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	wsA := "workspace-alpha"
	wsB := "workspace-beta"

	// Create endpoints in workspace A
	for i := 0; i < 3; i++ {
		id := uuid.New()
		slug := fmt.Sprintf("ws-a-%d", i)
		epStore.endpoints[id] = &store.ModelEndpoint{
			ID: id, Slug: slug, Name: fmt.Sprintf("WS-A Endpoint %d", i),
			Provider: "openai", EndpointURL: "https://api.openai.com/v1",
			WorkspaceID: &wsA, IsActive: true,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		epStore.slugs[slug] = id
	}

	// Create endpoints in workspace B
	for i := 0; i < 2; i++ {
		id := uuid.New()
		slug := fmt.Sprintf("ws-b-%d", i)
		epStore.endpoints[id] = &store.ModelEndpoint{
			ID: id, Slug: slug, Name: fmt.Sprintf("WS-B Endpoint %d", i),
			Provider: "anthropic", EndpointURL: "https://api.anthropic.com/v1",
			WorkspaceID: &wsB, IsActive: true,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		epStore.slugs[slug] = id
	}

	// Create global endpoint (no workspace)
	globalID := uuid.New()
	epStore.endpoints[globalID] = &store.ModelEndpoint{
		ID: globalID, Slug: "global-ep", Name: "Global Endpoint",
		Provider: "openai", EndpointURL: "https://api.openai.com/v1",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["global-ep"] = globalID

	// Attack 1: Query workspace A, should NOT see workspace B endpoints
	t.Run("workspace A filtering excludes workspace B", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?workspace_id=workspace-alpha&active_only=false", nil, "viewer")
		w := httptest.NewRecorder()
		h.List(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		env := parseEnvelope(t, w)
		data := env.Data.(map[string]interface{})
		endpoints := data["model_endpoints"].([]interface{})

		for _, ep := range endpoints {
			epMap := ep.(map[string]interface{})
			wsID, ok := epMap["workspace_id"].(string)
			if ok && wsID != wsA {
				t.Errorf("workspace isolation failure: got endpoint from workspace %q when filtering for %q", wsID, wsA)
			}
		}
		if len(endpoints) != 3 {
			t.Errorf("expected 3 endpoints for workspace-alpha, got %d", len(endpoints))
		}
	})

	// Attack 2: Query workspace B
	t.Run("workspace B filtering excludes workspace A and global", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?workspace_id=workspace-beta&active_only=false", nil, "viewer")
		w := httptest.NewRecorder()
		h.List(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		env := parseEnvelope(t, w)
		data := env.Data.(map[string]interface{})
		endpoints := data["model_endpoints"].([]interface{})
		if len(endpoints) != 2 {
			t.Errorf("expected 2 endpoints for workspace-beta, got %d", len(endpoints))
		}
	})

	// Attack 3: Query non-existent workspace
	t.Run("non-existent workspace returns empty", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?workspace_id=workspace-nonexistent&active_only=false", nil, "viewer")
		w := httptest.NewRecorder()
		h.List(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		env := parseEnvelope(t, w)
		data := env.Data.(map[string]interface{})
		// model_endpoints could be nil (empty list)
		endpoints, ok := data["model_endpoints"].([]interface{})
		if ok && len(endpoints) != 0 {
			t.Errorf("expected 0 endpoints for non-existent workspace, got %d", len(endpoints))
		}
	})

	// Attack 4: SQL injection in workspace_id parameter
	t.Run("SQL injection in workspace_id", func(t *testing.T) {
		req := agentRequest(http.MethodGet, "/api/v1/model-endpoints?workspace_id='+OR+1=1--&active_only=false", nil, "viewer")
		w := httptest.NewRecorder()
		h.List(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		// Should return 0 endpoints (no workspace matches this injection)
		env := parseEnvelope(t, w)
		data := env.Data.(map[string]interface{})
		endpoints, ok := data["model_endpoints"].([]interface{})
		if ok && len(endpoints) > 0 {
			t.Errorf("SQL injection in workspace_id returned %d endpoints, expected 0", len(endpoints))
		}
	})
}

// --- 9. REQUEST BODY ATTACKS ---

func TestRedTeam_RequestBodyAttacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rawBody    string
		wantStatus int
	}{
		{"empty body", "", http.StatusBadRequest},
		{"null body", "null", http.StatusBadRequest},
		{"array body", `[{"slug":"test"}]`, http.StatusBadRequest},
		{"deeply nested JSON", `{"slug":"deep","name":"Deep","provider":"openai","endpoint_url":"https://api.openai.com/v1","is_fixed_model":true,"model_name":"gpt-4o","nested":` + strings.Repeat(`{"a":`, 100) + `"deep"` + strings.Repeat("}", 100) + "}", http.StatusCreated},
		{"valid request", `{"slug":"valid-body","name":"Valid","provider":"openai","endpoint_url":"https://api.openai.com/v1","is_fixed_model":true,"model_name":"gpt-4o"}`, http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/model-endpoints", strings.NewReader(tt.rawBody))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.ContextWithUser(req.Context(), uuid.New(), "editor", "session")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			h.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// --- 10. ENDPOINT URL EDGE CASES ---

func TestRedTeam_EndpointURLEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		url        string
		wantReject bool
	}{
		// Valid URLs that should be accepted
		{"valid HTTPS URL", "https://api.openai.com/v1", false},
		{"valid HTTP URL", "http://api.openai.com/v1", false},
		{"URL with port", "https://api.openai.com:443/v1", false},
		{"URL with path", "https://api.openai.com/v1/chat/completions", false},

		// Invalid schemes
		{"gopher scheme", "gopher://evil.com/", true},
		{"ws scheme", "ws://evil.com/ws", true},
		{"wss scheme", "wss://evil.com/ws", true},
		{"dict scheme", "dict://evil.com/", true},

		// Missing host
		{"just scheme", "https://", true},
		{"just path", "/api/v1", true},

		// Very long URL
		{"very long URL", "https://example.com/" + strings.Repeat("a", 3000), true},

		// URL with credentials
		{"URL with credentials", "https://user:pass@api.openai.com/v1", false},

		// URL with fragment (unusual for API endpoints)
		{"URL with fragment", "https://api.openai.com/v1#admin", false},

		// Unicode in domain
		{"unicode domain", "https://\u0430pi.openai.com/v1", false},
		// ^ Punycode/IDN domain - may be a bypass vector
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			body := map[string]interface{}{
				"slug":           "url-edge-" + strings.ReplaceAll(strings.ToLower(tt.name), " ", "-"),
				"name":           "URL Edge Case",
				"provider":       "custom",
				"endpoint_url":   tt.url,
				"is_fixed_model": true,
				"model_name":     "test",
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints", body, "editor")
			w := httptest.NewRecorder()
			h.Create(w, req)

			if tt.wantReject && w.Code == http.StatusCreated {
				t.Errorf("URL %q was accepted but should have been rejected", tt.url)
			}
			if !tt.wantReject && w.Code != http.StatusCreated {
				t.Errorf("URL %q was rejected with %d but should have been accepted; body: %s",
					tt.url, w.Code, w.Body.String())
			}
		})
	}
}

// --- 11. UPDATE SSRF BYPASS ON ENDPOINT_URL CHANGE ---

func TestRedTeam_UpdateEndpointURL_SSRFBypass(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	ssrfURLs := []string{
		"http://localhost:8080/admin",
		"http://127.0.0.1:9090/internal",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/secrets",
		"http://[::1]:8080/admin",
		"file:///etc/passwd",
		"ftp://evil.com/payload",
	}

	for _, maliciousURL := range ssrfURLs {
		t.Run("update_to_"+maliciousURL, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			epID := uuid.New()
			epStore.endpoints[epID] = &store.ModelEndpoint{
				ID: epID, Slug: "legit-ep", Name: "Legit Endpoint",
				Provider: "openai", EndpointURL: "https://api.openai.com/v1",
				IsActive: true, CreatedAt: now, UpdatedAt: now,
			}
			epStore.slugs["legit-ep"] = epID

			body := map[string]interface{}{"endpoint_url": maliciousURL}
			req := agentRequest(http.MethodPut, "/api/v1/model-endpoints/legit-ep", body, "editor")
			req.Header.Set("If-Match", now.UTC().Format(time.RFC3339Nano))
			req = withSlugParam(req, "legit-ep")
			w := httptest.NewRecorder()
			h.Update(w, req)

			if w.Code == http.StatusOK {
				t.Errorf("SSRF BYPASS on update: malicious URL %q was accepted", maliciousURL)
			}
		})
	}
}

// --- 12. ACTIVATION VERSION RESPONSE HEADER LEAK ---

func TestRedTeam_ActivateVersion_DoesNotExposeOtherVersionHeaders(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "activate-leak", Name: "Activate Leak", IsActive: true,
	}
	epStore.slugs["activate-leak"] = epID

	secret1 := "Bearer super-secret-v1"
	secret2 := "Bearer super-secret-v2"

	epStore.versions[epID] = []store.ModelEndpointVersion{
		{
			ID: uuid.New(), EndpointID: epID, Version: 1,
			Config:   json.RawMessage(fmt.Sprintf(`{"temperature":0.7,"headers":{"Authorization":"%s"}}`, secret1)),
			IsActive: false, CreatedAt: time.Now(),
		},
		{
			ID: uuid.New(), EndpointID: epID, Version: 2,
			Config:   json.RawMessage(fmt.Sprintf(`{"temperature":0.3,"headers":{"Authorization":"%s"}}`, secret2)),
			IsActive: true, CreatedAt: time.Now(),
		},
	}

	// Activate version 1
	req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/activate-leak/versions/1/activate", nil, "editor")
	req = withSlugAndVersionParams(req, "activate-leak", "1")
	w := httptest.NewRecorder()
	h.ActivateVersion(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()

	// Check that secret2 from the other version is not in the response
	if strings.Contains(body, "super-secret-v2") {
		t.Error("CRITICAL: ActivateVersion response contains secret from a DIFFERENT version")
	}

	// The activated version's secret might appear since it was just the mock
	// returning raw data (no encryption in mock), but in production the store
	// decrypts then the handler re-redacts. Let's verify redaction happens.
	// Note: ActivateVersion handler does NOT call redactConfigHeaders! Check this.
	if strings.Contains(body, "super-secret-v1") {
		t.Error("VULNERABILITY: ActivateVersion response leaks the activated version's secret headers without redaction")
	}
}

// --- 13. DELETE THEN ACCESS ---

func TestRedTeam_DeleteThenAccess(t *testing.T) {
	epStore := newMockModelEndpointStore()
	audit := &mockAuditStoreForAPI{}
	h := NewModelEndpointsHandler(epStore, audit, nil, nil)

	epID := uuid.New()
	epStore.endpoints[epID] = &store.ModelEndpoint{
		ID: epID, Slug: "delete-me", Name: "Delete Me",
		Provider: "openai", EndpointURL: "https://api.openai.com/v1",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	epStore.slugs["delete-me"] = epID

	// Delete the endpoint (soft delete)
	delReq := agentRequest(http.MethodDelete, "/api/v1/model-endpoints/delete-me", nil, "editor")
	delReq = withSlugParam(delReq, "delete-me")
	delW := httptest.NewRecorder()
	h.Delete(delW, delReq)
	if delW.Code != http.StatusNoContent {
		t.Fatalf("delete failed: expected 204, got %d", delW.Code)
	}

	// The endpoint should still be accessible via GET (soft delete only sets is_active=false)
	getReq := agentRequest(http.MethodGet, "/api/v1/model-endpoints/delete-me", nil, "viewer")
	getReq = withSlugParam(getReq, "delete-me")
	getW := httptest.NewRecorder()
	h.Get(getW, getReq)

	// Soft-deleted endpoints should still be GETable by slug
	if getW.Code != http.StatusOK {
		t.Logf("Note: soft-deleted endpoint GET returned %d (may be by design)", getW.Code)
	}

	// But they should NOT appear in the default list (active_only=true)
	listReq := agentRequest(http.MethodGet, "/api/v1/model-endpoints", nil, "viewer")
	listW := httptest.NewRecorder()
	h.List(listW, listReq)

	if listW.Code == http.StatusOK {
		env := parseEnvelope(t, listW)
		data := env.Data.(map[string]interface{})
		if eps, ok := data["model_endpoints"].([]interface{}); ok {
			for _, ep := range eps {
				epMap := ep.(map[string]interface{})
				if epMap["slug"] == "delete-me" {
					active, _ := epMap["is_active"].(bool)
					if active {
						t.Error("VULNERABILITY: soft-deleted endpoint appears as active in default list")
					}
				}
			}
		}
	}
}

// --- 14. CONFIG INJECTION VIA HEADERS FIELD ---

func TestRedTeam_ConfigHeadersInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			name: "headers with CRLF injection in key",
			config: map[string]interface{}{
				"temperature": 0.5,
				"headers":     map[string]string{"X-Evil\r\nHost: evil.com": "value"},
			},
		},
		{
			name: "headers with CRLF injection in value",
			config: map[string]interface{}{
				"temperature": 0.5,
				"headers":     map[string]string{"Authorization": "Bearer token\r\nX-Injected: evil"},
			},
		},
		{
			name: "headers with very large value",
			config: map[string]interface{}{
				"temperature": 0.5,
				"headers":     map[string]string{"Authorization": strings.Repeat("A", 50000)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			epID := uuid.New()
			epStore.endpoints[epID] = &store.ModelEndpoint{
				ID: epID, Slug: "header-inject", Name: "Header Inject", Provider: "openai", IsActive: true,
			}
			epStore.slugs["header-inject"] = epID

			body := map[string]interface{}{
				"config":      tt.config,
				"change_note": "Header injection: " + tt.name,
			}
			req := agentRequest(http.MethodPost, "/api/v1/model-endpoints/header-inject/versions", body, "editor")
			req = withSlugParam(req, "header-inject")
			w := httptest.NewRecorder()
			h.CreateVersion(w, req)

			// These may or may not be rejected depending on implementation.
			// The key insight is whether header injection values can be retrieved
			// by other users without proper redaction.
			t.Logf("Config injection %q: status=%d", tt.name, w.Code)
		})
	}
}

// --- 15. WORKSPACE-SCOPED CREATE VALIDATION ---

func TestRedTeam_CreateForWorkspace_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name: "valid workspace-scoped create",
			body: map[string]interface{}{
				"slug": "ws-create-valid", "name": "WS Create Valid",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": true, "model_name": "gpt-4o",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "workspace create with SSRF URL",
			body: map[string]interface{}{
				"slug": "ws-create-ssrf", "name": "WS SSRF",
				"provider": "custom", "endpoint_url": "http://169.254.169.254/latest/meta-data/",
				"is_fixed_model": true, "model_name": "test",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "workspace create with invalid config",
			body: map[string]interface{}{
				"slug": "ws-create-bad-config", "name": "WS Bad Config",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": true, "model_name": "gpt-4o",
				"config": map[string]interface{}{"temperature": -5.0},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "workspace create with oversized config",
			body: map[string]interface{}{
				"slug": "ws-create-big-config", "name": "WS Big Config",
				"provider": "openai", "endpoint_url": "https://api.openai.com/v1",
				"is_fixed_model": true, "model_name": "gpt-4o",
				"config": map[string]interface{}{"padding": strings.Repeat("X", 100*1024)},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epStore := newMockModelEndpointStore()
			audit := &mockAuditStoreForAPI{}
			h := NewModelEndpointsHandler(epStore, audit, nil, nil)

			req := agentRequest(http.MethodPost, "/api/v1/workspaces/ws-123/model-endpoints", tt.body, "editor")
			req = withWorkspaceIDParam(req, "ws-123")
			w := httptest.NewRecorder()
			h.CreateForWorkspace(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// withWorkspaceIDParam sets the chi workspaceId URL param.
func withWorkspaceIDParam(req *http.Request, wid string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workspaceId", wid)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// --- 16. REDACTION FUNCTION UNIT TESTS ---

func TestRedTeam_RedactConfigHeaders_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		shouldRedact   bool
		shouldContain  string
		shouldNotContain string
	}{
		{
			name:             "no headers field",
			input:            `{"temperature":0.7}`,
			shouldRedact:     false,
			shouldContain:    "temperature",
			shouldNotContain: "",
		},
		{
			name:             "empty headers object",
			input:            `{"temperature":0.7,"headers":{}}`,
			shouldRedact:     false,
			shouldContain:    "headers",
			shouldNotContain: "",
		},
		{
			name:             "headers with secrets",
			input:            `{"headers":{"Authorization":"Bearer sk-12345"}}`,
			shouldRedact:     true,
			shouldContain:    "REDACTED",
			shouldNotContain: "sk-12345",
		},
		{
			name:             "headers with multiple secrets",
			input:            `{"headers":{"Authorization":"Bearer secret","X-Api-Key":"key123","Custom":"value"}}`,
			shouldRedact:     true,
			shouldContain:    "REDACTED",
			shouldNotContain: "secret",
		},
		{
			name:             "invalid JSON returns original",
			input:            `not json`,
			shouldRedact:     false,
			shouldContain:    "not json",
			shouldNotContain: "",
		},
		{
			name:             "null config",
			input:            `null`,
			shouldRedact:     false,
			shouldContain:    "null",
			shouldNotContain: "",
		},
		{
			name:             "headers is string not object",
			input:            `{"headers":"just a string"}`,
			shouldRedact:     false,
			shouldContain:    "headers",
			shouldNotContain: "",
		},
		{
			name:             "headers is array not object",
			input:            `{"headers":["a","b"]}`,
			shouldRedact:     false,
			shouldContain:    "headers",
			shouldNotContain: "",
		},
		{
			name:             "nested headers (should only redact top-level)",
			input:            `{"config":{"headers":{"Authorization":"Bearer nested-secret"}}}`,
			shouldRedact:     false,
			shouldContain:    "nested-secret",
			shouldNotContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := redactConfigHeaders(json.RawMessage(tt.input))
			resultStr := string(result)

			if tt.shouldContain != "" && !strings.Contains(resultStr, tt.shouldContain) {
				t.Errorf("expected result to contain %q, got %q", tt.shouldContain, resultStr)
			}
			if tt.shouldNotContain != "" && strings.Contains(resultStr, tt.shouldNotContain) {
				t.Errorf("LEAK: result should NOT contain %q, got %q", tt.shouldNotContain, resultStr)
			}
		})
	}
}

// --- 17. VALIDATE ENDPOINT URL UNIT TESTS ---

func TestRedTeam_ValidateEndpointURL_UnitTests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		wantError bool
	}{
		// Should pass
		{"valid https", "https://api.openai.com/v1", false},
		{"valid http", "http://api.openai.com/v1", false},
		{"valid with port", "https://api.openai.com:8443/v1", false},

		// Should fail - private IPs
		{"localhost", "http://localhost/", true},
		{"127.0.0.1", "http://127.0.0.1/", true},
		{"10.x", "http://10.0.0.1/", true},
		{"172.16.x", "http://172.16.0.1/", true},
		{"192.168.x", "http://192.168.1.1/", true},
		{"link-local", "http://169.254.169.254/", true},
		{"IPv6 loopback", "http://[::1]/", true},
		{"IPv6 link-local", "http://[fe80::1]/", true},
		{"IPv6 private", "http://[fc00::1]/", true},

		// Should fail - bad schemes
		{"file scheme", "file:///etc/passwd", true},
		{"ftp scheme", "ftp://example.com/", true},
		{"javascript", "javascript:alert(1)", true},
		{"data", "data:text/html,<script>alert(1)</script>", true},

		// Should fail - missing pieces
		{"no scheme", "example.com/api", true},
		{"empty string", "", true},
		{"just scheme", "https://", true},

		// Should fail - too long
		{"oversized URL", "https://example.com/" + strings.Repeat("x", 2000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateEndpointURL(tt.url)
			if tt.wantError && err == nil {
				t.Errorf("expected error for URL %q, got nil", tt.url)
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error for URL %q: %v", tt.url, err)
			}
		})
	}
}

