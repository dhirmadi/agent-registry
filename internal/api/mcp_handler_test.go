package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-smit/agentic-registry/internal/mcp"
)

// --- Mock implementations for MCPHandler interfaces ---

type mockMCPToolExecutorForHandler struct {
	listToolsFn func() []MCPToolDefinition
	callToolFn  func(ctx context.Context, name string, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError)
}

func (m *mockMCPToolExecutorForHandler) ListTools() []MCPToolDefinition {
	if m.listToolsFn != nil {
		return m.listToolsFn()
	}
	return []MCPToolDefinition{
		{Name: "list_agents", Description: "List agents", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
}

func (m *mockMCPToolExecutorForHandler) CallTool(ctx context.Context, name string, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
	if m.callToolFn != nil {
		return m.callToolFn(ctx, name, args)
	}
	return &MCPToolResult{
		Content: []MCPToolResultContent{{Type: "text", Text: `{"agents":[]}`}},
	}, nil
}

type mockMCPResourceProviderForHandler struct{}

func (m *mockMCPResourceProviderForHandler) ListResources(_ context.Context) ([]MCPResource, error) {
	return []MCPResource{
		{URI: "config://model", Name: "Model config", MimeType: "application/json"},
	}, nil
}

func (m *mockMCPResourceProviderForHandler) ReadResource(_ context.Context, uri string) (*MCPResourceContent, *MCPJSONRPCError) {
	if uri == "config://model" {
		return &MCPResourceContent{URI: uri, MimeType: "application/json", Text: `{"default_model":"gpt-4"}`}, nil
	}
	return nil, &MCPJSONRPCError{Code: -32602, Message: "unknown URI: " + uri}
}

func (m *mockMCPResourceProviderForHandler) ListTemplates() []MCPResourceTemplate {
	return []MCPResourceTemplate{
		{URITemplate: "agent://{agentId}", Name: "Agent by ID", MimeType: "application/json"},
	}
}

type mockMCPPromptProviderForHandler struct{}

func (m *mockMCPPromptProviderForHandler) ListPrompts(_ context.Context) ([]MCPPromptDefinition, error) {
	return []MCPPromptDefinition{
		{Name: "test-agent", Description: "Test prompt", Arguments: []MCPPromptArgument{
			{Name: "topic", Description: "The topic", Required: true},
		}},
	}, nil
}

func (m *mockMCPPromptProviderForHandler) GetPrompt(_ context.Context, name string, _ map[string]string) (*MCPPromptResult, *MCPJSONRPCError) {
	if name == "test-agent" {
		return &MCPPromptResult{
			Description: "Active prompt for Test Agent",
			Messages: []MCPPromptMessage{
				{Role: "user", Content: MCPTextContent{Type: "text", Text: "You are a test agent."}},
			},
		}, nil
	}
	return nil, &MCPJSONRPCError{Code: -32602, Message: "prompt not found: " + name}
}

type mockMCPManifestProviderForHandler struct{}

func (m *mockMCPManifestProviderForHandler) GetManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"name":"agentic-registry","version":"1.0.0"}`))
}

// --- Helper: build handler with standard mocks ---

func newTestMCPHandler() *MCPHandler {
	return NewMCPHandler(
		&mockMCPToolExecutorForHandler{},
		&mockMCPResourceProviderForHandler{},
		&mockMCPPromptProviderForHandler{},
		&mockMCPManifestProviderForHandler{},
	)
}

// --- Helper: make a JSON-RPC request and parse the response ---

func mcpPost(t *testing.T, handler http.HandlerFunc, body string) *mcp.JSONRPCResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)

	respBody, _ := io.ReadAll(rec.Body)

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC response: %v\nbody: %s", err, string(respBody))
	}
	return &resp
}

// --- Constructor tests ---

func TestNewMCPHandler(t *testing.T) {
	h := newTestMCPHandler()
	if h == nil {
		t.Fatal("NewMCPHandler returned nil")
	}
	if h.tools == nil {
		t.Error("tools is nil")
	}
	if h.resources == nil {
		t.Error("resources is nil")
	}
	if h.prompts == nil {
		t.Error("prompts is nil")
	}
	if h.manifest == nil {
		t.Error("manifest is nil")
	}
	if h.transport == nil {
		t.Error("transport is nil")
	}
}

func TestMCPHandler_NilProviders(t *testing.T) {
	h := NewMCPHandler(nil, nil, nil, nil)
	if h == nil {
		t.Fatal("NewMCPHandler returned nil with all nil providers")
	}
	if h.transport == nil {
		t.Error("transport should still be created")
	}
}

// --- MethodHandler interface compliance ---

func TestMCPHandler_ServerInfo(t *testing.T) {
	h := newTestMCPHandler()
	info := h.ServerInfo()
	if info.Name != "agentic-registry" {
		t.Errorf("Name = %q, want agentic-registry", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", info.Version)
	}
}

func TestMCPHandler_Capabilities(t *testing.T) {
	h := newTestMCPHandler()
	caps := h.Capabilities()
	if caps.Tools == nil {
		t.Error("Tools capability is nil")
	}
	if caps.Resources == nil {
		t.Error("Resources capability is nil")
	}
	if caps.Prompts == nil {
		t.Error("Prompts capability is nil")
	}
}

// --- Manifest endpoint ---

func TestMCPHandler_ServeManifest(t *testing.T) {
	h := newTestMCPHandler()

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	rec := httptest.NewRecorder()
	h.ServeManifest(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestMCPHandler_ServeManifest_NilManifest(t *testing.T) {
	h := NewMCPHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	rec := httptest.NewRecorder()
	h.ServeManifest(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- SSE endpoint ---

func TestMCPHandler_HandleSSE_MethodNotAllowed(t *testing.T) {
	h := newTestMCPHandler()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	h.HandleSSE(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// --- HandleMethod dispatch tests ---

func TestMCPHandler_HandleMethod_Ping(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "ping", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result for ping")
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestMCPHandler_HandleMethod_Initialized(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "initialized", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for initialized, got %v", result)
	}
}

func TestMCPHandler_HandleMethod_Initialize(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "initialize", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	initResult, ok := result.(mcp.InitializeResult)
	if !ok {
		t.Fatal("expected InitializeResult")
	}
	if initResult.ProtocolVersion != "2025-03-26" {
		t.Errorf("ProtocolVersion = %q", initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "agentic-registry" {
		t.Errorf("ServerInfo.Name = %q", initResult.ServerInfo.Name)
	}
}

func TestMCPHandler_HandleMethod_UnknownMethod(t *testing.T) {
	h := newTestMCPHandler()
	_, err := h.HandleMethod(context.Background(), "nonexistent/method", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	if err.Code != mcp.MethodNotFound {
		t.Errorf("error code = %d, want %d", err.Code, mcp.MethodNotFound)
	}
}

// --- tools/list ---

func TestMCPHandler_HandleMethod_ToolsList(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected map result")
	}
	tools, ok := m["tools"].([]mcp.ToolDefinition)
	if !ok {
		t.Fatal("expected tools array")
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "list_agents" {
		t.Errorf("tool name = %q, want list_agents", tools[0].Name)
	}
}

func TestMCPHandler_HandleMethod_ToolsList_NilProvider(t *testing.T) {
	h := NewMCPHandler(nil, nil, nil, nil)
	result, err := h.HandleMethod(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]interface{})
	tools := m["tools"].([]interface{})
	if len(tools) != 0 {
		t.Errorf("expected empty tools, got %d", len(tools))
	}
}

// --- tools/call ---

func TestMCPHandler_HandleMethod_ToolsCall(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{"name":"list_agents","arguments":{}}`)
	result, err := h.HandleMethod(context.Background(), "tools/call", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	toolResult, ok := result.(mcp.ToolResult)
	if !ok {
		t.Fatal("expected ToolResult")
	}
	if len(toolResult.Content) != 1 {
		t.Errorf("expected 1 content, got %d", len(toolResult.Content))
	}
}

