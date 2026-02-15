package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock stores for MCP tool tests ---

type mockAgentStoreForMCPTools struct {
	listFn    func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error)
	getByIDFn func(ctx context.Context, id string) (*store.Agent, error)
}

func (m *mockAgentStoreForMCPTools) Create(ctx context.Context, agent *store.Agent) error {
	return nil
}
func (m *mockAgentStoreForMCPTools) GetByID(ctx context.Context, id string) (*store.Agent, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockAgentStoreForMCPTools) List(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
	if m.listFn != nil {
		return m.listFn(ctx, activeOnly, offset, limit)
	}
	return nil, 0, nil
}
func (m *mockAgentStoreForMCPTools) Update(ctx context.Context, agent *store.Agent, updatedAt time.Time) error {
	return nil
}
func (m *mockAgentStoreForMCPTools) Patch(ctx context.Context, id string, fields map[string]interface{}, updatedAt time.Time, actor string) (*store.Agent, error) {
	return nil, nil
}
func (m *mockAgentStoreForMCPTools) Delete(ctx context.Context, id string) error { return nil }
func (m *mockAgentStoreForMCPTools) ListVersions(ctx context.Context, agentID string, offset, limit int) ([]store.AgentVersion, int, error) {
	return nil, 0, nil
}
func (m *mockAgentStoreForMCPTools) GetVersion(ctx context.Context, agentID string, version int) (*store.AgentVersion, error) {
	return nil, nil
}
func (m *mockAgentStoreForMCPTools) Rollback(ctx context.Context, agentID string, targetVersion int, actor string) (*store.Agent, error) {
	return nil, nil
}

type mockPromptStoreForMCPTools struct {
	getActiveFn func(ctx context.Context, agentID string) (*store.Prompt, error)
}

func (m *mockPromptStoreForMCPTools) List(ctx context.Context, agentID string, activeOnly bool, offset, limit int) ([]store.Prompt, int, error) {
	return nil, 0, nil
}
func (m *mockPromptStoreForMCPTools) GetActive(ctx context.Context, agentID string) (*store.Prompt, error) {
	if m.getActiveFn != nil {
		return m.getActiveFn(ctx, agentID)
	}
	return nil, errors.New("not found")
}
func (m *mockPromptStoreForMCPTools) GetByID(ctx context.Context, id uuid.UUID) (*store.Prompt, error) {
	return nil, nil
}
func (m *mockPromptStoreForMCPTools) Create(ctx context.Context, prompt *store.Prompt) error {
	return nil
}
func (m *mockPromptStoreForMCPTools) Activate(ctx context.Context, id uuid.UUID) (*store.Prompt, error) {
	return nil, nil
}
func (m *mockPromptStoreForMCPTools) Rollback(ctx context.Context, agentID string, targetVersion int, actor string) (*store.Prompt, error) {
	return nil, nil
}

type mockMCPServerStoreForMCPTools struct {
	listFn func(ctx context.Context) ([]store.MCPServer, error)
}

func (m *mockMCPServerStoreForMCPTools) Create(ctx context.Context, server *store.MCPServer) error {
	return nil
}
func (m *mockMCPServerStoreForMCPTools) GetByID(ctx context.Context, id uuid.UUID) (*store.MCPServer, error) {
	return nil, nil
}
func (m *mockMCPServerStoreForMCPTools) GetByLabel(ctx context.Context, label string) (*store.MCPServer, error) {
	return nil, nil
}
func (m *mockMCPServerStoreForMCPTools) List(ctx context.Context) ([]store.MCPServer, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}
func (m *mockMCPServerStoreForMCPTools) Update(ctx context.Context, server *store.MCPServer) error {
	return nil
}
func (m *mockMCPServerStoreForMCPTools) Delete(ctx context.Context, id uuid.UUID) error { return nil }

type mockModelConfigStoreForMCPTools struct {
	getByScopeFn func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error)
}

func (m *mockModelConfigStoreForMCPTools) GetByScope(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	if m.getByScopeFn != nil {
		return m.getByScopeFn(ctx, scope, scopeID)
	}
	return nil, nil
}
func (m *mockModelConfigStoreForMCPTools) GetMerged(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	return nil, nil
}
func (m *mockModelConfigStoreForMCPTools) Update(ctx context.Context, config *store.ModelConfig, etag time.Time) error {
	return nil
}
func (m *mockModelConfigStoreForMCPTools) Upsert(ctx context.Context, config *store.ModelConfig) error {
	return nil
}

