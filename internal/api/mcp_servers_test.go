package api

import (
	"bytes"
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

// --- Mock MCP server store ---

type mockMCPServerStore struct {
	servers   map[uuid.UUID]*store.MCPServer
	labels    map[string]uuid.UUID
	createErr error
	updateErr error
}

func newMockMCPServerStore() *mockMCPServerStore {
	return &mockMCPServerStore{
		servers: make(map[uuid.UUID]*store.MCPServer),
		labels:  make(map[string]uuid.UUID),
	}
}

func (m *mockMCPServerStore) Create(_ context.Context, server *store.MCPServer) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.labels[server.Label]; exists {
		return fmt.Errorf("duplicate label")
	}
	if server.ID == uuid.Nil {
		server.ID = uuid.New()
	}
	server.CreatedAt = time.Now()
	server.UpdatedAt = time.Now()
	m.servers[server.ID] = server
	m.labels[server.Label] = server.ID
	return nil
}

func (m *mockMCPServerStore) GetByID(_ context.Context, id uuid.UUID) (*store.MCPServer, error) {
	s, ok := m.servers[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return s, nil
}

func (m *mockMCPServerStore) GetByLabel(_ context.Context, label string) (*store.MCPServer, error) {
	id, ok := m.labels[label]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return m.servers[id], nil
}

func (m *mockMCPServerStore) List(_ context.Context) ([]store.MCPServer, error) {
	var all []store.MCPServer
	for _, s := range m.servers {
		all = append(all, *s)
	}
	return all, nil
}

func (m *mockMCPServerStore) Update(_ context.Context, server *store.MCPServer) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	existing, ok := m.servers[server.ID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if !existing.UpdatedAt.Equal(server.UpdatedAt) {
		return fmt.Errorf("conflict")
	}
	// Check for duplicate label (different ID)
	if existingID, exists := m.labels[server.Label]; exists && existingID != server.ID {
		return fmt.Errorf("duplicate label")
	}
	delete(m.labels, existing.Label)
	server.UpdatedAt = time.Now()
	m.servers[server.ID] = server
	m.labels[server.Label] = server.ID
	return nil
}

func (m *mockMCPServerStore) Delete(_ context.Context, id uuid.UUID) error {
	s, ok := m.servers[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	delete(m.labels, s.Label)
	delete(m.servers, id)
	return nil
}

// --- MCP Server handler tests ---

func TestMCPServersHandler_Create(t *testing.T) {
	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid creation with bearer auth",
			body: map[string]interface{}{
				"label":              "my-mcp-server",
				"endpoint":          "https://mcp.example.com",
				"auth_type":         "bearer",
				"auth_credential":   "secret-token-123",
				"health_endpoint":   "https://mcp.example.com/health",
				"discovery_interval": "10m",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid creation with no auth",
			body: map[string]interface{}{
				"label":    "public-mcp",
				"endpoint": "https://public.example.com",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "missing label rejected",
			body: map[string]interface{}{
				"endpoint": "https://mcp.example.com",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "missing endpoint rejected",
			body: map[string]interface{}{
				"label": "no-endpoint",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "invalid auth_type rejected",
			body: map[string]interface{}{
				"label":     "bad-auth",
				"endpoint":  "https://mcp.example.com",
				"auth_type": "oauth2",
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpStore := newMockMCPServerStore()
			audit := &mockAuditStoreForAPI{}
			h := NewMCPServersHandler(mcpStore, audit, testEncKey)

			req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", tt.body)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			if tt.wantErr && env.Success {
				t.Fatal("expected success=false")
			}
			if !tt.wantErr {
				if !env.Success {
					t.Fatal("expected success=true")
				}
				// Verify auth_credential is NOT in the response
				data, _ := json.Marshal(env.Data)
				if bytes.Contains(data, []byte("auth_credential")) {
					t.Fatal("response must NOT contain auth_credential")
				}
			}
		})
	}
}

func TestMCPServersHandler_List_NoCredential(t *testing.T) {
	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	// Seed a server with a credential
	mcpStore.servers[uuid.New()] = &store.MCPServer{
		ID:             uuid.New(),
		Label:          "test-server",
		Endpoint:       "https://mcp.example.com",
		AuthType:       "bearer",
		AuthCredential: "encrypted-secret",
		IsEnabled:      true,
		CircuitBreaker: json.RawMessage(`{}`),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	req := adminRequest(http.MethodGet, "/api/v1/mcp-servers", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := parseEnvelope(t, w)
	if !env.Success {
		t.Fatal("expected success=true")
	}

	// Verify auth_credential is NOT in the response
	data, _ := json.Marshal(env.Data)
	if bytes.Contains(data, []byte("auth_credential")) {
		t.Fatal("list response must NOT contain auth_credential")
	}
}

func TestMCPServersHandler_Get(t *testing.T) {
	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	serverID := uuid.New()
	mcpStore.servers[serverID] = &store.MCPServer{
		ID:             serverID,
		Label:          "test-server",
		Endpoint:       "https://mcp.example.com",
		AuthType:       "none",
		IsEnabled:      true,
		CircuitBreaker: json.RawMessage(`{}`),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	tests := []struct {
		name       string
		serverID   string
		wantStatus int
	}{
		{
			name:       "existing server",
			serverID:   serverID.String(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent server",
			serverID:   uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			serverID:   "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodGet, "/api/v1/mcp-servers/"+tt.serverID, nil)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("serverId", tt.serverID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Get(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				data, _ := json.Marshal(parseEnvelope(t, w).Data)
				if bytes.Contains(data, []byte("auth_credential")) {
					t.Fatal("get response must NOT contain auth_credential")
				}
			}
		})
	}
}

func TestMCPServersHandler_Update(t *testing.T) {
	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	serverID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	mcpStore.servers[serverID] = &store.MCPServer{
		ID:                serverID,
		Label:             "test-server",
		Endpoint:          "https://mcp.example.com",
		AuthType:          "none",
		IsEnabled:         true,
		CircuitBreaker:    json.RawMessage(`{}`),
		DiscoveryInterval: "5m",
		CreatedAt:         time.Now(),
		UpdatedAt:         updatedAt,
	}
	mcpStore.labels["test-server"] = serverID

	tests := []struct {
		name       string
		serverID   string
		ifMatch    string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:     "valid update",
			serverID: serverID.String(),
			ifMatch:  updatedAt.UTC().Format(time.RFC3339Nano),
			body: map[string]interface{}{
				"label":    "updated-server",
				"endpoint": "https://new.example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing If-Match",
			serverID:   serverID.String(),
			ifMatch:    "",
			body:       map[string]interface{}{"label": "x"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-existent server",
			serverID:   uuid.New().String(),
			ifMatch:    time.Now().UTC().Format(time.RFC3339Nano),
			body:       map[string]interface{}{"label": "x"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodPut, "/api/v1/mcp-servers/"+tt.serverID, tt.body)
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("serverId", tt.serverID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Update(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestMCPServersHandler_Delete(t *testing.T) {
	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	serverID := uuid.New()
	mcpStore.servers[serverID] = &store.MCPServer{
		ID:       serverID,
		Label:    "to-delete",
		Endpoint: "https://mcp.example.com",
	}
	mcpStore.labels["to-delete"] = serverID

	tests := []struct {
		name       string
		serverID   string
		wantStatus int
	}{
		{
			name:       "delete existing",
			serverID:   serverID.String(),
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete non-existent",
			serverID:   uuid.New().String(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid UUID",
			serverID:   "bad-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := adminRequest(http.MethodDelete, "/api/v1/mcp-servers/"+tt.serverID, nil)
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("serverId", tt.serverID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Delete(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// --- Input validation gap tests ---
// These tests prove that the MCP server handler does NOT validate endpoints,
// circuit_breaker schema, or discovery_interval values.

func TestMCPServersHandler_Create_RejectsInternalEndpoint(t *testing.T) {
	t.Parallel()

	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	// SSRF: internal/cloud metadata endpoints must be rejected
	ssrfEndpoints := []struct {
		name     string
		endpoint string
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
			mcpStore := newMockMCPServerStore()
			audit := &mockAuditStoreForAPI{}
			h := NewMCPServersHandler(mcpStore, audit, testEncKey)

			body := map[string]interface{}{
				"label":    "ssrf-test-" + tt.name,
				"endpoint": tt.endpoint,
			}
			req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for SSRF endpoint %q, got %d; body: %s",
					tt.endpoint, w.Code, w.Body.String())
			}

			env := parseEnvelope(t, w)
			if env.Success {
				t.Fatal("expected success=false for internal endpoint")
			}
		})
	}
}

func TestMCPServersHandler_Create_RejectsInvalidScheme(t *testing.T) {
	t.Parallel()

	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	invalidSchemes := []struct {
		name     string
		endpoint string
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
			mcpStore := newMockMCPServerStore()
			audit := &mockAuditStoreForAPI{}
			h := NewMCPServersHandler(mcpStore, audit, testEncKey)

			body := map[string]interface{}{
				"label":    "scheme-test-" + tt.name,
				"endpoint": tt.endpoint,
			}
			req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid scheme %q, got %d; body: %s",
					tt.endpoint, w.Code, w.Body.String())
			}
		})
	}
}

func TestMCPServersHandler_Create_RejectsInvalidCircuitBreaker(t *testing.T) {
	t.Parallel()

	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	invalidCBs := []struct {
		name           string
		circuitBreaker interface{}
	}{
		{"arbitrary keys", map[string]interface{}{"evil": true}},
		{"missing fail_threshold", map[string]interface{}{"open_duration_s": 30}},
		{"missing open_duration_s", map[string]interface{}{"fail_threshold": 5}},
		{"negative fail_threshold", map[string]interface{}{"fail_threshold": -1, "open_duration_s": 30}},
		{"zero open_duration_s", map[string]interface{}{"fail_threshold": 5, "open_duration_s": 0}},
		{"string values", map[string]interface{}{"fail_threshold": "five", "open_duration_s": "thirty"}},
	}

	for _, tt := range invalidCBs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mcpStore := newMockMCPServerStore()
			audit := &mockAuditStoreForAPI{}
			h := NewMCPServersHandler(mcpStore, audit, testEncKey)

			body := map[string]interface{}{
				"label":           "cb-test-" + tt.name,
				"endpoint":        "https://valid.example.com",
				"circuit_breaker": tt.circuitBreaker,
			}
			req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid circuit_breaker %q, got %d; body: %s",
					tt.name, w.Code, w.Body.String())
			}
		})
	}
}

func TestMCPServersHandler_Create_RejectsInvalidDiscoveryInterval(t *testing.T) {
	t.Parallel()

	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	invalidIntervals := []struct {
		name     string
		interval string
	}{
		{"zero duration", "0s"},
		{"negative duration", "-5m"},
		{"not a duration", "never"},
		{"too short", "1s"},
	}

	for _, tt := range invalidIntervals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mcpStore := newMockMCPServerStore()
			audit := &mockAuditStoreForAPI{}
			h := NewMCPServersHandler(mcpStore, audit, testEncKey)

			body := map[string]interface{}{
				"label":              "interval-test-" + tt.name,
				"endpoint":           "https://valid.example.com",
				"discovery_interval": tt.interval,
			}
			req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
			w := httptest.NewRecorder()

			h.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid discovery_interval %q, got %d; body: %s",
					tt.interval, w.Code, w.Body.String())
			}
		})
	}
}