func TestMCPHandler_HandleMethod_ToolsCall_MissingName(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{}`)
	_, err := h.HandleMethod(context.Background(), "tools/call", params)
	if err == nil {
		t.Fatal("expected error for missing tool name")
	}
	if err.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", err.Code, mcp.InvalidParams)
	}
}

func TestMCPHandler_HandleMethod_ToolsCall_InvalidParams(t *testing.T) {
	h := newTestMCPHandler()
	_, err := h.HandleMethod(context.Background(), "tools/call", json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
	if err.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", err.Code, mcp.InvalidParams)
	}
}

func TestMCPHandler_HandleMethod_ToolsCall_ErrorFromExecutor(t *testing.T) {
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, name string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				return nil, &MCPJSONRPCError{Code: -32601, Message: "unknown tool: " + name}
			},
		},
		nil, nil, nil,
	)
	params := json.RawMessage(`{"name":"bad_tool"}`)
	_, err := h.HandleMethod(context.Background(), "tools/call", params)
	if err == nil {
		t.Fatal("expected error from executor")
	}
	if err.Code != -32601 {
		t.Errorf("error code = %d, want -32601", err.Code)
	}
}

// --- resources/list ---

func TestMCPHandler_HandleMethod_ResourcesList(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "resources/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]interface{})
	resources := m["resources"].([]mcp.Resource)
	if len(resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].URI != "config://model" {
		t.Errorf("URI = %q, want config://model", resources[0].URI)
	}
}

// --- resources/read ---

func TestMCPHandler_HandleMethod_ResourcesRead(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{"uri":"config://model"}`)
	result, err := h.HandleMethod(context.Background(), "resources/read", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]interface{})
	contents := m["contents"].([]mcp.ResourceContent)
	if len(contents) != 1 {
		t.Errorf("expected 1 content, got %d", len(contents))
	}
	if contents[0].URI != "config://model" {
		t.Errorf("URI = %q", contents[0].URI)
	}
}

