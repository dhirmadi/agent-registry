package mcp

import (
	"context"
	"encoding/json"
)

// JSON-RPC 2.0 error codes.
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification returns true if the request has no ID (a JSON-RPC notification).
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil || string(r.ID) == ""
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification (no ID).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return e.Message
}

// Error constructors.

func NewParseError(msg string) *JSONRPCError {
	return &JSONRPCError{Code: ParseError, Message: msg}
}

func NewInvalidRequest(msg string) *JSONRPCError {
	return &JSONRPCError{Code: InvalidRequest, Message: msg}
}

func NewMethodNotFound(msg string) *JSONRPCError {
	return &JSONRPCError{Code: MethodNotFound, Message: msg}
}

func NewInvalidParams(msg string) *JSONRPCError {
	return &JSONRPCError{Code: InvalidParams, Message: msg}
}

func NewInternalError(msg string) *JSONRPCError {
	return &JSONRPCError{Code: InternalError, Message: msg}
}

// MCP protocol types.

// ServerInfo identifies the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientInfo identifies the MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities declares what the server supports.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe"`
	ListChanged bool `json:"listChanged"`
}

type PromptsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ClientCapabilities declares what the client supports.
type ClientCapabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

type RootsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type SamplingCapability struct{}

// InitializeParams is the params for the initialize request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// InitializeResult is the result for the initialize response.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// Tool types.

// ToolDefinition describes a tool for tools/list.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCallParams is the params for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolResult is the result for tools/call.
type ToolResult struct {
	Content []ToolResultContent `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

// ToolResultContent is a content item in a tool result.
type ToolResultContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Resource *EmbeddedResource `json:"resource,omitempty"`
}

// EmbeddedResource is a resource embedded in tool result content.
type EmbeddedResource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// Resource types.

// Resource describes a concrete resource for resources/list.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceTemplate describes a parameterized resource URI.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent is the content of a resource read.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ReadResourceParams is the params for resources/read.
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// Prompt types.

// PromptDefinition describes a prompt for prompts/list.
type PromptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes a prompt argument.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// GetPromptParams is the params for prompts/get.
type GetPromptParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// PromptResult is the result for prompts/get.
type PromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage is a message in a prompt result.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// PromptContent is the content of a prompt message.
type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// MethodHandler is the interface that MCP method dispatchers must implement.
// The transport layer calls HandleMethod for each incoming JSON-RPC request.
type MethodHandler interface {
	HandleMethod(ctx context.Context, method string, params json.RawMessage) (interface{}, *JSONRPCError)
	ServerInfo() ServerInfo
	Capabilities() ServerCapabilities
}
