package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// Types MCPResource, MCPResourceTemplate, MCPResourceContent, MCPJSONRPCError
// are defined in mcp_types.go (shared across all MCP files).

// --- Store interfaces for MCP resource provider ---

// AgentStoreForMCP is the minimal agent store interface needed by the resource provider.
type AgentStoreForMCP interface {
	GetByID(ctx context.Context, id string) (*store.Agent, error)
	List(ctx context.Context, activeOnly bool, offset, limit int) ([]store.Agent, int, error)
}

// PromptStoreForMCP is the minimal prompt store interface needed by the resource provider.
type PromptStoreForMCP interface {
	GetActive(ctx context.Context, agentID string) (*store.Prompt, error)
}

// ModelConfigStoreForMCP is the minimal model config interface needed by the resource provider.
type ModelConfigStoreForMCP interface {
	GetByScope(ctx context.Context, scope, scopeID string) (*store.ModelConfig, error)
}

// MCPResourceProvider implements MCP resources/list and resources/read.
type MCPResourceProvider struct {
	agents  AgentStoreForMCP
	prompts PromptStoreForMCP
	model   ModelConfigStoreForMCP
}

// NewMCPResourceProvider creates a new MCPResourceProvider.
func NewMCPResourceProvider(agents AgentStoreForMCP, prompts PromptStoreForMCP, model ModelConfigStoreForMCP) *MCPResourceProvider {
	return &MCPResourceProvider{
		agents:  agents,
		prompts: prompts,
		model:   model,
	}
}

// ListTemplates returns the URI templates this provider supports.
func (p *MCPResourceProvider) ListTemplates() []MCPResourceTemplate {
	return []MCPResourceTemplate{
		{
			URITemplate: "agent://{agentId}",
			Name:        "Agent Definition",
			Description: "Full agent configuration including tools, system prompt, and metadata",
			MimeType:    "application/json",
		},
		{
			URITemplate: "prompt://{agentId}/active",
			Name:        "Active Prompt",
			Description: "The currently active prompt for an agent",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "config://model",
			Name:        "Model Configuration",
			Description: "Global model configuration (default model, temperature, token limits)",
			MimeType:    "application/json",
		},
		{
			URITemplate: "config://context",
			Name:        "Context Configuration",
			Description: "Context assembly configuration derived from model config (context window, history budget)",
			MimeType:    "application/json",
		},
	}
}

// ListResources returns concrete resource instances. It lists all active agents
// as agent:// and prompt:// resources, plus the two static config:// resources.
func (p *MCPResourceProvider) ListResources(ctx context.Context) ([]MCPResource, error) {
	agents, _, err := p.agents.List(ctx, true, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}

	resources := make([]MCPResource, 0, len(agents)*2+2)

	for _, a := range agents {
		resources = append(resources, MCPResource{
			URI:         "agent://" + a.ID,
			Name:        a.Name,
			Description: a.Description,
			MimeType:    "application/json",
		})
		resources = append(resources, MCPResource{
			URI:         "prompt://" + a.ID + "/active",
			Name:        a.Name + " — Active Prompt",
			Description: "Active prompt for agent " + a.ID,
			MimeType:    "text/plain",
		})
	}

	resources = append(resources,
		MCPResource{
			URI:         "config://model",
			Name:        "Model Configuration",
			Description: "Global model configuration",
			MimeType:    "application/json",
		},
		MCPResource{
			URI:         "config://context",
			Name:        "Context Configuration",
			Description: "Context assembly configuration",
			MimeType:    "application/json",
		},
	)

	return resources, nil
}

// ReadResource parses the given URI and fetches the corresponding data.
func (p *MCPResourceProvider) ReadResource(ctx context.Context, uri string) (*MCPResourceContent, *MCPJSONRPCError) {
	switch {
	case strings.HasPrefix(uri, "agent://"):
		return p.readAgent(ctx, uri)
	case strings.HasPrefix(uri, "prompt://"):
		return p.readPrompt(ctx, uri)
	case uri == "config://model":
		return p.readModelConfig(ctx)
	case uri == "config://context":
		return p.readContextConfig(ctx)
	default:
		return nil, &MCPJSONRPCError{
			Code:    -32602,
			Message: fmt.Sprintf("unsupported resource URI: %s", uri),
		}
	}
}

func (p *MCPResourceProvider) readAgent(ctx context.Context, uri string) (*MCPResourceContent, *MCPJSONRPCError) {
	agentID := strings.TrimPrefix(uri, "agent://")
	if agentID == "" {
		return nil, &MCPJSONRPCError{Code: -32602, Message: "agent ID is required in agent:// URI"}
	}

	agent, err := p.agents.GetByID(ctx, agentID)
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32602, Message: fmt.Sprintf("agent not found: %s", agentID)}
	}

	data, err := json.Marshal(agent)
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32603, Message: "failed to serialize agent"}
	}

	return &MCPResourceContent{
		URI:      uri,
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

func (p *MCPResourceProvider) readPrompt(ctx context.Context, uri string) (*MCPResourceContent, *MCPJSONRPCError) {
	// Expected format: prompt://{agentId}/active
	path := strings.TrimPrefix(uri, "prompt://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "active" {
		return nil, &MCPJSONRPCError{Code: -32602, Message: "invalid prompt URI: expected prompt://{agentId}/active"}
	}

	agentID := parts[0]
	if agentID == "" {
		return nil, &MCPJSONRPCError{Code: -32602, Message: "agent ID is required in prompt:// URI"}
	}

	prompt, err := p.prompts.GetActive(ctx, agentID)
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32602, Message: fmt.Sprintf("active prompt not found for agent: %s", agentID)}
	}

	return &MCPResourceContent{
		URI:      uri,
		MimeType: "text/plain",
		Text:     prompt.SystemPrompt,
	}, nil
}

func (p *MCPResourceProvider) readModelConfig(ctx context.Context) (*MCPResourceContent, *MCPJSONRPCError) {
	config, err := p.model.GetByScope(ctx, "global", "")
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32603, Message: "failed to fetch model config"}
	}

	if config == nil {
		return &MCPResourceContent{
			URI:      "config://model",
			MimeType: "application/json",
			Text:     "{}",
		}, nil
	}

	data, err := json.Marshal(config)
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32603, Message: "failed to serialize model config"}
	}

	return &MCPResourceContent{
		URI:      "config://model",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

// contextConfig is the derived context assembly portion of model config.
type contextConfig struct {
	DefaultContextWindow   int `json:"default_context_window"`
	DefaultMaxOutputTokens int `json:"default_max_output_tokens"`
	HistoryTokenBudget     int `json:"history_token_budget"`
	MaxHistoryMessages     int `json:"max_history_messages"`
}

func (p *MCPResourceProvider) readContextConfig(ctx context.Context) (*MCPResourceContent, *MCPJSONRPCError) {
	config, err := p.model.GetByScope(ctx, "global", "")
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32603, Message: "failed to fetch model config"}
	}

	cc := contextConfig{}
	if config != nil {
		cc = contextConfig{
			DefaultContextWindow:   config.DefaultContextWindow,
			DefaultMaxOutputTokens: config.DefaultMaxOutputTokens,
			HistoryTokenBudget:     config.HistoryTokenBudget,
			MaxHistoryMessages:     config.MaxHistoryMessages,
		}
	}

	data, err := json.Marshal(cc)
	if err != nil {
		return nil, &MCPJSONRPCError{Code: -32603, Message: "failed to serialize context config"}
	}

	return &MCPResourceContent{
		URI:      "config://context",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}