func TestMCPHandler_HandleMethod_ResourcesRead_MissingURI(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{}`)
	_, err := h.HandleMethod(context.Background(), "resources/read", params)
	if err == nil {
		t.Fatal("expected error for missing URI")
	}
	if err.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", err.Code, mcp.InvalidParams)
	}
}

func TestMCPHandler_HandleMethod_ResourcesRead_UnknownURI(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{"uri":"bad://uri"}`)
	_, err := h.HandleMethod(context.Background(), "resources/read", params)
	if err == nil {
		t.Fatal("expected error for unknown URI")
	}
}

// --- resources/templates/list ---

func TestMCPHandler_HandleMethod_ResourcesTemplatesList(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "resources/templates/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]interface{})
	templates := m["resourceTemplates"].([]mcp.ResourceTemplate)
	if len(templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(templates))
	}
}

// --- prompts/list ---

func TestMCPHandler_HandleMethod_PromptsList(t *testing.T) {
	h := newTestMCPHandler()
	result, err := h.HandleMethod(context.Background(), "prompts/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]interface{})
	prompts := m["prompts"].([]mcp.PromptDefinition)
	if len(prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(prompts))
	}
	if prompts[0].Name != "test-agent" {
		t.Errorf("prompt name = %q", prompts[0].Name)
	}
	if len(prompts[0].Arguments) != 1 {
		t.Errorf("expected 1 argument, got %d", len(prompts[0].Arguments))
	}
}

// --- prompts/get ---

func TestMCPHandler_HandleMethod_PromptsGet(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{"name":"test-agent"}`)
	result, err := h.HandleMethod(context.Background(), "prompts/get", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	promptResult, ok := result.(mcp.PromptResult)
	if !ok {
		t.Fatal("expected PromptResult")
	}
	if len(promptResult.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(promptResult.Messages))
	}
	if promptResult.Messages[0].Role != "user" {
		t.Errorf("role = %q, want user", promptResult.Messages[0].Role)
	}
}

func TestMCPHandler_HandleMethod_PromptsGet_MissingName(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{}`)
	_, err := h.HandleMethod(context.Background(), "prompts/get", params)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if err.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", err.Code, mcp.InvalidParams)
	}
}

func TestMCPHandler_HandleMethod_PromptsGet_NotFound(t *testing.T) {
	h := newTestMCPHandler()
	params := json.RawMessage(`{"name":"unknown-agent"}`)
	_, err := h.HandleMethod(context.Background(), "prompts/get", params)
	if err == nil {
		t.Fatal("expected error for unknown prompt")
	}
}