type mockModelEndpointStoreForMCPTools struct {
	listFn             func(ctx context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error)
	getActiveVersionFn func(ctx context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error)
}

func (m *mockModelEndpointStoreForMCPTools) Create(ctx context.Context, ep *store.ModelEndpoint, initialConfig json.RawMessage, changeNote string) error {
	return nil
}
func (m *mockModelEndpointStoreForMCPTools) GetBySlug(ctx context.Context, slug string) (*store.ModelEndpoint, error) {
	return nil, nil
}
func (m *mockModelEndpointStoreForMCPTools) List(ctx context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error) {
	if m.listFn != nil {
		return m.listFn(ctx, workspaceID, activeOnly, offset, limit)
	}
	return nil, 0, nil
}
func (m *mockModelEndpointStoreForMCPTools) Update(ctx context.Context, ep *store.ModelEndpoint, updatedAt time.Time) error {
	return nil
}
func (m *mockModelEndpointStoreForMCPTools) Delete(ctx context.Context, slug string) error {
	return nil
}
func (m *mockModelEndpointStoreForMCPTools) CreateVersion(ctx context.Context, endpointID uuid.UUID, config json.RawMessage, changeNote, createdBy string) (*store.ModelEndpointVersion, error) {
	return nil, nil
}
func (m *mockModelEndpointStoreForMCPTools) ListVersions(ctx context.Context, endpointID uuid.UUID, offset, limit int) ([]store.ModelEndpointVersion, int, error) {
	return nil, 0, nil
}
func (m *mockModelEndpointStoreForMCPTools) GetVersion(ctx context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error) {
	return nil, nil
}
func (m *mockModelEndpointStoreForMCPTools) GetActiveVersion(ctx context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error) {
	if m.getActiveVersionFn != nil {
		return m.getActiveVersionFn(ctx, endpointID)
	}
	return nil, nil
}
func (m *mockModelEndpointStoreForMCPTools) ActivateVersion(ctx context.Context, endpointID uuid.UUID, version int) (*store.ModelEndpointVersion, error) {
	return nil, nil
}
func (m *mockModelEndpointStoreForMCPTools) CountAll(ctx context.Context) (int, error) {
	return 0, nil
}

// --- Helper to build executor with defaults ---

func newTestMCPToolExecutor() (*MCPToolExecutor, *mockAgentStoreForMCPTools, *mockPromptStoreForMCPTools, *mockMCPServerStoreForMCPTools, *mockModelConfigStoreForMCPTools, *mockModelEndpointStoreForMCPTools) {
	agents := &mockAgentStoreForMCPTools{}
	prompts := &mockPromptStoreForMCPTools{}
	mcp := &mockMCPServerStoreForMCPTools{}
	model := &mockModelConfigStoreForMCPTools{}
	endpoints := &mockModelEndpointStoreForMCPTools{}

	exec := NewMCPToolExecutor(agents, prompts, mcp, model, endpoints, "https://registry.example.com")
	return exec, agents, prompts, mcp, model, endpoints
}

// --- Tests ---

func TestMCPToolExecutor_ListTools(t *testing.T) {
	exec, _, _, _, _, _ := newTestMCPToolExecutor()
	tools := exec.ListTools()

	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}

	expected := map[string]bool{
		"list_agents":      false,
		"get_agent":        false,
		"get_discovery":    false,
		"list_mcp_servers": false,
		"get_model_config": false,
	}

	for _, tool := range tools {
		if _, ok := expected[tool.Name]; !ok {
			t.Errorf("unexpected tool: %s", tool.Name)
		}
		expected[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %s has nil input schema", tool.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("tool %s not found in list", name)
		}
	}
}

func TestMCPToolExecutor_ListTools_InputSchemas(t *testing.T) {
	exec, _, _, _, _, _ := newTestMCPToolExecutor()
	tools := exec.ListTools()

	for _, tool := range tools {
		// Verify each schema is valid JSON
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Errorf("tool %s has invalid JSON schema: %v", tool.Name, err)
		}

		// All should be type=object
		if schema["type"] != "object" {
			t.Errorf("tool %s schema type is %v, expected object", tool.Name, schema["type"])
		}
	}
}

