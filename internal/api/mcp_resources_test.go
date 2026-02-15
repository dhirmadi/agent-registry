package api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Mock stores for MCP resource tests ---

type mockAgentStoreForMCP struct {
	agents []store.Agent
	byID   map[string]*store.Agent
	err    error
}

func (m *mockAgentStoreForMCP) GetByID(_ context.Context, id string) (*store.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	if a, ok := m.byID[id]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockAgentStoreForMCP) List(_ context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	result := m.agents
	if activeOnly {
		var filtered []store.Agent
		for _, a := range result {
			if a.IsActive {
				filtered = append(filtered, a)
			}
		}
		result = filtered
	}
	return result, len(result), nil
}

type mockPromptStoreForMCP struct {
	prompts map[string]*store.Prompt
	err     error
}

func (m *mockPromptStoreForMCP) GetActive(_ context.Context, agentID string) (*store.Prompt, error) {
	if m.err != nil {
		return nil, m.err
	}
	if p, ok := m.prompts[agentID]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("not found")
}

type mockModelConfigStoreForMCP struct {
	config *store.ModelConfig
	err    error
}

func (m *mockModelConfigStoreForMCP) GetByScope(_ context.Context, scope, scopeID string) (*store.ModelConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

// --- Tests ---

func TestMCPResourceProvider_ListTemplates(t *testing.T) {
	p := NewMCPResourceProvider(nil, nil, nil)
	templates := p.ListTemplates()

	if len(templates) != 4 {
		t.Fatalf("expected 4 templates, got %d", len(templates))
	}

	expectedURIs := []string{
		"agent://{agentId}",
		"prompt://{agentId}/active",
		"config://model",
		"config://context",
	}
	for i, expected := range expectedURIs {
		if templates[i].URITemplate != expected {
			t.Errorf("template[%d]: expected URI %q, got %q", i, expected, templates[i].URITemplate)
		}
	}

	// All templates should have non-empty Name and Description
	for i, tmpl := range templates {
		if tmpl.Name == "" {
			t.Errorf("template[%d]: Name is empty", i)
		}
		if tmpl.Description == "" {
			t.Errorf("template[%d]: Description is empty", i)
		}
		if tmpl.MimeType == "" {
			t.Errorf("template[%d]: MimeType is empty", i)
		}
	}
}

func TestMCPResourceProvider_ListResources(t *testing.T) {
	agents := []store.Agent{
		{ID: "code_agent", Name: "Code Agent", Description: "Writes code", IsActive: true},
		{ID: "test_agent", Name: "Test Agent", Description: "Tests code", IsActive: true},
	}
	agentStore := &mockAgentStoreForMCP{
		agents: agents,
		byID:   map[string]*store.Agent{},
	}
	p := NewMCPResourceProvider(agentStore, nil, nil)

	resources, err := p.ListResources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 agents * 2 (agent + prompt) + 2 config = 6
	if len(resources) != 6 {
		t.Fatalf("expected 6 resources, got %d", len(resources))
	}

	// Verify agent resources
	if resources[0].URI != "agent://code_agent" {
		t.Errorf("expected agent://code_agent, got %s", resources[0].URI)
	}
	if resources[1].URI != "prompt://code_agent/active" {
		t.Errorf("expected prompt://code_agent/active, got %s", resources[1].URI)
	}
	if resources[2].URI != "agent://test_agent" {
		t.Errorf("expected agent://test_agent, got %s", resources[2].URI)
	}
	if resources[3].URI != "prompt://test_agent/active" {
		t.Errorf("expected prompt://test_agent/active, got %s", resources[3].URI)
	}

	// Verify config resources
	if resources[4].URI != "config://model" {
		t.Errorf("expected config://model, got %s", resources[4].URI)
	}
	if resources[5].URI != "config://context" {
		t.Errorf("expected config://context, got %s", resources[5].URI)
	}
}

func TestMCPResourceProvider_ListResources_NoAgents(t *testing.T) {
	agentStore := &mockAgentStoreForMCP{agents: nil}
	p := NewMCPResourceProvider(agentStore, nil, nil)

	resources, err := p.ListResources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 0 agents + 2 config = 2
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
}

func TestMCPResourceProvider_ListResources_StoreError(t *testing.T) {
	agentStore := &mockAgentStoreForMCP{err: fmt.Errorf("db down")}
	p := NewMCPResourceProvider(agentStore, nil, nil)

	_, err := p.ListResources(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPResourceProvider_ReadAgent(t *testing.T) {
	agent := &store.Agent{
		ID:          "code_agent",
		Name:        "Code Agent",
		Description: "Writes code",
		IsActive:    true,
		Version:     3,
	}
	agentStore := &mockAgentStoreForMCP{
		byID: map[string]*store.Agent{"code_agent": agent},
	}
	p := NewMCPResourceProvider(agentStore, nil, nil)

	content, rpcErr := p.ReadResource(context.Background(), "agent://code_agent")
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	if content.URI != "agent://code_agent" {
		t.Errorf("expected URI agent://code_agent, got %s", content.URI)
	}
	if content.MimeType != "application/json" {
		t.Errorf("expected application/json, got %s", content.MimeType)
	}

	// Verify JSON contains agent data
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["id"] != "code_agent" {
		t.Errorf("expected id=code_agent, got %v", parsed["id"])
	}
}

func TestMCPResourceProvider_ReadAgent_NotFound(t *testing.T) {
	agentStore := &mockAgentStoreForMCP{byID: map[string]*store.Agent{}}
	p := NewMCPResourceProvider(agentStore, nil, nil)

	_, rpcErr := p.ReadResource(context.Background(), "agent://nonexistent")
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent agent")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected code -32602, got %d", rpcErr.Code)
	}
}

func TestMCPResourceProvider_ReadAgent_EmptyID(t *testing.T) {
	p := NewMCPResourceProvider(nil, nil, nil)

	_, rpcErr := p.ReadResource(context.Background(), "agent://")
	if rpcErr == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected code -32602, got %d", rpcErr.Code)
	}
}

