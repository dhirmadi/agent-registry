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

type mockDiscoveryContextConfigStore struct {
	config *store.ContextConfig
	err    error
}

func (m *mockDiscoveryContextConfigStore) GetByScope(_ context.Context, scope, scopeID string) (*store.ContextConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

func (m *mockDiscoveryContextConfigStore) GetMerged(_ context.Context, scope, scopeID string) (*store.ContextConfig, error) {
	return nil, nil
}

func (m *mockDiscoveryContextConfigStore) Update(_ context.Context, config *store.ContextConfig, etag time.Time) error {
	return nil
}

func (m *mockDiscoveryContextConfigStore) Upsert(_ context.Context, config *store.ContextConfig) error {
	return nil
}

type mockDiscoverySignalConfigStore struct {
	configs []store.SignalConfig
	err     error
}

func (m *mockDiscoverySignalConfigStore) List(_ context.Context) ([]store.SignalConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.configs, nil
}

func (m *mockDiscoverySignalConfigStore) GetByID(_ context.Context, id uuid.UUID) (*store.SignalConfig, error) {
	return nil, nil
}

func (m *mockDiscoverySignalConfigStore) Update(_ context.Context, config *store.SignalConfig, etag time.Time) error {
	return nil
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

	contextConfigStore := &mockDiscoveryContextConfigStore{
		config: &store.ContextConfig{
			Scope:          "global",
			ScopeID:        "",
			MaxTotalTokens: 8000,
			LayerBudgets:   json.RawMessage(`{"system":2000,"user":6000}`),
			EnabledLayers:  json.RawMessage(`["system","user"]`),
			UpdatedAt:      time.Now(),
		},
	}

	signalConfigStore := &mockDiscoverySignalConfigStore{
		configs: []store.SignalConfig{
			{
				ID:           uuid.New(),
				Source:       "conversation_tone",
				PollInterval: "60s",
				IsEnabled:    true,
				UpdatedAt:    time.Now(),
			},
		},
	}

	handler := NewDiscoveryHandler(
		agentStore,
		mcpStore,
		trustStore,
		modelConfigStore,
		contextConfigStore,
		signalConfigStore,
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
	if _, ok := data["context_config"]; !ok {
		t.Errorf("missing context_config field")
	}
	if _, ok := data["signal_config"]; !ok {
		t.Errorf("missing signal_config field")
	}
	if _, ok := data["fetched_at"]; !ok {
		t.Errorf("missing fetched_at field")
	}

	// Validate agents array
	agents, ok := data["agents"].([]interface{})
	if !ok {
		t.Fatalf("expected agents to be an array")
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestDiscoveryHandler_GetDiscovery_AgentStoreFailure(t *testing.T) {
	// Arrange: Agent store returns error
	agentStore := &mockDiscoveryAgentStore{
		err: fmt.Errorf("database error"),
	}

	handler := NewDiscoveryHandler(
		agentStore,
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryContextConfigStore{config: nil},
		&mockDiscoverySignalConfigStore{configs: []store.SignalConfig{}},
	)

	// Act
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	// Assert
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if env.Success {
		t.Errorf("expected success=false")
	}
}

func TestDiscoveryHandler_GetDiscovery_EmptyState(t *testing.T) {
	// Arrange: All stores return empty/nil results
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryContextConfigStore{config: nil},
		&mockDiscoverySignalConfigStore{configs: []store.SignalConfig{}},
	)

	// Act
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	// Assert
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

	// Verify empty arrays and empty objects for nil configs
	agents, _ := data["agents"].([]interface{})
	if len(agents) != 0 {
		t.Errorf("expected empty agents array")
	}

	// Model config and context config should be empty objects when nil
	modelConfig, ok := data["model_config"].(map[string]interface{})
	if !ok {
		t.Errorf("expected model_config to be an object")
	}
	if len(modelConfig) != 0 {
		t.Errorf("expected empty model_config object when nil")
	}
}

func TestDiscoveryHandler_GetDiscovery_MCPStoreFailure(t *testing.T) {
	// Arrange: MCP store returns error
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{err: fmt.Errorf("mcp error")},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryContextConfigStore{config: nil},
		&mockDiscoverySignalConfigStore{configs: []store.SignalConfig{}},
	)

	// Act
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	// Assert
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestDiscoveryHandler_GetDiscovery_ModelConfigStoreFailure(t *testing.T) {
	// Arrange: Model config store returns error
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{err: fmt.Errorf("model config error")},
		&mockDiscoveryContextConfigStore{config: nil},
		&mockDiscoverySignalConfigStore{configs: []store.SignalConfig{}},
	)

	// Act
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	// Assert
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestDiscoveryHandler_GetDiscovery_SignalConfigStoreFailure(t *testing.T) {
	// Arrange: Signal config store returns error
	handler := NewDiscoveryHandler(
		&mockDiscoveryAgentStore{agents: []store.Agent{}},
		&mockDiscoveryMCPStore{servers: []store.MCPServer{}},
		&mockDiscoveryTrustStore{defaults: []store.TrustDefault{}},
		&mockDiscoveryModelConfigStore{config: nil},
		&mockDiscoveryContextConfigStore{config: nil},
		&mockDiscoverySignalConfigStore{err: fmt.Errorf("signal config error")},
	)

	// Act
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
	ctx := auth.ContextWithUser(req.Context(), uuid.New(), "admin", "session")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.GetDiscovery(rec, req)

	// Assert
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}