func TestMCPToolExecutor_CallTool_UnknownTool(t *testing.T) {
	exec, _, _, _, _, _ := newTestMCPToolExecutor()
	_, rpcErr := exec.CallTool(context.Background(), "nonexistent", json.RawMessage(`{}`))

	if rpcErr == nil {
		t.Fatal("expected error for unknown tool")
	}
	if rpcErr.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", rpcErr.Code)
	}
}

func TestMCPToolExecutor_CallTool_InvalidJSON(t *testing.T) {
	exec, _, _, _, _, _ := newTestMCPToolExecutor()
	_, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{invalid`))

	if rpcErr == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected error code -32602 (InvalidParams), got %d", rpcErr.Code)
	}
}

// --- list_agents tests ---

func TestMCPToolExecutor_ListAgents_Default(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		if !activeOnly {
			t.Error("expected activeOnly=true by default")
		}
		if limit != 100 {
			t.Errorf("expected default limit=100, got %d", limit)
		}
		return []store.Agent{
			{ID: "agent1", Name: "Agent One", IsActive: true, Version: 1, Tools: json.RawMessage(`[]`), TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`[]`)},
			{ID: "agent2", Name: "Agent Two", IsActive: true, Version: 2, Tools: json.RawMessage(`[]`), TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`[]`)},
		}, 2, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected content type 'text', got %s", result.Content[0].Type)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &data); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	agentList, ok := data["agents"].([]interface{})
	if !ok {
		t.Fatal("expected 'agents' array in response")
	}
	if len(agentList) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agentList))
	}
}

func TestMCPToolExecutor_ListAgents_WithParams(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		if activeOnly {
			t.Error("expected activeOnly=false")
		}
		if limit != 5 {
			t.Errorf("expected limit=5, got %d", limit)
		}
		return []store.Agent{}, 0, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{"active_only": false, "limit": 5}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected isError=false")
	}
}

func TestMCPToolExecutor_ListAgents_LimitClamped(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		if limit != 1000 {
			t.Errorf("expected limit clamped to 1000, got %d", limit)
		}
		return []store.Agent{}, 0, nil
	}

	_, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{"limit": 9999}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
}

func TestMCPToolExecutor_ListAgents_StoreError(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		return nil, 0, errors.New("db connection lost")
	}

	result, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("store errors should not be JSON-RPC errors, got: %v", rpcErr)
	}
	if !result.IsError {
		t.Error("expected isError=true for store failure")
	}
}

// --- get_agent tests ---

func TestMCPToolExecutor_GetAgent_Valid(t *testing.T) {
	exec, agents, prompts, _, _, _ := newTestMCPToolExecutor()
	agents.getByIDFn = func(ctx context.Context, id string) (*store.Agent, error) {
		if id != "test_agent" {
			t.Errorf("expected id 'test_agent', got '%s'", id)
		}
		return &store.Agent{
			ID: "test_agent", Name: "Test Agent", IsActive: true, Version: 3,
			Tools: json.RawMessage(`[{"name":"search","source":"internal"}]`),
			TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`[]`),
		}, nil
	}
	prompts.getActiveFn = func(ctx context.Context, agentID string) (*store.Prompt, error) {
		return &store.Prompt{
			ID:           uuid.New(),
			AgentID:      "test_agent",
			SystemPrompt: "You are a helpful assistant",
			IsActive:     true,
			Version:      1,
		}, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_agent", json.RawMessage(`{"agent_id": "test_agent"}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["id"] != "test_agent" {
		t.Errorf("expected agent id 'test_agent', got %v", data["id"])
	}
	if data["active_prompt"] == nil {
		t.Error("expected active_prompt in response")
	}
}

func TestMCPToolExecutor_GetAgent_MissingID(t *testing.T) {
	exec, _, _, _, _, _ := newTestMCPToolExecutor()
	_, rpcErr := exec.CallTool(context.Background(), "get_agent", json.RawMessage(`{}`))

	if rpcErr == nil {
		t.Fatal("expected error for missing agent_id")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected InvalidParams error code -32602, got %d", rpcErr.Code)
	}
}

func TestMCPToolExecutor_GetAgent_NotFound(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.getByIDFn = func(ctx context.Context, id string) (*store.Agent, error) {
		return nil, errors.New("not found")
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_agent", json.RawMessage(`{"agent_id": "missing"}`))
	if rpcErr != nil {
		t.Fatalf("store errors should not be JSON-RPC errors: %v", rpcErr)
	}
	if !result.IsError {
		t.Error("expected isError=true for not found")
	}
}

func TestMCPToolExecutor_GetAgent_NoActivePrompt(t *testing.T) {
	exec, agents, prompts, _, _, _ := newTestMCPToolExecutor()
	agents.getByIDFn = func(ctx context.Context, id string) (*store.Agent, error) {
		return &store.Agent{
			ID: "test_agent", Name: "Test", IsActive: true, Version: 1,
			Tools: json.RawMessage(`[]`), TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`[]`),
		}, nil
	}
	prompts.getActiveFn = func(ctx context.Context, agentID string) (*store.Prompt, error) {
		return nil, errors.New("no active prompt")
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_agent", json.RawMessage(`{"agent_id": "test_agent"}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("should succeed even without active prompt")
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result.Content[0].Text), &data)
	if data["active_prompt"] != nil {
		t.Error("expected active_prompt to be nil when no prompt exists")
	}
}

