package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/agent-smit/agentic-registry/internal/mcp"
)

// MCPToolExecutorInterface defines the contract for MCP tool operations.
// Satisfied by *MCPToolExecutor (mcp_tools.go).
type MCPToolExecutorInterface interface {
	ListTools() []MCPToolDefinition
	CallTool(ctx context.Context, name string, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError)
}

// MCPResourceProviderInterface defines the contract for MCP resource operations.
// Satisfied by *MCPResourceProvider (mcp_resources.go).
type MCPResourceProviderInterface interface {
	ListResources(ctx context.Context) ([]MCPResource, error)
	ReadResource(ctx context.Context, uri string) (*MCPResourceContent, *MCPJSONRPCError)
	ListTemplates() []MCPResourceTemplate
}

// MCPPromptProviderInterface defines the contract for MCP prompt operations.
// Satisfied by *MCPPromptProvider (mcp_prompts.go).
type MCPPromptProviderInterface interface {
	ListPrompts(ctx context.Context) ([]MCPPromptDefinition, error)
	GetPrompt(ctx context.Context, name string, args map[string]string) (*MCPPromptResult, *MCPJSONRPCError)
}

// MCPManifestProviderInterface defines the contract for serving the MCP manifest.
// Satisfied by *MCPManifestHandler (mcp_manifest.go).
type MCPManifestProviderInterface interface {
	GetManifest(w http.ResponseWriter, r *http.Request)
}

// MCPHandler is the top-level handler for MCP protocol endpoints.
// It implements mcp.MethodHandler and dispatches JSON-RPC methods to sub-providers.
type MCPHandler struct {
	tools     MCPToolExecutorInterface
	resources MCPResourceProviderInterface
	prompts   MCPPromptProviderInterface
	manifest  MCPManifestProviderInterface
	transport *mcp.Transport
}

// NewMCPHandler creates a new MCPHandler with all sub-providers.
func NewMCPHandler(
	tools MCPToolExecutorInterface,
	resources MCPResourceProviderInterface,
	prompts MCPPromptProviderInterface,
	manifest MCPManifestProviderInterface,
) *MCPHandler {
	h := &MCPHandler{
		tools:     tools,
		resources: resources,
		prompts:   prompts,
		manifest:  manifest,
	}

	// Create MCP transport with session support.
	sessions := mcp.NewSessionStore()
	h.transport = mcp.NewTransportWithSessions(h, sessions)

	return h
}

// ServerInfo implements mcp.MethodHandler.
func (h *MCPHandler) ServerInfo() mcp.ServerInfo {
	return mcp.ServerInfo{
		Name:    "agentic-registry",
		Version: "1.0.0",
	}
}

// Capabilities implements mcp.MethodHandler.
func (h *MCPHandler) Capabilities() mcp.ServerCapabilities {
	return mcp.ServerCapabilities{
		Tools:     &mcp.ToolsCapability{ListChanged: false},
		Resources: &mcp.ResourcesCapability{Subscribe: false, ListChanged: false},
		Prompts:   &mcp.PromptsCapability{ListChanged: false},
	}
}

// HandleMethod implements mcp.MethodHandler. It dispatches JSON-RPC methods
// to the appropriate sub-provider.
func (h *MCPHandler) HandleMethod(ctx context.Context, method string, params json.RawMessage) (interface{}, *mcp.JSONRPCError) {
	switch method {
	case "initialize":
		return h.handleInitialize(params)
	case "initialized":
		return nil, nil // no-op ack
	case "ping":
		return map[string]interface{}{}, nil

	case "tools/list":
		return h.handleToolsList()
	case "tools/call":
		return h.handleToolsCall(ctx, params)

	case "resources/list":
		return h.handleResourcesList(ctx)
	case "resources/read":
		return h.handleResourcesRead(ctx, params)
	case "resources/templates/list":
		return h.handleResourcesTemplatesList()

	case "prompts/list":
		return h.handlePromptsList(ctx)
	case "prompts/get":
		return h.handlePromptsGet(ctx, params)

	default:
		return nil, mcp.NewMethodNotFound("unknown method: " + method)
	}
}

// --- Method implementations ---

func (h *MCPHandler) handleInitialize(params json.RawMessage) (interface{}, *mcp.JSONRPCError) {
	// Initialize is handled by the transport layer for session creation.
	// If it reaches here (transport without sessions), return server capabilities.
	return mcp.InitializeResult{
		ProtocolVersion: "2025-03-26",
		Capabilities:    h.Capabilities(),
		ServerInfo:      h.ServerInfo(),
	}, nil
}

func (h *MCPHandler) handleToolsList() (interface{}, *mcp.JSONRPCError) {
	if h.tools == nil {
		return map[string]interface{}{"tools": []interface{}{}}, nil
	}
	apiTools := h.tools.ListTools()
	mcpTools := make([]mcp.ToolDefinition, 0, len(apiTools))
	for _, t := range apiTools {
		mcpTools = append(mcpTools, mcp.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return map[string]interface{}{"tools": mcpTools}, nil
}

func (h *MCPHandler) handleToolsCall(ctx context.Context, params json.RawMessage) (interface{}, *mcp.JSONRPCError) {
	if h.tools == nil {
		return nil, mcp.NewMethodNotFound("tools not available")
	}

	var p mcp.ToolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, mcp.NewInvalidParams("invalid tools/call params: " + err.Error())
	}
	if p.Name == "" {
		return nil, mcp.NewInvalidParams("tool name is required")
	}

	result, rpcErr := h.tools.CallTool(ctx, p.Name, p.Arguments)
	if rpcErr != nil {
		return nil, &mcp.JSONRPCError{Code: rpcErr.Code, Message: rpcErr.Message}
	}

	// Convert api MCPToolResult to mcp ToolResult
	mcpResult := mcp.ToolResult{
		IsError: result.IsError,
		Content: make([]mcp.ToolResultContent, len(result.Content)),
	}
	for i, c := range result.Content {
		mcpResult.Content[i] = mcp.ToolResultContent{Type: c.Type, Text: c.Text}
	}
	return mcpResult, nil
}

func (h *MCPHandler) handleResourcesList(ctx context.Context) (interface{}, *mcp.JSONRPCError) {
	if h.resources == nil {
		return map[string]interface{}{"resources": []interface{}{}}, nil
	}
	apiResources, err := h.resources.ListResources(ctx)
	if err != nil {
		return nil, mcp.NewInternalError("failed to list resources: " + err.Error())
	}
	mcpResources := make([]mcp.Resource, 0, len(apiResources))
	for _, r := range apiResources {
		mcpResources = append(mcpResources, mcp.Resource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		})
	}
	return map[string]interface{}{"resources": mcpResources}, nil
}