// --- Full JSON-RPC lifecycle via HandlePost ---

func TestMCPHandler_HandlePost_FullLifecycle(t *testing.T) {
	h := newTestMCPHandler()

	// 1. Initialize
	resp := mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	var initResult mcp.InitializeResult
	json.Unmarshal(resp.Result, &initResult)
	if initResult.ProtocolVersion != "2025-03-26" {
		t.Errorf("protocol version = %q", initResult.ProtocolVersion)
	}

	// 2. Initialized notification (no response expected for notifications, but our simple test helper will get a response)
	// Skip: notifications are handled differently at the transport level

	// 3. Ping
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if resp.Error != nil {
		t.Fatalf("ping error: %v", resp.Error)
	}

	// 4. tools/list
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}

	// 5. tools/call
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error)
	}

	// 6. resources/list
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":5,"method":"resources/list"}`)
	if resp.Error != nil {
		t.Fatalf("resources/list error: %v", resp.Error)
	}

	// 7. resources/read
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"config://model"}}`)
	if resp.Error != nil {
		t.Fatalf("resources/read error: %v", resp.Error)
	}

	// 8. prompts/list
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":7,"method":"prompts/list"}`)
	if resp.Error != nil {
		t.Fatalf("prompts/list error: %v", resp.Error)
	}

	// 9. prompts/get
	resp = mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":8,"method":"prompts/get","params":{"name":"test-agent"}}`)
	if resp.Error != nil {
		t.Fatalf("prompts/get error: %v", resp.Error)
	}
}

func TestMCPHandler_HandlePost_UnknownMethod(t *testing.T) {
	h := newTestMCPHandler()
	resp := mcpPost(t, h.HandlePost, `{"jsonrpc":"2.0","id":1,"method":"nonexistent"}`)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != mcp.MethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, mcp.MethodNotFound)
	}
}

func TestMCPHandler_HandlePost_InvalidJSON(t *testing.T) {
	h := newTestMCPHandler()
	resp := mcpPost(t, h.HandlePost, `{not valid json`)
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp.Error.Code != mcp.ParseError {
		t.Errorf("error code = %d, want %d", resp.Error.Code, mcp.ParseError)
	}
}

func TestMCPHandler_HandlePost_MissingJSONRPC(t *testing.T) {
	h := newTestMCPHandler()
	resp := mcpPost(t, h.HandlePost, `{"id":1,"method":"ping"}`)
	if resp.Error == nil {
		t.Fatal("expected error for missing jsonrpc")
	}
	if resp.Error.Code != mcp.InvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, mcp.InvalidRequest)
	}
}

func TestMCPHandler_HandlePost_WrongContentType(t *testing.T) {
	h := newTestMCPHandler()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.HandlePost(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", rec.Code)
	}
}

// --- Delete endpoint ---

func TestMCPHandler_HandleDelete(t *testing.T) {
	h := newTestMCPHandler()
	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

// --- Batch request ---

func TestMCPHandler_HandlePost_Batch(t *testing.T) {
	h := newTestMCPHandler()
	body := `[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","id":2,"method":"tools/list"},
		{"jsonrpc":"2.0","id":3,"method":"nonexistent"}
	]`

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePost(rec, req)

	respBody, _ := io.ReadAll(rec.Body)

	var responses []mcp.JSONRPCResponse
	if err := json.Unmarshal(respBody, &responses); err != nil {
		t.Fatalf("failed to parse batch response: %v\nbody: %s", err, string(respBody))
	}

	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	// First (ping) should succeed
	if responses[0].Error != nil {
		t.Errorf("ping should succeed, got error: %v", responses[0].Error)
	}

	// Second (tools/list) should succeed
	if responses[1].Error != nil {
		t.Errorf("tools/list should succeed, got error: %v", responses[1].Error)
	}

	// Third (nonexistent) should fail
	if responses[2].Error == nil {
		t.Error("nonexistent method should fail")
	} else if responses[2].Error.Code != mcp.MethodNotFound {
		t.Errorf("error code = %d, want %d", responses[2].Error.Code, mcp.MethodNotFound)
	}
}