func TestMCPResourceProvider_ReadPrompt(t *testing.T) {
	prompt := &store.Prompt{
		AgentID:      "code_agent",
		SystemPrompt: "You are a helpful coding assistant.",
		Version:      2,
		IsActive:     true,
	}
	promptStore := &mockPromptStoreForMCP{
		prompts: map[string]*store.Prompt{"code_agent": prompt},
	}
	p := NewMCPResourceProvider(nil, promptStore, nil)

	content, rpcErr := p.ReadResource(context.Background(), "prompt://code_agent/active")
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	if content.URI != "prompt://code_agent/active" {
		t.Errorf("expected URI prompt://code_agent/active, got %s", content.URI)
	}
	if content.MimeType != "text/plain" {
		t.Errorf("expected text/plain, got %s", content.MimeType)
	}
	if content.Text != "You are a helpful coding assistant." {
		t.Errorf("unexpected prompt text: %s", content.Text)
	}
}

func TestMCPResourceProvider_ReadPrompt_NotFound(t *testing.T) {
	promptStore := &mockPromptStoreForMCP{prompts: map[string]*store.Prompt{}}
	p := NewMCPResourceProvider(nil, promptStore, nil)

	_, rpcErr := p.ReadResource(context.Background(), "prompt://nonexistent/active")
	if rpcErr == nil {
		t.Fatal("expected error for nonexistent prompt")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("expected code -32602, got %d", rpcErr.Code)
	}
}

func TestMCPResourceProvider_ReadPrompt_InvalidURI(t *testing.T) {
	p := NewMCPResourceProvider(nil, nil, nil)

	tests := []struct {
		name string
		uri  string
	}{
		{"no path", "prompt://code_agent"},
		{"wrong suffix", "prompt://code_agent/latest"},
		{"empty agent", "prompt:///active"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, rpcErr := p.ReadResource(context.Background(), tc.uri)
			if rpcErr == nil {
				t.Fatal("expected error")
			}
			if rpcErr.Code != -32602 {
				t.Errorf("expected code -32602, got %d", rpcErr.Code)
			}
		})
	}
}

