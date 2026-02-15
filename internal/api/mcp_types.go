package api

import "encoding/json"

// Shared MCP types used across mcp_tools.go, mcp_resources.go, mcp_prompts.go, and mcp_handler.go.
// These mirror the canonical types in internal/mcp/protocol.go but live in the api package
// to avoid circular imports. Agent D (task #9) will bridge these with the mcp package
// during handler dispatch wiring.

// --- JSON-RPC Error ---

// MCPJSONRPCError is a JSON-RPC 2.0 error used in MCP tool/resource/prompt responses.
type MCPJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Tool types ---

// MCPToolDefinition describes an MCP tool with its JSON Schema input.
type MCPToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPToolDef is an alias for MCPToolDefinition used by the tool executor.
type MCPToolDef = MCPToolDefinition

// MCPToolResultContent is a single content item in a tool result.
type MCPToolResultContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPToolResult is the result of executing an MCP tool.
type MCPToolResult struct {
	Content []MCPToolResultContent `json:"content"`
	IsError bool                   `json:"isError,omitempty"`
}

// --- Resource types ---

// MCPResource describes a concrete resource that the server offers.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceTemplate describes a parameterized URI template for dynamic resources.
type MCPResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceContent holds the content returned when reading a resource.
type MCPResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// --- Prompt types ---

// MCPPromptArgument describes an argument that a prompt template accepts.
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPPromptDefinition describes a prompt template exposed via MCP.
type MCPPromptDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

// MCPPromptMessage is a single message in a prompt result.
type MCPPromptMessage struct {
	Role    string         `json:"role"`
	Content MCPTextContent `json:"content"`
}

// MCPTextContent represents text content in an MCP message.
type MCPTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPPromptResult is the result of a prompts/get call.
type MCPPromptResult struct {
	Description string             `json:"description,omitempty"`
	Messages    []MCPPromptMessage `json:"messages"`
}