// --- get_discovery tests ---

func TestMCPToolExecutor_GetDiscovery(t *testing.T) {
	exec, agents, _, mcpStore, modelStore, endpointStore := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		return []store.Agent{
			{ID: "a1", Name: "Agent 1", IsActive: true, Version: 1, Tools: json.RawMessage(`[]`), TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`[]`)},
		}, 1, nil
	}
	mcpStore.listFn = func(ctx context.Context) ([]store.MCPServer, error) {
		return []store.MCPServer{
			{ID: uuid.New(), Label: "mcp1", Endpoint: "https://mcp.example.com", AuthType: "bearer"},
		}, nil
	}
	modelStore.getByScopeFn = func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
		return &store.ModelConfig{DefaultModel: "gpt-4"}, nil
	}
	epID := uuid.New()
	endpointStore.listFn = func(ctx context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error) {
		return []store.ModelEndpoint{
			{ID: epID, Slug: "openai-prod", Name: "OpenAI Prod", Provider: "openai", EndpointURL: "https://api.openai.com", IsActive: true},
		}, 1, nil
	}
	endpointStore.getActiveVersionFn = func(ctx context.Context, endpointID uuid.UUID) (*store.ModelEndpointVersion, error) {
		return &store.ModelEndpointVersion{Version: 1, Config: json.RawMessage(`{"temperature":0.7}`)}, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_discovery", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify all sections present
	for _, key := range []string{"agents", "mcp_servers", "model_config", "model_endpoints", "fetched_at"} {
		if _, ok := data[key]; !ok {
			t.Errorf("expected key %s in discovery response", key)
		}
	}
}

func TestMCPToolExecutor_GetDiscovery_StoreError(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		return nil, 0, errors.New("db error")
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_discovery", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("store errors should be tool errors, not JSON-RPC errors: %v", rpcErr)
	}
	if !result.IsError {
		t.Error("expected isError=true for store failure")
	}
}

// --- list_mcp_servers tests ---

func TestMCPToolExecutor_ListMCPServers(t *testing.T) {
	exec, _, _, mcpStore, _, _ := newTestMCPToolExecutor()
	mcpStore.listFn = func(ctx context.Context) ([]store.MCPServer, error) {
		return []store.MCPServer{
			{
				ID: uuid.New(), Label: "git-server", Endpoint: "https://mcp-git.example.com",
				AuthType: "bearer", AuthCredential: "encrypted-secret-stuff",
				IsEnabled: true, CircuitBreaker: json.RawMessage(`{"fail_threshold":5,"open_duration_s":30}`),
			},
		}, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "list_mcp_servers", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result.Content[0].Text), &data)
	servers := data["servers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	// Verify credentials are stripped
	server := servers[0].(map[string]interface{})
	if _, hasCredential := server["auth_credential"]; hasCredential {
		t.Error("auth_credential should be stripped from MCP server response")
	}
}

func TestMCPToolExecutor_ListMCPServers_StoreError(t *testing.T) {
	exec, _, _, mcpStore, _, _ := newTestMCPToolExecutor()
	mcpStore.listFn = func(ctx context.Context) ([]store.MCPServer, error) {
		return nil, errors.New("db down")
	}

	result, rpcErr := exec.CallTool(context.Background(), "list_mcp_servers", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("store errors should be tool errors: %v", rpcErr)
	}
	if !result.IsError {
		t.Error("expected isError=true")
	}
}

// --- get_model_config tests ---