func TestMCPResourceProvider_ReadModelConfig(t *testing.T) {
	config := &store.ModelConfig{
		DefaultModel:           "claude-sonnet-4-5-20250929",
		Temperature:            0.7,
		MaxTokens:              4096,
		MaxToolRounds:          10,
		DefaultContextWindow:   128000,
		DefaultMaxOutputTokens: 8192,
		HistoryTokenBudget:     64000,
		MaxHistoryMessages:     50,
		EmbeddingModel:         "text-embedding-3-small",
	}
	modelStore := &mockModelConfigStoreForMCP{config: config}
	p := NewMCPResourceProvider(nil, nil, modelStore)

	content, rpcErr := p.ReadResource(context.Background(), "config://model")
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	if content.URI != "config://model" {
		t.Errorf("expected URI config://model, got %s", content.URI)
	}
	if content.MimeType != "application/json" {
		t.Errorf("expected application/json, got %s", content.MimeType)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["default_model"] != "claude-sonnet-4-5-20250929" {
		t.Errorf("unexpected default_model: %v", parsed["default_model"])
	}
}

func TestMCPResourceProvider_ReadModelConfig_Nil(t *testing.T) {
	modelStore := &mockModelConfigStoreForMCP{config: nil}
	p := NewMCPResourceProvider(nil, nil, modelStore)

	content, rpcErr := p.ReadResource(context.Background(), "config://model")
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	if content.Text != "{}" {
		t.Errorf("expected empty JSON, got %s", content.Text)
	}
}

func TestMCPResourceProvider_ReadModelConfig_StoreError(t *testing.T) {
	modelStore := &mockModelConfigStoreForMCP{err: fmt.Errorf("db error")}
	p := NewMCPResourceProvider(nil, nil, modelStore)

	_, rpcErr := p.ReadResource(context.Background(), "config://model")
	if rpcErr == nil {
		t.Fatal("expected error")
	}
	if rpcErr.Code != -32603 {
		t.Errorf("expected code -32603, got %d", rpcErr.Code)
	}
}

func TestMCPResourceProvider_ReadContextConfig(t *testing.T) {
	config := &store.ModelConfig{
		DefaultContextWindow:   128000,
		DefaultMaxOutputTokens: 8192,
		HistoryTokenBudget:     64000,
		MaxHistoryMessages:     50,
		DefaultModel:           "should-not-appear",
		Temperature:            0.7,
	}
	modelStore := &mockModelConfigStoreForMCP{config: config}
	p := NewMCPResourceProvider(nil, nil, modelStore)

	content, rpcErr := p.ReadResource(context.Background(), "config://context")
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	if content.URI != "config://context" {
		t.Errorf("expected URI config://context, got %s", content.URI)
	}

	var parsed contextConfig
	if err := json.Unmarshal([]byte(content.Text), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.DefaultContextWindow != 128000 {
		t.Errorf("expected DefaultContextWindow=128000, got %d", parsed.DefaultContextWindow)
	}
	if parsed.DefaultMaxOutputTokens != 8192 {
		t.Errorf("expected DefaultMaxOutputTokens=8192, got %d", parsed.DefaultMaxOutputTokens)
	}
	if parsed.HistoryTokenBudget != 64000 {
		t.Errorf("expected HistoryTokenBudget=64000, got %d", parsed.HistoryTokenBudget)
	}
	if parsed.MaxHistoryMessages != 50 {
		t.Errorf("expected MaxHistoryMessages=50, got %d", parsed.MaxHistoryMessages)
	}

	// Ensure model-specific fields are NOT in the context config
	raw := make(map[string]interface{})
	json.Unmarshal([]byte(content.Text), &raw)
	if _, exists := raw["default_model"]; exists {
		t.Error("context config should not contain default_model")
	}
	if _, exists := raw["temperature"]; exists {
		t.Error("context config should not contain temperature")
	}
}

func TestMCPResourceProvider_ReadContextConfig_Nil(t *testing.T) {
	modelStore := &mockModelConfigStoreForMCP{config: nil}
	p := NewMCPResourceProvider(nil, nil, modelStore)

	content, rpcErr := p.ReadResource(context.Background(), "config://context")
	if rpcErr != nil {
		t.Fatalf("unexpected error: %+v", rpcErr)
	}

	var parsed contextConfig
	if err := json.Unmarshal([]byte(content.Text), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// All zero-values
	if parsed.DefaultContextWindow != 0 {
		t.Errorf("expected 0, got %d", parsed.DefaultContextWindow)
	}
}

func TestMCPResourceProvider_ReadResource_UnsupportedURI(t *testing.T) {
	p := NewMCPResourceProvider(nil, nil, nil)

	tests := []string{
		"file:///etc/passwd",
		"http://example.com",
		"unknown://something",
		"",
	}

	for _, uri := range tests {
		t.Run(uri, func(t *testing.T) {
			_, rpcErr := p.ReadResource(context.Background(), uri)
			if rpcErr == nil {
				t.Fatal("expected error for unsupported URI")
			}
			if rpcErr.Code != -32602 {
				t.Errorf("expected code -32602, got %d", rpcErr.Code)
			}
		})
	}
}
