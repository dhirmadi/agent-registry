package api

import (
	"context"
	"encoding/json"
	"time"

	"golang.org/x/sync/errgroup"
)

// JSON-RPC 2.0 error codes used by MCP tool executor.
const (
	mcpToolErrMethodNotFound = -32601
	mcpToolErrInvalidParams  = -32602
)

// MCPToolExecutor provides MCP tool definitions and execution against registry stores.
// Implements MCPToolExecutorInterface defined in mcp_handler.go.
type MCPToolExecutor struct {
	agents         AgentStoreForAPI
	prompts        PromptStoreForAPI
	mcpServers     MCPServerStoreForAPI
	model          ModelConfigStoreForAPI
	modelEndpoints ModelEndpointStoreForAPI
	externalURL    string
}

// NewMCPToolExecutor creates a new MCPToolExecutor with the required store dependencies.
func NewMCPToolExecutor(
	agents AgentStoreForAPI,
	prompts PromptStoreForAPI,
	mcpServers MCPServerStoreForAPI,
	model ModelConfigStoreForAPI,
	modelEndpoints ModelEndpointStoreForAPI,
	externalURL string,
) *MCPToolExecutor {
	return &MCPToolExecutor{
		agents:         agents,
		prompts:        prompts,
		mcpServers:     mcpServers,
		model:          model,
		modelEndpoints: modelEndpoints,
		externalURL:    externalURL,
	}
}

// ListTools returns all 5 MCP tool definitions with JSON Schema input schemas.
func (e *MCPToolExecutor) ListTools() []MCPToolDefinition {
	return []MCPToolDefinition{
		{
			Name:        "list_agents",
			Description: "List registered agents. Returns agent summaries with id, name, description, capabilities, and status.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"active_only": {
						"type": "boolean",
						"description": "If true (default), only return active agents"
					},
					"limit": {
						"type": "integer",
						"description": "Maximum number of agents to return (1-1000, default 100)"
					}
				}
			}`),
		},
		{
			Name:        "get_agent",
			Description: "Get full details of a specific agent by ID, including its tools, trust overrides, and active prompt.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"agent_id": {
						"type": "string",
						"description": "The unique agent identifier"
					}
				},
				"required": ["agent_id"]
			}`),
		},
		{
			Name:        "get_discovery",
			Description: "Get the full discovery payload with all active agents, MCP servers, model config, and model endpoints. Used for BFF cold-start.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "list_mcp_servers",
			Description: "List all registered MCP servers. Credentials are never included in the response.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "get_model_config",
			Description: "Get model configuration for a given scope. Defaults to global scope if no scope is specified.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scope": {
						"type": "string",
						"description": "Config scope: global, workspace, or agent (default: global)"
					},
					"workspace_id": {
						"type": "string",
						"description": "Workspace ID when scope is 'workspace'"
					}
				}
			}`),
		},
	}
}

// CallTool dispatches a tool call by name with JSON arguments.
// Returns a tool result on success, or a JSON-RPC error for protocol-level failures
// (unknown tool, invalid params). Store/execution errors are returned as tool results
// with IsError=true, not as JSON-RPC errors.
func (e *MCPToolExecutor) CallTool(ctx context.Context, name string, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	// Normalize nil/empty args
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}

	switch name {
	case "list_agents":
		return e.callListAgents(ctx, args)
	case "get_agent":
		return e.callGetAgent(ctx, args)
	case "get_discovery":
		return e.callGetDiscovery(ctx, args)
	case "list_mcp_servers":
		return e.callListMCPServers(ctx, args)
	case "get_model_config":
		return e.callGetModelConfig(ctx, args)
	default:
		return nil, &MCPJSONRPCError{Code: mcpToolErrMethodNotFound, Message: "unknown tool: " + name}
	}
}

// --- Tool implementations ---

func (e *MCPToolExecutor) callListAgents(ctx context.Context, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	var params struct {
		ActiveOnly *bool `json:"active_only"`
		Limit      *int  `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, &MCPJSONRPCError{Code: mcpToolErrInvalidParams, Message: "invalid parameters: " + err.Error()}
	}

	activeOnly := true
	if params.ActiveOnly != nil {
		activeOnly = *params.ActiveOnly
	}

	limit := 100
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	agents, total, err := e.agents.List(ctx, activeOnly, 0, limit)
	if err != nil {
		return mcpToolError("failed to list agents: " + err.Error()), nil
	}

	summaries := make([]agentAPIResponse, 0, len(agents))
	for i := range agents {
		summaries = append(summaries, toAgentAPIResponse(&agents[i], false))
	}

	return mcpToolJSON(map[string]interface{}{
		"agents": summaries,
		"total":  total,
	})
}