func (h *MCPHandler) handleResourcesRead(ctx context.Context, params json.RawMessage) (interface{}, *mcp.JSONRPCError) {
	if h.resources == nil {
		return nil, mcp.NewMethodNotFound("resources not available")
	}

	var p mcp.ReadResourceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, mcp.NewInvalidParams("invalid resources/read params: " + err.Error())
	}
	if p.URI == "" {
		return nil, mcp.NewInvalidParams("resource URI is required")
	}

	content, rpcErr := h.resources.ReadResource(ctx, p.URI)
	if rpcErr != nil {
		return nil, &mcp.JSONRPCError{Code: rpcErr.Code, Message: rpcErr.Message}
	}

	mcpContent := mcp.ResourceContent{
		URI:      content.URI,
		MimeType: content.MimeType,
		Text:     content.Text,
	}
	return map[string]interface{}{"contents": []mcp.ResourceContent{mcpContent}}, nil
}

func (h *MCPHandler) handleResourcesTemplatesList() (interface{}, *mcp.JSONRPCError) {
	if h.resources == nil {
		return map[string]interface{}{"resourceTemplates": []interface{}{}}, nil
	}
	apiTemplates := h.resources.ListTemplates()
	mcpTemplates := make([]mcp.ResourceTemplate, 0, len(apiTemplates))
	for _, t := range apiTemplates {
		mcpTemplates = append(mcpTemplates, mcp.ResourceTemplate{
			URITemplate: t.URITemplate,
			Name:        t.Name,
			Description: t.Description,
			MimeType:    t.MimeType,
		})
	}
	return map[string]interface{}{"resourceTemplates": mcpTemplates}, nil
}

func (h *MCPHandler) handlePromptsList(ctx context.Context) (interface{}, *mcp.JSONRPCError) {
	if h.prompts == nil {
		return map[string]interface{}{"prompts": []interface{}{}}, nil
	}
	apiPrompts, err := h.prompts.ListPrompts(ctx)
	if err != nil {
		return nil, mcp.NewInternalError("failed to list prompts: " + err.Error())
	}
	mcpPrompts := make([]mcp.PromptDefinition, 0, len(apiPrompts))
	for _, p := range apiPrompts {
		def := mcp.PromptDefinition{
			Name:        p.Name,
			Description: p.Description,
		}
		for _, a := range p.Arguments {
			def.Arguments = append(def.Arguments, mcp.PromptArgument{
				Name:        a.Name,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		mcpPrompts = append(mcpPrompts, def)
	}
	return map[string]interface{}{"prompts": mcpPrompts}, nil
}

func (h *MCPHandler) handlePromptsGet(ctx context.Context, params json.RawMessage) (interface{}, *mcp.JSONRPCError) {
	if h.prompts == nil {
		return nil, mcp.NewMethodNotFound("prompts not available")
	}

	var p mcp.GetPromptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, mcp.NewInvalidParams("invalid prompts/get params: " + err.Error())
	}
	if p.Name == "" {
		return nil, mcp.NewInvalidParams("prompt name is required")
	}

	result, rpcErr := h.prompts.GetPrompt(ctx, p.Name, p.Arguments)
	if rpcErr != nil {
		return nil, &mcp.JSONRPCError{Code: rpcErr.Code, Message: rpcErr.Message}
	}

	mcpResult := mcp.PromptResult{
		Description: result.Description,
	}
	for _, m := range result.Messages {
		mcpResult.Messages = append(mcpResult.Messages, mcp.PromptMessage{
			Role: m.Role,
			Content: mcp.PromptContent{
				Type: m.Content.Type,
				Text: m.Content.Text,
			},
		})
	}
	return mcpResult, nil
}

// ServeManifest handles GET /mcp.json (public, no auth).
func (h *MCPHandler) ServeManifest(w http.ResponseWriter, r *http.Request) {
	if h.manifest != nil {
		h.manifest.GetManifest(w, r)
		return
	}
	http.NotFound(w, r)
}

// HandlePost handles POST /mcp — the main JSON-RPC request endpoint.
// Delegates to the Streamable HTTP transport.
func (h *MCPHandler) HandlePost(w http.ResponseWriter, r *http.Request) {
	h.transport.ServeHTTP(w, r)
}

// HandleSSE handles GET /mcp — SSE stream for server-to-client notifications.
// Not implemented in this phase; returns 405 Method Not Allowed.
func (h *MCPHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "SSE not supported, use POST for JSON-RPC", http.StatusMethodNotAllowed)
}

// HandleDelete handles DELETE /mcp — session termination.
// Delegates to the transport layer for session cleanup.
func (h *MCPHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	h.transport.ServeHTTP(w, r)
}