func TestMCPServersHandler_Create_RejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	// Create a circuit_breaker with 10KB of data
	largeData := make([]byte, 10*1024)
	for i := range largeData {
		largeData[i] = 'A'
	}
	body := map[string]interface{}{
		"label":    "oversized-cb",
		"endpoint": "https://valid.example.com",
		"circuit_breaker": map[string]interface{}{
			"fail_threshold": 5,
			"open_duration_s": 30,
			"padding": string(largeData),
		},
	}
	req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized circuit_breaker, got %d; body: %s",
			w.Code, w.Body.String())
	}
}

func TestMCPServersHandler_Update_ValidatesEndpoint(t *testing.T) {
	t.Parallel()

	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	serverID := uuid.New()
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	mcpStore.servers[serverID] = &store.MCPServer{
		ID:                serverID,
		Label:             "test-server",
		Endpoint:          "https://valid.example.com",
		AuthType:          "none",
		IsEnabled:         true,
		CircuitBreaker:    json.RawMessage(`{"fail_threshold":5,"open_duration_s":30}`),
		DiscoveryInterval: "5m",
		CreatedAt:         time.Now(),
		UpdatedAt:         updatedAt,
	}
	mcpStore.labels["test-server"] = serverID

	ssrfEndpoints := []struct {
		name     string
		endpoint string
	}{
		{"AWS metadata on update", "http://169.254.169.254/latest/meta-data/"},
		{"localhost on update", "http://localhost:8080/admin"},
		{"file scheme on update", "file:///etc/passwd"},
		{"private IP on update", "http://10.0.0.1/internal"},
	}

	for _, tt := range ssrfEndpoints {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]interface{}{
				"endpoint": tt.endpoint,
			}
			req := adminRequest(http.MethodPut, "/api/v1/mcp-servers/"+serverID.String(), body)
			req.Header.Set("If-Match", updatedAt.UTC().Format(time.RFC3339Nano))
			w := httptest.NewRecorder()

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("serverId", serverID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.Update(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for SSRF endpoint on update %q, got %d; body: %s",
					tt.endpoint, w.Code, w.Body.String())
			}
		})
	}
}

func TestMCPServersHandler_DuplicateLabel(t *testing.T) {
	testEncKey := make([]byte, 32)
	for i := range testEncKey {
		testEncKey[i] = byte(i)
	}

	mcpStore := newMockMCPServerStore()
	audit := &mockAuditStoreForAPI{}
	h := NewMCPServersHandler(mcpStore, audit, testEncKey)

	body := map[string]interface{}{
		"label":    "dup-label",
		"endpoint": "https://mcp.example.com",
	}

	// First create should succeed
	req := adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w.Code)
	}

	// Second create with same label should fail with 409
	req = adminRequest(http.MethodPost, "/api/v1/mcp-servers", body)
	w = httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate label: expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}