func (e *MCPToolExecutor) callGetAgent(ctx context.Context, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	var params struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, &MCPJSONRPCError{Code: mcpToolErrInvalidParams, Message: "invalid parameters: " + err.Error()}
	}
	if params.AgentID == "" {
		return nil, &MCPJSONRPCError{Code: mcpToolErrInvalidParams, Message: "agent_id is required"}
	}

	agent, err := e.agents.GetByID(ctx, params.AgentID)
	if err != nil {
		return mcpToolError("agent not found: " + params.AgentID), nil
	}

	resp := toAgentAPIResponse(agent, true)

	// Fetch active prompt (optional -- not an error if missing)
	var activePrompt interface{}
	if prompt, err := e.prompts.GetActive(ctx, params.AgentID); err == nil && prompt != nil {
		activePrompt = prompt
	}

	return mcpToolJSON(map[string]interface{}{
		"id":                   resp.ID,
		"name":                 resp.Name,
		"description":          resp.Description,
		"system_prompt":        resp.SystemPrompt,
		"tools":                resp.Tools,
		"trust_overrides":      resp.TrustOverrides,
		"capabilities":         resp.Capabilities,
		"example_prompts":      resp.ExamplePrompts,
		"required_connections": resp.RequiredConnections,
		"is_active":            resp.IsActive,
		"version":              resp.Version,
		"created_by":           resp.CreatedBy,
		"created_at":           resp.CreatedAt,
		"updated_at":           resp.UpdatedAt,
		"active_prompt":        activePrompt,
	})
}

func (e *MCPToolExecutor) callGetDiscovery(ctx context.Context, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	g, gCtx := errgroup.WithContext(ctx)

	var (
		agentsList         []agentAPIResponse
		mcpServersList     []mcpServerResponse
		modelConfig        interface{}
		modelEndpointsList []map[string]interface{}
	)

	g.Go(func() error {
		agents, _, err := e.agents.List(gCtx, true, 0, 1000)
		if err != nil {
			return err
		}
		agentsList = make([]agentAPIResponse, 0, len(agents))
		for i := range agents {
			agentsList = append(agentsList, toAgentAPIResponse(&agents[i], false))
		}
		return nil
	})

	g.Go(func() error {
		servers, err := e.mcpServers.List(gCtx)
		if err != nil {
			return err
		}
		mcpServersList = make([]mcpServerResponse, 0, len(servers))
		for i := range servers {
			mcpServersList = append(mcpServersList, toMCPServerResponse(&servers[i]))
		}
		return nil
	})

	g.Go(func() error {
		config, err := e.model.GetByScope(gCtx, "global", "")
		if err != nil {
			return err
		}
		if config == nil {
			modelConfig = map[string]interface{}{}
		} else {
			modelConfig = config
		}
		return nil
	})

	if e.modelEndpoints != nil {
		g.Go(func() error {
			endpoints, _, err := e.modelEndpoints.List(gCtx, nil, true, 0, 1000)
			if err != nil {
				return err
			}
			modelEndpointsList = make([]map[string]interface{}, 0, len(endpoints))
			for i := range endpoints {
				ep := &endpoints[i]
				entry := map[string]interface{}{
					"slug":         ep.Slug,
					"name":         ep.Name,
					"provider":     ep.Provider,
					"endpoint_url": ep.EndpointURL,
					"model_name":   ep.ModelName,
					"is_active":    ep.IsActive,
				}
				activeVer, err := e.modelEndpoints.GetActiveVersion(gCtx, ep.ID)
				if err == nil && activeVer != nil {
					entry["active_version"] = activeVer.Version
					entry["config"] = redactConfigHeaders(activeVer.Config)
				}
				modelEndpointsList = append(modelEndpointsList, entry)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return mcpToolError("failed to fetch discovery data: " + err.Error()), nil
	}

	if agentsList == nil {
		agentsList = []agentAPIResponse{}
	}
	if mcpServersList == nil {
		mcpServersList = []mcpServerResponse{}
	}
	if modelEndpointsList == nil {
		modelEndpointsList = []map[string]interface{}{}
	}

	return mcpToolJSON(map[string]interface{}{
		"agents":          agentsList,
		"mcp_servers":     mcpServersList,
		"model_config":    modelConfig,
		"model_endpoints": modelEndpointsList,
		"fetched_at":      time.Now().UTC().Format(time.RFC3339),
	})
}

func (e *MCPToolExecutor) callListMCPServers(ctx context.Context, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	servers, err := e.mcpServers.List(ctx)
	if err != nil {
		return mcpToolError("failed to list MCP servers: " + err.Error()), nil
	}

	resp := make([]mcpServerResponse, 0, len(servers))
	for i := range servers {
		resp = append(resp, toMCPServerResponse(&servers[i]))
	}

	return mcpToolJSON(map[string]interface{}{
		"servers": resp,
		"total":   len(resp),
	})
}

func (e *MCPToolExecutor) callGetModelConfig(ctx context.Context, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	var params struct {
		Scope       string `json:"scope"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, &MCPJSONRPCError{Code: mcpToolErrInvalidParams, Message: "invalid parameters: " + err.Error()}
	}

	scope := params.Scope
	if scope == "" {
		scope = "global"
	}
	scopeID := params.WorkspaceID

	config, err := e.model.GetByScope(ctx, scope, scopeID)
	if err != nil {
		return mcpToolError("failed to get model config: " + err.Error()), nil
	}

	if config == nil {
		return mcpToolJSON(map[string]interface{}{})
	}

	return mcpToolJSON(config)
}

// --- Helpers ---

func mcpToolJSON(data interface{}) (*MCPToolResult, *MCPJSONRPCError) {
	b, err := json.Marshal(data)
	if err != nil {
		return mcpToolError("failed to serialize result: " + err.Error()), nil
	}
	return &MCPToolResult{
		Content: []MCPToolResultContent{
			{Type: "text", Text: string(b)},
		},
	}, nil
}

func mcpToolError(msg string) *MCPToolResult {
	return &MCPToolResult{
		Content: []MCPToolResultContent{
			{Type: "text", Text: msg},
		},
		IsError: true,
	}
}
