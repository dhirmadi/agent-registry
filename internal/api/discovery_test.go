package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock stores for discovery ---

type mockDiscoveryAgentStore struct {
	agents []store.Agent
	err    error
}

func (m *mockDiscoveryAgentStore) List(_ context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	filtered := []store.Agent{}
	for _, a := range m.agents {
		if !activeOnly || a.IsActive {
			filtered = append(filtered, a)
		}
	}
	total := len(filtered)
	if offset >= total {
		return []store.Agent{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (m *mockDiscoveryAgentStore) Create(_ context.Context, agent *store.Agent) error {
	return nil
}

func (m *mockDiscoveryAgentStore) GetByID(_ context.Context, id string) (*store.Agent, error) {
	return nil, nil
}

func (m *mockDiscoveryAgentStore) Update(_ context.Context, agent *store.Agent, updatedAt time.Time) error {
	return nil
}

func (m *mockDiscoveryAgentStore) Patch(_ context.Context, id string, fields map[string]interface{}, updatedAt time.Time, actor string) (*store.Agent, error) {
	return nil, nil
}

func (m *mockDiscoveryAgentStore) Delete(_ context.Context, id string) error {
	return nil
}

func (m *mockDiscoveryAgentStore) ListVersions(_ context.Context, agentID string, offset, limit int) ([]store.AgentVersion, int, error) {
	return nil, 0, nil
}

func (m *mockDiscoveryAgentStore) GetVersion(_ context.Context, agentID string, version int) (*store.AgentVersion, error) {
	return nil, nil
}

func (m *mockDiscoveryAgentStore) Rollback(_ context.Context, agentID string, targetVersion int, actor string) (*store.Agent, error) {
	return nil, nil
}

type mockDiscoveryMCPStore struct {
	servers []store.MCPServer
	err     error
}

func (m *mockDiscoveryMCPStore) List(_ context.Context) ([]store.MCPServer, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.servers, nil
}

func (m *mockDiscoveryMCPStore) Create(_ context.Context, server *store.MCPServer) error {
	return nil
}

func (m *mockDiscoveryMCPStore) GetByID(_ context.Context, id uuid.UUID) (*store.MCPServer, error) {
	return nil, nil
}

func (m *mockDiscoveryMCPStore) GetByLabel(_ context.Context, label string) (*store.MCPServer, error) {
	return nil, nil
}

func (m *mockDiscoveryMCPStore) Update(_ context.Context, server *store.MCPServer) error {
	return nil
}

func (m *mockDiscoveryMCPStore) Delete(_ context.Context, id uuid.UUID) error {
	return nil
}

type mockDiscoveryTrustStore struct {
	defaults []store.TrustDefault
	err      error
}

func (m *mockDiscoveryTrustStore) List(_ context.Context) ([]store.TrustDefault, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.defaults, nil
}

func (m *mockDiscoveryTrustStore) GetByID(_ context.Context, id uuid.UUID) (*store.TrustDefault, error) {
	return nil, nil
}

func (m *mockDiscoveryTrustStore) Update(_ context.Context, d *store.TrustDefault) error {
	return nil
}

type mockDiscoveryModelConfigStore struct {
	config *store.ModelConfig
	err    error
}

func (m *mockDiscoveryModelConfigStore) GetByScope(_ context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

func (m *mockDiscoveryModelConfigStore) GetMerged(_ context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	return nil, nil
}

func (m *mockDiscoveryModelConfigStore) Update(_ context.Context, config *store.ModelConfig, etag time.Time) error {
	return nil
}

func (m *mockDiscoveryModelConfigStore) Upsert(_ context.Context, config *store.ModelConfig) error {
	return nil
}

type mockDiscoveryModelEndpointStore struct {
	endpoints []store.ModelEndpoint
	versions  map[uuid.UUID]*store.ModelEndpointVersion
	err       error
}

func (m *mockDiscoveryModelEndpointStore) Create(_ context.Context, _ *store.ModelEndpoint, _ json.RawMessage, _ string) error {
	return nil
}

func (m *mockDiscoveryModelEndpointStore) GetBySlug(_ context.Context, _ string) (*store.ModelEndpoint, error) {
	return nil, nil
}

func (m *mockDiscoveryModelEndpointStore) List(_ context.Context, _ *string, _ bool, _, _ int) ([]store.ModelEndpoint, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.endpoints, len(m.endpoints), nil
}

func (m *mockDiscoveryModelEndpointStore) Update(_ context.Context, _ *store.ModelEndpoint, _ time.Time) error {
	return nil
}

func (m *mockDiscoveryModelEndpointStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockDiscoveryModelEndpointStore) CreateVersion(_ context.Context, _ uuid.UUID, _ json.RawMessage, _, _ string) (*store.ModelEndpointVersion, error) {
	return nil, nil
}

func (m *mockDiscoveryModelEndpointStore) ListVersions(_ context.Context, _ uuid.UUID, _, _ int) ([]store.ModelEndpointVersion, int, error) {
	return nil, 0, nil
}

func (m *mockDiscoveryModelEndpointStore) GetVersion(_ context.Context, _ uuid.UUID, _ int) (*store.ModelEndpointVersion, error) {
	return nil, nil
}

func (m *mockDiscoveryModelEndpointStore) GetActiveVersion(_ context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error) {
	if m.versions != nil {
		if v, ok := m.versions[endpointID]; ok {
			return v, nil
		}
	}
	return nil, fmt.Errorf("NOT_FOUND: no active version")
}

func (m *mockDiscoveryModelEndpointStore) ActivateVersion(_ context.Context, _ uuid.UUID, _ int) (*store.ModelEndpointVersion, error) {
	return nil, nil
}

func (m *mockDiscoveryModelEndpointStore) CountAll(_ context.Context) (int, error) {
	return len(m.endpoints), nil
}

// --- Tests ---

func TestDiscoveryHandler_GetDiscovery_Success(t *testing.T) {
	// Arrange: Create mock stores with sample data
	agentStore := &mockDiscoveryAgentStore{
		agents: []store.Agent{
			{
				ID:          "agent_foo",
				Name:        "Foo Agent",
				Description: "A test agent",
				IsActive:    true,
				Tools:       json.RawMessage(`[]`),
				Version:     1,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		},
	}

	mcpStore := &mockDiscoveryMCPStore{
		servers: []store.MCPServer{
			{
				ID:                uuid.New(),
				Label:             "mcp-test",
				Endpoint:          "https://mcp.example.com",
				AuthType:          "none",
				DiscoveryInterval: "5m",
				IsEnabled:         true,
				CircuitBreaker:    json.RawMessage(`{"fail_threshold":5,"open_duration_s":30}`),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			},
		},
	}

	trustStore := &mockDiscoveryTrustStore{
		defaults: []store.TrustDefault{
			{
				ID:        uuid.New(),
				Tier:      "filesystem",
				Patterns:  json.RawMessage(`[]`),
				Priority:  1,
				UpdatedAt: time.Now(),
			},
		},
	}

	modelConfigStore := &mockDiscoveryModelConfigStore{
		config: &store.ModelConfig{
			Scope:        "global",
			ScopeID:      "",
			DefaultModel: "gpt-4",
			Temperature:  0.7,
			MaxTokens:    2000,
			UpdatedAt:    time.Now(),
		},
	}

	epID := uuid.New()
	modelEndpointStore := &mockDiscoveryModelEndpointStore{
		endpoints: []store.ModelEndpoint{
			{
				ID:          epID,
				Slug:        "openai-gpt4o",
				Name:        "GPT-4o Production",
				Provider:    "openai",
				EndpointURL: "https://api.openai.com/v1",
				ModelName:   "gpt-4o",
				IsActive:    true,
			},
		},
		versions: map[uuid.UUID]*store.ModelEndpointVersion{
			epID: {
				ID:         uuid.New(),
				EndpointID: epID,
				Version:    1,
				Config:     json.RawMessage(`{"temperature":0.3,"max_tokens":4096}`),
				IsActive:   true,
				CreatedAt:  time.Now(),
			},
		},
	}

	handler := NewDiscoveryHandler(
		agentStore,
		mcpStore,
		trustStore,
		modelConfigStore,
		modelEndpointStore,
	)

	// Act: Make request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	// Assert: Check response
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !env.Success {
		t.Fatalf("expected success=true, got false")
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object")
	}

	// Check all required fields
	if _, ok := data["agents"]; !ok {
		t.Errorf("missing agents field")
	}
	if _, ok := data["mcp_servers"]; !ok {
		t.Errorf("missing mcp_servers field")
	}
	if _, ok := data["trust_defaults"]; !ok {
		t.Errorf("missing trust_defaults field")
	}
	if _, ok := data["model_config"]; !ok {
		t.Errorf("missing model_config field")
	}
	if _, ok := data["fetched_at"]; !ok {
		t.Errorf("missing fetched_at field")
	}
	if _, ok := data["model_endpoints"]; !ok {
		t.Errorf("missing model_endpoints field")
	}

	// Validate agents array
	agents, ok := data["agents"].([]interface{})
	if !ok {
		t.Fatalf("expected agents to be an array")
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}

	// Validate model_endpoints array
	modelEps, ok := data["model_endpoints"].([]interface{})
	if !ok {
		t.Fatalf("expected model_endpoints to be an array")
	}
	if len(modelEps) != 1 {
		t.Errorf("expected 1 model endpoint, got %d", len(modelEps))
	}
	if len(modelEps) > 0 {
		ep := modelEps[0].(map[string]interface{})
		if ep["slug"] != "openai-gpt4o" {
			t.Errorf("expected slug 'openai-gpt4o', got %v", ep["slug"])
		}
		if ep["active_version"] != float64(1) {
			t.Errorf("expected active_version 1, got %v", ep["active_version"])
		}
	}
}

func TestDiscoveryHandler_GetDiscovery_AgentStoreFailure(t *testing.T) {
	agentStore := &mockDiscoveryAgentStore{
		err: fmt.Errorf("database error"),
	}

	handler := NewDiscoveryHandler(
		agentStore,
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryModelEndpointStore{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestDiscoveryHandler_GetDiscovery_EmptyState(t *testing.T) {
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryModelEndpointStore{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !env.Success {
		t.Fatalf("expected success=true")
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object")
	}

	agents, _ := data["agents"].([]interface{})
	if len(agents) != 0 {
		t.Errorf("expected empty agents array")
	}

	modelConfig, ok := data["model_config"].(map[string]interface{})
	if !ok {
		t.Errorf("expected model_config to be an object")
	}
	if len(modelConfig) != 0 {
		t.Errorf("expected empty model_config object when nil")
	}
}

func TestDiscoveryHandler_GetDiscovery_MCPStoreFailure(t *testing.T) {
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{err: fmt.Errorf("mcp error")},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryModelEndpointStore{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestDiscoveryHandler_GetDiscovery_ModelConfigStoreFailure(t *testing.T) {
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{err: fmt.Errorf("model config error")},
		&mockDiscoveryModelEndpointStore{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}