func TestMCPToolExecutor_GetModelConfig_GlobalDefault(t *testing.T) {
	exec, _, _, _, modelStore, _ := newTestMCPToolExecutor()
	modelStore.getByScopeFn = func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
		if scope != "global" || scopeID != "" {
			t.Errorf("expected scope=global, scopeID='', got scope=%s, scopeID=%s", scope, scopeID)
		}
		return &store.ModelConfig{
			DefaultModel: "gpt-4",
			Temperature:  0.7,
			MaxTokens:    4096,
		}, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_model_config", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error")
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result.Content[0].Text), &data)
	if data["default_model"] != "gpt-4" {
		t.Errorf("expected default_model=gpt-4, got %v", data["default_model"])
	}
}

func TestMCPToolExecutor_GetModelConfig_WithScope(t *testing.T) {
	exec, _, _, _, modelStore, _ := newTestMCPToolExecutor()
	modelStore.getByScopeFn = func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
		if scope != "workspace" || scopeID != "ws-123" {
			t.Errorf("expected scope=workspace, scopeID=ws-123, got scope=%s, scopeID=%s", scope, scopeID)
		}
		return &store.ModelConfig{
			DefaultModel: "claude-3",
			Temperature:  0.5,
		}, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_model_config", json.RawMessage(`{"scope": "workspace", "workspace_id": "ws-123"}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error")
	}
}

func TestMCPToolExecutor_GetModelConfig_NilResult(t *testing.T) {
	exec, _, _, _, modelStore, _ := newTestMCPToolExecutor()
	modelStore.getByScopeFn = func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
		return nil, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_model_config", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error for nil config (returns empty object)")
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result.Content[0].Text), &data)
	// Should return empty object or empty JSON
	if len(data) != 0 {
		t.Errorf("expected empty object for nil config, got %v", data)
	}
}

func TestMCPToolExecutor_GetModelConfig_StoreError(t *testing.T) {
	exec, _, _, _, modelStore, _ := newTestMCPToolExecutor()
	modelStore.getByScopeFn = func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
		return nil, errors.New("db error")
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_model_config", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("store errors should be tool errors: %v", rpcErr)
	}
	if !result.IsError {
		t.Error("expected isError=true")
	}
}

// --- Edge case tests ---

func TestMCPToolExecutor_CallTool_NilArgs(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		return []store.Agent{}, 0, nil
	}

	// nil args should be treated as empty object
	result, rpcErr := exec.CallTool(context.Background(), "list_agents", nil)
	if rpcErr != nil {
		t.Fatalf("nil args should be handled: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error for nil args")
	}
}

func TestMCPToolExecutor_CallTool_EmptyArgs(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		return []store.Agent{}, 0, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("empty args should work: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error for empty args")
	}
}

func TestMCPToolExecutor_GetDiscovery_EmptyStores(t *testing.T) {
	exec, agents, _, mcpStore, modelStore, endpointStore := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		return []store.Agent{}, 0, nil
	}
	mcpStore.listFn = func(ctx context.Context) ([]store.MCPServer, error) {
		return []store.MCPServer{}, nil
	}
	modelStore.getByScopeFn = func(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error) {
		return nil, nil
	}
	endpointStore.listFn = func(ctx context.Context, workspaceID *string, activeOnly bool, offset, limit int) ([]store.ModelEndpoint, int, error) {
		return []store.ModelEndpoint{}, 0, nil
	}

	result, rpcErr := exec.CallTool(context.Background(), "get_discovery", json.RawMessage(`{}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
	if result.IsError {
		t.Error("expected no error for empty stores")
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result.Content[0].Text), &data)
	// All arrays should be empty, not nil
	agentsList := data["agents"].([]interface{})
	if len(agentsList) != 0 {
		t.Error("expected empty agents array")
	}
}

func TestMCPToolExecutor_ListAgents_NegativeLimit(t *testing.T) {
	exec, agents, _, _, _, _ := newTestMCPToolExecutor()
	agents.listFn = func(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
		if limit != 100 {
			t.Errorf("negative limit should use default 100, got %d", limit)
		}
		return []store.Agent{}, 0, nil
	}

	_, rpcErr := exec.CallTool(context.Background(), "list_agents", json.RawMessage(`{"limit": -5}`))
	if rpcErr != nil {
		t.Fatalf("unexpected error: %v", rpcErr)
	}
}
