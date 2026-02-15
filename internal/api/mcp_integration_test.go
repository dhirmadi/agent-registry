package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/mcp"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// Task #6: Integration Testing — MCP Client Compatibility
//
// Tests simulate a real MCP client performing the full handshake + protocol
// lifecycle via HTTP, verifying the server responds correctly at every step.
// =============================================================================

// --- Fixtures ---

func seedAgentStore() *mockAgentStoreForMCPTools {
	return &mockAgentStoreForMCPTools{
		listFn: func(_ context.Context, activeOnly bool, _, limit int) ([]store.Agent, int, error) {
			agents := []store.Agent{
				{
					ID: "search-agent", Name: "Search Agent", Description: "Searches the web",
					IsActive: true, Version: 2,
					Tools:          json.RawMessage(`[{"name":"web_search","source":"internal"}]`),
					TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`["Search for X"]`),
					CreatedBy: "admin", CreatedAt: time.Now(), UpdatedAt: time.Now(),
				},
				{
					ID: "code-agent", Name: "Code Agent", Description: "Writes code",
					IsActive: true, Version: 5,
					Tools:          json.RawMessage(`[{"name":"file_edit","source":"mcp","server_label":"editor"}]`),
					TrustOverrides: json.RawMessage(`{"allow_file_write": true}`), ExamplePrompts: json.RawMessage(`[]`),
					CreatedBy: "admin", CreatedAt: time.Now(), UpdatedAt: time.Now(),
				},
			}
			if activeOnly {
				var filtered []store.Agent
				for _, a := range agents {
					if a.IsActive {
						filtered = append(filtered, a)
					}
				}
				agents = filtered
			}
			total := len(agents)
			if limit > 0 && limit < len(agents) {
				agents = agents[:limit]
			}
			return agents, total, nil
		},
		getByIDFn: func(_ context.Context, id string) (*store.Agent, error) {
			switch id {
			case "search-agent":
				return &store.Agent{
					ID: "search-agent", Name: "Search Agent", Description: "Searches the web",
					IsActive: true, Version: 2,
					Tools:          json.RawMessage(`[{"name":"web_search","source":"internal"}]`),
					TrustOverrides: json.RawMessage(`{}`), ExamplePrompts: json.RawMessage(`["Search for X"]`),
					CreatedBy: "admin",
				}, nil
			case "code-agent":
				return &store.Agent{
					ID: "code-agent", Name: "Code Agent", Description: "Writes code",
					IsActive: true, Version: 5,
					Tools:          json.RawMessage(`[{"name":"file_edit","source":"mcp","server_label":"editor"}]`),
					TrustOverrides: json.RawMessage(`{"allow_file_write": true}`), ExamplePrompts: json.RawMessage(`[]`),
					CreatedBy: "admin",
				}, nil
			default:
				return nil, fmt.Errorf("agent not found: %s", id)
			}
		},
	}
}

func seedPromptStore() *mockPromptStoreForMCPTools {
	return &mockPromptStoreForMCPTools{
		getActiveFn: func(_ context.Context, agentID string) (*store.Prompt, error) {
			switch agentID {
			case "search-agent":
				return &store.Prompt{
					ID: uuid.New(), AgentID: "search-agent",
					SystemPrompt: "You are a search assistant. Search for {{query}} about {{topic}}.",
					TemplateVars: json.RawMessage(`[{"name":"query","description":"Search query","required":true},{"name":"topic","description":"Topic context","required":false}]`),
					Version: 3, Mode: "toolcalling_auto", IsActive: true,
				}, nil
			case "code-agent":
				return &store.Prompt{
					ID: uuid.New(), AgentID: "code-agent",
					SystemPrompt: "You write {{language}} code.",
					TemplateVars: json.RawMessage(`[{"name":"language","description":"Programming language","required":true}]`),
					Version: 1, Mode: "rag_readonly", IsActive: true,
				}, nil
			default:
				return nil, fmt.Errorf("no active prompt for agent: %s", agentID)
			}
		},
	}
}

func seedMCPServerStore() *mockMCPServerStoreForMCPTools {
	return &mockMCPServerStoreForMCPTools{
		listFn: func(_ context.Context) ([]store.MCPServer, error) {
			return []store.MCPServer{
				{
					ID: uuid.New(), Label: "editor-mcp", Endpoint: "https://mcp-editor.example.com",
					AuthType: "bearer", AuthCredential: "super-secret-token",
					IsEnabled: true, CircuitBreaker: json.RawMessage(`{"fail_threshold":3,"open_duration_s":60}`),
					DiscoveryInterval: "5m", HealthEndpoint: "https://mcp-editor.example.com/health",
				},
			}, nil
		},
	}
}

func seedModelConfigStore() *mockModelConfigStoreForMCPTools {
	return &mockModelConfigStoreForMCPTools{
		getByScopeFn: func(_ context.Context, scope, scopeID string) (*store.ModelConfig, error) {
			return &store.ModelConfig{
				DefaultModel:           "claude-sonnet-4-5-20250929",
				Temperature:            0.7,
				MaxTokens:              4096,
				MaxToolRounds:          10,
				DefaultContextWindow:   128000,
				DefaultMaxOutputTokens: 8192,
				HistoryTokenBudget:     64000,
				MaxHistoryMessages:     50,
				EmbeddingModel:         "text-embedding-3-small",
			}, nil
		},
	}
}

func seedModelEndpointStore() *mockModelEndpointStoreForMCPTools {
	epID := uuid.New()
	return &mockModelEndpointStoreForMCPTools{
		listFn: func(_ context.Context, _ *string, _ bool, _, _ int) ([]store.ModelEndpoint, int, error) {
			return []store.ModelEndpoint{
				{ID: epID, Slug: "openai-prod", Name: "OpenAI Production", Provider: "openai", EndpointURL: "https://api.openai.com/v1", ModelName: "gpt-4", IsActive: true},
			}, 1, nil
		},
		getActiveVersionFn: func(_ context.Context, _ uuid.UUID) (*store.ModelEndpointVersion, error) {
			return &store.ModelEndpointVersion{Version: 1, Config: json.RawMessage(`{"temperature":0.7,"headers":{"Authorization":"Bearer sk-secret"}}`)}, nil
		},
	}
}

func newSeededMCPHandler() *MCPHandler {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	mcpServers := seedMCPServerStore()
	modelConfig := seedModelConfigStore()
	modelEndpoints := seedModelEndpointStore()

	tools := NewMCPToolExecutor(agents, prompts, mcpServers, modelConfig, modelEndpoints, "https://registry.example.com")
	resources := NewMCPResourceProvider(agents, prompts, modelConfig)
	promptProvider := NewMCPPromptProvider(agents, prompts)
	manifest := NewMCPManifestHandler("https://registry.example.com")

	return NewMCPHandler(tools, resources, promptProvider, manifest)
}

// mcpRPC sends a single JSON-RPC request and returns the parsed response.
func mcpRPC(t *testing.T, handler http.HandlerFunc, id interface{}, method string, params interface{}) *mcp.JSONRPCResponse {
	t.Helper()
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if id != nil {
		req["id"] = id
	}
	if params != nil {
		req["params"] = params
	}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, httpReq)

	respBody, _ := io.ReadAll(rec.Body)
	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC response: %v\nbody: %s", err, string(respBody))
	}
	return &resp
}

// mcpBatch sends a batch of JSON-RPC requests and returns the parsed responses.
func mcpBatch(t *testing.T, handler http.HandlerFunc, requests []map[string]interface{}) []mcp.JSONRPCResponse {
	t.Helper()
	body, _ := json.Marshal(requests)
	httpReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, httpReq)

	respBody, _ := io.ReadAll(rec.Body)
	var responses []mcp.JSONRPCResponse
	if err := json.Unmarshal(respBody, &responses); err != nil {
		t.Fatalf("failed to parse batch response: %v\nbody: %s", err, string(respBody))
	}
	return responses
}

// --- Full MCP Client Handshake + Lifecycle ---

func TestIntegration_ClientHandshake_FullLifecycle(t *testing.T) {
	h := newSeededMCPHandler()

	// Step 1: initialize — client introduces itself
	resp := mcpRPC(t, h.HandlePost, 1, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "test-client", "version": "0.1.0"},
	})
	if resp.Error != nil {
		t.Fatalf("initialize failed: %s", resp.Error.Message)
	}
	var initResult mcp.InitializeResult
	json.Unmarshal(resp.Result, &initResult)
	if initResult.ProtocolVersion != "2025-03-26" {
		t.Errorf("protocol version = %q, want 2025-03-26", initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "agentic-registry" {
		t.Errorf("server name = %q", initResult.ServerInfo.Name)
	}
	if initResult.Capabilities.Tools == nil {
		t.Error("expected Tools capability")
	}
	if initResult.Capabilities.Resources == nil {
		t.Error("expected Resources capability")
	}
	if initResult.Capabilities.Prompts == nil {
		t.Error("expected Prompts capability")
	}

	// Step 2: initialized — client ack (notification, no ID)
	notifReq := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","method":"initialized"}`))
	notifReq.Header.Set("Content-Type", "application/json")
	notifRec := httptest.NewRecorder()
	h.HandlePost(notifRec, notifReq)
	if notifRec.Code != http.StatusNoContent {
		t.Errorf("initialized notification: status = %d, want 204", notifRec.Code)
	}

	// Step 3: ping
	resp = mcpRPC(t, h.HandlePost, 2, "ping", nil)
	if resp.Error != nil {
		t.Fatalf("ping failed: %s", resp.Error.Message)
	}

	// Step 4: tools/list — discover available tools
	resp = mcpRPC(t, h.HandlePost, 3, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list failed: %s", resp.Error.Message)
	}
	var toolsList map[string]interface{}
	json.Unmarshal(resp.Result, &toolsList)
	tools, ok := toolsList["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}

	// Step 5: tools/call — execute list_agents
	resp = mcpRPC(t, h.HandlePost, 4, "tools/call", map[string]interface{}{
		"name":      "list_agents",
		"arguments": map[string]interface{}{},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call list_agents failed: %s", resp.Error.Message)
	}

	// Step 6: tools/call — execute get_agent
	resp = mcpRPC(t, h.HandlePost, 5, "tools/call", map[string]interface{}{
		"name":      "get_agent",
		"arguments": map[string]interface{}{"agent_id": "search-agent"},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call get_agent failed: %s", resp.Error.Message)
	}

	// Step 7: tools/call — execute get_discovery
	resp = mcpRPC(t, h.HandlePost, 6, "tools/call", map[string]interface{}{
		"name":      "get_discovery",
		"arguments": map[string]interface{}{},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call get_discovery failed: %s", resp.Error.Message)
	}

	// Step 8: tools/call — execute list_mcp_servers
	resp = mcpRPC(t, h.HandlePost, 7, "tools/call", map[string]interface{}{
		"name":      "list_mcp_servers",
		"arguments": map[string]interface{}{},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call list_mcp_servers failed: %s", resp.Error.Message)
	}

	// Step 9: tools/call — execute get_model_config
	resp = mcpRPC(t, h.HandlePost, 8, "tools/call", map[string]interface{}{
		"name":      "get_model_config",
		"arguments": map[string]interface{}{},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call get_model_config failed: %s", resp.Error.Message)
	}

	// Step 10: resources/list
	resp = mcpRPC(t, h.HandlePost, 9, "resources/list", nil)
	if resp.Error != nil {
		t.Fatalf("resources/list failed: %s", resp.Error.Message)
	}

	// Step 11: resources/read (agent)
	resp = mcpRPC(t, h.HandlePost, 10, "resources/read", map[string]interface{}{
		"uri": "agent://search-agent",
	})
	if resp.Error != nil {
		t.Fatalf("resources/read agent failed: %s", resp.Error.Message)
	}

	// Step 12: resources/read (prompt)
	resp = mcpRPC(t, h.HandlePost, 11, "resources/read", map[string]interface{}{
		"uri": "prompt://search-agent/active",
	})
	if resp.Error != nil {
		t.Fatalf("resources/read prompt failed: %s", resp.Error.Message)
	}

	// Step 13: resources/read (config)
	resp = mcpRPC(t, h.HandlePost, 12, "resources/read", map[string]interface{}{
		"uri": "config://model",
	})
	if resp.Error != nil {
		t.Fatalf("resources/read config failed: %s", resp.Error.Message)
	}

	// Step 14: resources/templates/list
	resp = mcpRPC(t, h.HandlePost, 13, "resources/templates/list", nil)
	if resp.Error != nil {
		t.Fatalf("resources/templates/list failed: %s", resp.Error.Message)
	}

	// Step 15: prompts/list
	resp = mcpRPC(t, h.HandlePost, 14, "prompts/list", nil)
	if resp.Error != nil {
		t.Fatalf("prompts/list failed: %s", resp.Error.Message)
	}

	// Step 16: prompts/get
	resp = mcpRPC(t, h.HandlePost, 15, "prompts/get", map[string]interface{}{
		"name": "search-agent",
	})
	if resp.Error != nil {
		t.Fatalf("prompts/get failed: %s", resp.Error.Message)
	}

	// Step 17: Session termination (DELETE)
	delReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	delRec := httptest.NewRecorder()
	h.HandleDelete(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", delRec.Code)
	}
}

func TestIntegration_SessionManagement(t *testing.T) {
	h := newSeededMCPHandler()

	// Initialize and get session ID
	initReq := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"session-test","version":"1.0"}}}`))
	initReq.Header.Set("Content-Type", "application/json")
	initRec := httptest.NewRecorder()
	h.HandlePost(initRec, initReq)

	sessionID := initRec.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("expected Mcp-Session-Id in initialize response")
	}
	if len(sessionID) != 64 {
		t.Errorf("session ID length = %d, want 64 hex chars", len(sessionID))
	}

	// Second initialize should produce a different session ID
	initReq2 := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"session-test-2","version":"1.0"}}}`))
	initReq2.Header.Set("Content-Type", "application/json")
	initRec2 := httptest.NewRecorder()
	h.HandlePost(initRec2, initReq2)

	sessionID2 := initRec2.Header().Get("Mcp-Session-Id")
	if sessionID2 == "" {
		t.Fatal("second initialize should also return session ID")
	}
	if sessionID == sessionID2 {
		t.Error("two initializations should produce different session IDs")
	}

	// DELETE with session header should succeed
	delReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	delReq.Header.Set("Mcp-Session-Id", sessionID)
	delRec := httptest.NewRecorder()
	h.HandleDelete(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", delRec.Code)
	}
}

func TestIntegration_BatchRequest(t *testing.T) {
	h := newSeededMCPHandler()

	requests := []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "ping"},
		{"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
		{"jsonrpc": "2.0", "id": 3, "method": "resources/list"},
		{"jsonrpc": "2.0", "id": 4, "method": "prompts/list"},
		{"jsonrpc": "2.0", "id": 5, "method": "nonexistent"},
	}

	responses := mcpBatch(t, h.HandlePost, requests)
	if len(responses) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(responses))
	}

	// First 4 should succeed
	for i := 0; i < 4; i++ {
		if responses[i].Error != nil {
			t.Errorf("response[%d] unexpected error: %s", i, responses[i].Error.Message)
		}
	}

	// Last should fail
	if responses[4].Error == nil {
		t.Error("response[4] expected error for nonexistent method")
	} else if responses[4].Error.Code != mcp.MethodNotFound {
		t.Errorf("response[4] error code = %d, want %d", responses[4].Error.Code, mcp.MethodNotFound)
	}
}

func TestIntegration_BatchWithNotifications(t *testing.T) {
	h := newSeededMCPHandler()

	// Mix of requests and notifications in batch
	requests := []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "ping"},
		{"jsonrpc": "2.0", "method": "initialized"},     // notification (no id)
		{"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
	}

	responses := mcpBatch(t, h.HandlePost, requests)

	// Notifications should be skipped in batch response
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses (notifications skipped), got %d", len(responses))
	}
}

func TestIntegration_JSONRPCEnvelopeCorrectness(t *testing.T) {
	h := newSeededMCPHandler()

	resp := mcpRPC(t, h.HandlePost, 42, "ping", nil)

	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	// ID should be preserved
	var id float64
	if err := json.Unmarshal(resp.ID, &id); err != nil {
		t.Fatalf("failed to parse ID: %v", err)
	}
	if id != 42 {
		t.Errorf("response ID = %v, want 42", id)
	}
	if resp.Error != nil {
		t.Error("unexpected error in response")
	}
	if resp.Result == nil {
		t.Error("expected result in response")
	}
}

func TestIntegration_StringRequestID(t *testing.T) {
	h := newSeededMCPHandler()

	resp := mcpRPC(t, h.HandlePost, "my-request-id", "ping", nil)

	var id string
	if err := json.Unmarshal(resp.ID, &id); err != nil {
		t.Fatalf("failed to parse string ID: %v", err)
	}
	if id != "my-request-id" {
		t.Errorf("response ID = %q, want my-request-id", id)
	}
}

func TestIntegration_ManifestEndpoint(t *testing.T) {
	h := newSeededMCPHandler()

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	rec := httptest.NewRecorder()
	h.ServeManifest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, want 200", rec.Code)
	}

	var manifest MCPManifest
	json.NewDecoder(rec.Body).Decode(&manifest)

	// Verify manifest has all the tools that the actual handler exposes
	executor := NewMCPToolExecutor(seedAgentStore(), seedPromptStore(), seedMCPServerStore(), seedModelConfigStore(), seedModelEndpointStore(), "https://registry.example.com")
	actualTools := executor.ListTools()

	if len(manifest.Tools) != len(actualTools) {
		t.Errorf("manifest has %d tools, handler has %d — mismatch", len(manifest.Tools), len(actualTools))
	}
	for i, mt := range manifest.Tools {
		if i < len(actualTools) && mt.Name != actualTools[i].Name {
			t.Errorf("manifest tool[%d] = %q, handler tool[%d] = %q", i, mt.Name, i, actualTools[i].Name)
		}
	}

	// Verify transport URL
	transport, ok := manifest.Transport["streamableHttp"].(map[string]interface{})
	if !ok {
		t.Fatal("expected streamableHttp transport")
	}
	url := transport["url"].(string)
	if url != "https://registry.example.com/mcp/v1" {
		t.Errorf("transport URL = %q", url)
	}

	// Verify auth
	if manifest.Authentication["type"] != "bearer" {
		t.Errorf("auth type = %v", manifest.Authentication["type"])
	}
}

// =============================================================================
// Task #8: Error Handling and Edge Case Testing
// =============================================================================

func TestEdgeCase_ToolsCall_EmptyAgentID(t *testing.T) {
	h := newSeededMCPHandler()
	resp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
		"name":      "get_agent",
		"arguments": map[string]interface{}{"agent_id": ""},
	})
	if resp.Error == nil {
		// Empty agent_id should return InvalidParams error
		var result mcp.ToolResult
		json.Unmarshal(resp.Result, &result)
		// Or it may be caught at the tool level
		if !result.IsError {
			t.Error("expected error for empty agent_id")
		}
	}
}

func TestEdgeCase_ToolsCall_NonexistentAgent(t *testing.T) {
	h := newSeededMCPHandler()
	resp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
		"name":      "get_agent",
		"arguments": map[string]interface{}{"agent_id": "00000000-0000-0000-0000-000000000000"},
	})
	if resp.Error != nil {
		t.Fatalf("expected tool error (isError=true), not JSON-RPC error: %s", resp.Error.Message)
	}
	var result mcp.ToolResult
	json.Unmarshal(resp.Result, &result)
	if !result.IsError {
		t.Error("expected isError=true for nonexistent agent")
	}
	// Verify the error message text exists and is meaningful
	if len(result.Content) == 0 || result.Content[0].Text == "" {
		t.Error("error tool result should have content with error message")
	}
}

func TestEdgeCase_ToolsCall_SpecialCharAgentID(t *testing.T) {
	h := newSeededMCPHandler()

	specialIDs := []string{
		"../etc/passwd",
		"<script>alert(1)</script>",
		"'; DROP TABLE agents; --",
		"agent%20with%20spaces",
		"",
		"\x00null\x00byte",
	}

	for _, id := range specialIDs {
		t.Run(id, func(t *testing.T) {
			resp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
				"name":      "get_agent",
				"arguments": map[string]interface{}{"agent_id": id},
			})
			// Should either be InvalidParams (empty) or tool error (not found) — never a 500
			if resp.Error != nil {
				if resp.Error.Code != mcp.InvalidParams {
					t.Errorf("unexpected JSON-RPC error code %d for id=%q", resp.Error.Code, id)
				}
				return
			}
			var result mcp.ToolResult
			json.Unmarshal(resp.Result, &result)
			if !result.IsError && id == "" {
				t.Error("empty agent_id should be an error")
			}
		})
	}
}

func TestEdgeCase_ResourcesRead_SpecialCharURIs(t *testing.T) {
	h := newSeededMCPHandler()

	malformedURIs := []string{
		"",
		"agent://",
		"prompt://",
		"prompt:///active",
		"prompt://code-agent",          // missing /active suffix
		"prompt://code-agent/latest",   // wrong suffix
		"file:///etc/passwd",
		"http://example.com",
		"javascript:alert(1)",
		"agent://../../secret",
		"config://unknown",
		"ftp://malicious.com",
	}

	for _, uri := range malformedURIs {
		t.Run(uri, func(t *testing.T) {
			resp := mcpRPC(t, h.HandlePost, 1, "resources/read", map[string]interface{}{
				"uri": uri,
			})
			if resp.Error == nil {
				t.Errorf("expected error for malformed URI %q", uri)
			} else if resp.Error.Code != mcp.InvalidParams {
				t.Errorf("expected InvalidParams (%d), got %d for URI %q", mcp.InvalidParams, resp.Error.Code, uri)
			}
		})
	}
}

func TestEdgeCase_InvalidJSON(t *testing.T) {
	h := newSeededMCPHandler()

	invalidPayloads := []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"plain text", "hello world"},
		{"truncated JSON", `{"jsonrpc":"2.0","id":1,"meth`},
		{"nested invalid", `{"jsonrpc":"2.0","id":1,"method":"ping","params":{invalid}}`},
		{"only whitespace", "   \n\t  "},
		{"null", "null"},
	}

	for _, tc := range invalidPayloads {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandlePost(rec, req)

			body, _ := io.ReadAll(rec.Body)
			if len(strings.TrimSpace(string(body))) == 0 && tc.body == "" {
				// Empty body may result in parse error
				return
			}

			var resp mcp.JSONRPCResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				// If can't parse response, that's OK as long as we got an HTTP error
				return
			}
			if resp.Error == nil {
				t.Errorf("expected error for invalid JSON: %q", tc.body)
			}
		})
	}
}

func TestEdgeCase_WrongHTTPMethod(t *testing.T) {
	h := newSeededMCPHandler()

	// SSE (GET on /mcp) returns 405
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	h.HandleSSE(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp status = %d, want 405", rec.Code)
	}
}

func TestEdgeCase_WrongContentType(t *testing.T) {
	h := newSeededMCPHandler()

	contentTypes := []string{
		"text/plain",
		"text/html",
		"application/xml",
		"multipart/form-data",
		"",
	}

	for _, ct := range contentTypes {
		t.Run(ct, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp",
				strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			rec := httptest.NewRecorder()
			h.HandlePost(rec, req)

			if rec.Code != http.StatusUnsupportedMediaType {
				t.Errorf("Content-Type %q: status = %d, want 415", ct, rec.Code)
			}
		})
	}
}

func TestEdgeCase_StoreErrorPropagation(t *testing.T) {
	// Verify store errors become tool errors (isError=true), NOT JSON-RPC errors
	agentStore := &mockAgentStoreForMCPTools{
		listFn: func(_ context.Context, _ bool, _, _ int) ([]store.Agent, int, error) {
			return nil, 0, errors.New("database connection refused")
		},
		getByIDFn: func(_ context.Context, _ string) (*store.Agent, error) {
			return nil, errors.New("timeout reading from primary")
		},
	}
	mcpStore := &mockMCPServerStoreForMCPTools{
		listFn: func(_ context.Context) ([]store.MCPServer, error) {
			return nil, errors.New("connection pool exhausted")
		},
	}
	modelStore := &mockModelConfigStoreForMCPTools{
		getByScopeFn: func(_ context.Context, _, _ string) (*store.ModelConfig, error) {
			return nil, errors.New("row scan failed")
		},
	}

	exec := NewMCPToolExecutor(agentStore, nil, mcpStore, modelStore, nil, "https://registry.example.com")

	tests := []struct {
		tool string
		args json.RawMessage
	}{
		{"list_agents", json.RawMessage(`{}`)},
		{"get_agent", json.RawMessage(`{"agent_id":"test"}`)},
		{"list_mcp_servers", json.RawMessage(`{}`)},
		{"get_model_config", json.RawMessage(`{}`)},
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			result, rpcErr := exec.CallTool(context.Background(), tc.tool, tc.args)
			if rpcErr != nil {
				t.Fatalf("store errors should NOT be JSON-RPC errors: %+v", rpcErr)
			}
			if result == nil {
				t.Fatal("expected tool result")
			}
			if !result.IsError {
				t.Error("expected isError=true for store failure")
			}
			// Verify error message doesn't leak stack traces
			msg := result.Content[0].Text
			if strings.Contains(msg, "goroutine") || strings.Contains(msg, "runtime.") {
				t.Error("error message should not contain stack traces")
			}
		})
	}
}

func TestEdgeCase_ListAgents_BoundaryLimits(t *testing.T) {
	h := newSeededMCPHandler()

	tests := []struct {
		name  string
		limit interface{}
	}{
		{"zero limit", 0},
		{"negative limit", -1},
		{"very large limit", 999999},
		{"max int", 2147483647},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
				"name":      "list_agents",
				"arguments": map[string]interface{}{"limit": tc.limit},
			})
			if resp.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: %s", resp.Error.Message)
			}
			var result mcp.ToolResult
			json.Unmarshal(resp.Result, &result)
			if result.IsError {
				t.Errorf("unexpected tool error for limit=%v: %s", tc.limit, result.Content[0].Text)
			}
		})
	}
}

func TestEdgeCase_EmptyBatch(t *testing.T) {
	h := newSeededMCPHandler()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`[]`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePost(rec, req)

	body, _ := io.ReadAll(rec.Body)
	var resp mcp.JSONRPCResponse
	json.Unmarshal(body, &resp)
	if resp.Error == nil {
		t.Error("empty batch should return error")
	}
	if resp.Error.Code != mcp.InvalidRequest {
		t.Errorf("empty batch error code = %d, want %d", resp.Error.Code, mcp.InvalidRequest)
	}
}

func TestEdgeCase_ToolsCall_MissingParamsField(t *testing.T) {
	h := newSeededMCPHandler()

	// tools/call with no params at all
	resp := mcpRPC(t, h.HandlePost, 1, "tools/call", nil)
	if resp.Error == nil {
		t.Fatal("expected error for tools/call with no params")
	}
	if resp.Error.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, mcp.InvalidParams)
	}
}

func TestEdgeCase_ResourcesRead_MissingParams(t *testing.T) {
	h := newSeededMCPHandler()

	resp := mcpRPC(t, h.HandlePost, 1, "resources/read", nil)
	if resp.Error == nil {
		t.Fatal("expected error for resources/read with no params")
	}
	if resp.Error.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, mcp.InvalidParams)
	}
}

func TestEdgeCase_PromptsGet_MissingParams(t *testing.T) {
	h := newSeededMCPHandler()

	resp := mcpRPC(t, h.HandlePost, 1, "prompts/get", nil)
	if resp.Error == nil {
		t.Fatal("expected error for prompts/get with no params")
	}
	if resp.Error.Code != mcp.InvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, mcp.InvalidParams)
	}
}

// =============================================================================
// Task #9: Data Validation and Consistency Testing
// =============================================================================

func TestDataValidation_ToolDefinitionIntegrity(t *testing.T) {
	exec := NewMCPToolExecutor(seedAgentStore(), seedPromptStore(), seedMCPServerStore(), seedModelConfigStore(), seedModelEndpointStore(), "https://registry.example.com")
	tools := exec.ListTools()

	if len(tools) != 5 {
		t.Fatalf("expected exactly 5 tools, got %d", len(tools))
	}

	// Verify uniqueness
	names := make(map[string]bool)
	for _, tool := range tools {
		if names[tool.Name] {
			t.Errorf("duplicate tool name: %s", tool.Name)
		}
		names[tool.Name] = true

		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}

		// Validate input schema is valid JSON Schema
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Errorf("tool %s has invalid JSON Schema: %v", tool.Name, err)
			continue
		}

		if schema["type"] != "object" {
			t.Errorf("tool %s schema type = %v, want object", tool.Name, schema["type"])
		}

		// Verify properties key exists
		if _, ok := schema["properties"]; !ok {
			t.Errorf("tool %s schema missing 'properties' key", tool.Name)
		}
	}

	// Verify expected tools by name
	expected := []string{"list_agents", "get_agent", "get_discovery", "list_mcp_servers", "get_model_config"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected tool %q not found", name)
		}
	}
}

func TestDataValidation_ToolSchemas_RequiredFields(t *testing.T) {
	exec := NewMCPToolExecutor(seedAgentStore(), seedPromptStore(), seedMCPServerStore(), seedModelConfigStore(), seedModelEndpointStore(), "https://registry.example.com")
	tools := exec.ListTools()

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			var schema map[string]interface{}
			json.Unmarshal(tool.InputSchema, &schema)

			// get_agent should have required: ["agent_id"]
			if tool.Name == "get_agent" {
				required, ok := schema["required"].([]interface{})
				if !ok {
					t.Fatal("get_agent schema missing 'required' field")
				}
				found := false
				for _, r := range required {
					if r == "agent_id" {
						found = true
					}
				}
				if !found {
					t.Error("get_agent schema should require 'agent_id'")
				}
			}

			// Verify property descriptions exist
			props, ok := schema["properties"].(map[string]interface{})
			if !ok {
				return // empty properties is valid for tools with no params
			}
			for propName, propValue := range props {
				propMap, ok := propValue.(map[string]interface{})
				if !ok {
					t.Errorf("property %q is not an object", propName)
					continue
				}
				if propMap["type"] == nil {
					t.Errorf("property %q missing 'type'", propName)
				}
				if propMap["description"] == nil {
					t.Errorf("property %q missing 'description'", propName)
				}
			}
		})
	}
}

func TestDataValidation_ResourceURIs(t *testing.T) {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	modelConfig := seedModelConfigStore()

	provider := NewMCPResourceProvider(agents, prompts, modelConfig)

	resources, err := provider.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources error: %v", err)
	}

	// Verify URI format for each resource
	for _, r := range resources {
		if r.URI == "" {
			t.Error("resource has empty URI")
			continue
		}
		if r.Name == "" {
			t.Errorf("resource %q has empty name", r.URI)
		}

		// Verify URI is parseable by ReadResource
		if strings.HasPrefix(r.URI, "agent://") {
			content, rpcErr := provider.ReadResource(context.Background(), r.URI)
			if rpcErr != nil {
				t.Errorf("could not read listed resource %q: %s", r.URI, rpcErr.Message)
				continue
			}
			if content.URI != r.URI {
				t.Errorf("read URI = %q, list URI = %q — mismatch", content.URI, r.URI)
			}
			if content.MimeType != r.MimeType {
				t.Errorf("read MimeType = %q, list MimeType = %q — mismatch", content.MimeType, r.MimeType)
			}
		}

		if strings.HasPrefix(r.URI, "config://") {
			content, rpcErr := provider.ReadResource(context.Background(), r.URI)
			if rpcErr != nil {
				t.Errorf("could not read config resource %q: %s", r.URI, rpcErr.Message)
				continue
			}
			if content.MimeType != "application/json" {
				t.Errorf("config resource MIME = %q, want application/json", content.MimeType)
			}
			// Verify valid JSON
			var parsed interface{}
			if err := json.Unmarshal([]byte(content.Text), &parsed); err != nil {
				t.Errorf("config resource %q content is not valid JSON: %v", r.URI, err)
			}
		}
	}
}

func TestDataValidation_ResourceTemplates(t *testing.T) {
	provider := NewMCPResourceProvider(nil, nil, nil)
	templates := provider.ListTemplates()

	if len(templates) != 4 {
		t.Fatalf("expected 4 templates, got %d", len(templates))
	}

	for _, tmpl := range templates {
		if tmpl.URITemplate == "" {
			t.Error("template has empty URITemplate")
		}
		if tmpl.Name == "" {
			t.Errorf("template %q has empty name", tmpl.URITemplate)
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", tmpl.URITemplate)
		}
		if tmpl.MimeType == "" {
			t.Errorf("template %q has empty MimeType", tmpl.URITemplate)
		}
	}

	// Verify template URIs contain proper RFC 6570 variable syntax
	expectedPatterns := map[string]string{
		"agent://{agentId}":        "agentId",
		"prompt://{agentId}/active": "agentId",
		"config://model":           "",
		"config://context":         "",
	}
	for _, tmpl := range templates {
		if expected, ok := expectedPatterns[tmpl.URITemplate]; ok {
			if expected != "" && !strings.Contains(tmpl.URITemplate, "{"+expected+"}") {
				t.Errorf("template %q missing variable {%s}", tmpl.URITemplate, expected)
			}
		}
	}
}

func TestDataValidation_PromptTemplateSubstitution(t *testing.T) {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	provider := NewMCPPromptProvider(agents, prompts)

	// Get prompt without substitution — template vars should remain
	result, rpcErr := provider.GetPrompt(context.Background(), "search-agent", nil)
	if rpcErr != nil {
		t.Fatalf("GetPrompt error: %+v", rpcErr)
	}
	rawText := result.Messages[0].Content.Text
	if !strings.Contains(rawText, "{{query}}") {
		t.Error("unsubstituted text should contain {{query}}")
	}
	if !strings.Contains(rawText, "{{topic}}") {
		t.Error("unsubstituted text should contain {{topic}}")
	}

	// Full substitution
	result, rpcErr = provider.GetPrompt(context.Background(), "search-agent", map[string]string{
		"query": "golang MCP",
		"topic": "programming",
	})
	if rpcErr != nil {
		t.Fatalf("GetPrompt with args error: %+v", rpcErr)
	}
	text := result.Messages[0].Content.Text
	if strings.Contains(text, "{{") {
		t.Errorf("fully substituted text should not contain {{ : %q", text)
	}
	if !strings.Contains(text, "golang MCP") {
		t.Error("expected 'golang MCP' in substituted text")
	}

	// Partial substitution — only supply 'query'
	result, rpcErr = provider.GetPrompt(context.Background(), "search-agent", map[string]string{
		"query": "test",
	})
	if rpcErr != nil {
		t.Fatalf("GetPrompt partial error: %+v", rpcErr)
	}
	partialText := result.Messages[0].Content.Text
	if strings.Contains(partialText, "{{query}}") {
		t.Error("substituted variable should be replaced")
	}
	if !strings.Contains(partialText, "{{topic}}") {
		t.Error("unsubstituted variable should remain as {{topic}}")
	}
}

func TestDataValidation_PromptListArguments(t *testing.T) {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	provider := NewMCPPromptProvider(agents, prompts)

	defs, err := provider.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("ListPrompts error: %v", err)
	}

	// search-agent has 2 template vars
	var searchDef *MCPPromptDefinition
	for i, d := range defs {
		if d.Name == "search-agent" {
			searchDef = &defs[i]
			break
		}
	}
	if searchDef == nil {
		t.Fatal("search-agent prompt not found")
	}
	if len(searchDef.Arguments) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(searchDef.Arguments))
	}

	// Verify argument structure
	queryArg := searchDef.Arguments[0]
	if queryArg.Name != "query" {
		t.Errorf("first arg name = %q, want query", queryArg.Name)
	}
	if !queryArg.Required {
		t.Error("query arg should be required")
	}
	if queryArg.Description == "" {
		t.Error("query arg should have description")
	}
}

func TestDataConsistency_ListVsIndividualAgents(t *testing.T) {
	h := newSeededMCPHandler()

	// Get list of agents via tool
	listResp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
		"name":      "list_agents",
		"arguments": map[string]interface{}{},
	})
	if listResp.Error != nil {
		t.Fatalf("list_agents error: %s", listResp.Error.Message)
	}

	var listResult mcp.ToolResult
	json.Unmarshal(listResp.Result, &listResult)
	var listData map[string]interface{}
	json.Unmarshal([]byte(listResult.Content[0].Text), &listData)
	agentList := listData["agents"].([]interface{})

	// For each agent in list, verify individual get returns the same core fields
	for _, agentRaw := range agentList {
		agent := agentRaw.(map[string]interface{})
		id := agent["id"].(string)

		getResp := mcpRPC(t, h.HandlePost, 2, "tools/call", map[string]interface{}{
			"name":      "get_agent",
			"arguments": map[string]interface{}{"agent_id": id},
		})
		if getResp.Error != nil {
			t.Errorf("get_agent(%s) error: %s", id, getResp.Error.Message)
			continue
		}

		var getResult mcp.ToolResult
		json.Unmarshal(getResp.Result, &getResult)
		var getAgent map[string]interface{}
		json.Unmarshal([]byte(getResult.Content[0].Text), &getAgent)

		// Verify core fields match
		if agent["id"] != getAgent["id"] {
			t.Errorf("agent %s: list ID=%v, get ID=%v", id, agent["id"], getAgent["id"])
		}
		if agent["name"] != getAgent["name"] {
			t.Errorf("agent %s: list name=%v, get name=%v", id, agent["name"], getAgent["name"])
		}
		if agent["is_active"] != getAgent["is_active"] {
			t.Errorf("agent %s: list is_active=%v, get is_active=%v", id, agent["is_active"], getAgent["is_active"])
		}
	}
}

func TestDataConsistency_DiscoveryMatchesIndividual(t *testing.T) {
	h := newSeededMCPHandler()

	// Get discovery payload
	discResp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
		"name":      "get_discovery",
		"arguments": map[string]interface{}{},
	})
	if discResp.Error != nil {
		t.Fatalf("get_discovery error: %s", discResp.Error.Message)
	}
	var discResult mcp.ToolResult
	json.Unmarshal(discResp.Result, &discResult)
	var discData map[string]interface{}
	json.Unmarshal([]byte(discResult.Content[0].Text), &discData)

	// Verify all required sections are present
	requiredKeys := []string{"agents", "mcp_servers", "model_config", "model_endpoints", "fetched_at"}
	for _, key := range requiredKeys {
		if _, ok := discData[key]; !ok {
			t.Errorf("discovery missing key: %s", key)
		}
	}

	// Verify agent count matches list_agents
	listResp := mcpRPC(t, h.HandlePost, 2, "tools/call", map[string]interface{}{
		"name":      "list_agents",
		"arguments": map[string]interface{}{},
	})
	var listResult mcp.ToolResult
	json.Unmarshal(listResp.Result, &listResult)
	var listData map[string]interface{}
	json.Unmarshal([]byte(listResult.Content[0].Text), &listData)

	discAgents := discData["agents"].([]interface{})
	listAgents := listData["agents"].([]interface{})
	if len(discAgents) != len(listAgents) {
		t.Errorf("discovery has %d agents, list has %d", len(discAgents), len(listAgents))
	}

	// Verify server count matches list_mcp_servers
	serversResp := mcpRPC(t, h.HandlePost, 3, "tools/call", map[string]interface{}{
		"name":      "list_mcp_servers",
		"arguments": map[string]interface{}{},
	})
	var serversResult mcp.ToolResult
	json.Unmarshal(serversResp.Result, &serversResult)
	var serversData map[string]interface{}
	json.Unmarshal([]byte(serversResult.Content[0].Text), &serversData)

	discServers := discData["mcp_servers"].([]interface{})
	listServers := serversData["servers"].([]interface{})
	if len(discServers) != len(listServers) {
		t.Errorf("discovery has %d servers, list has %d", len(discServers), len(listServers))
	}
}

func TestDataConsistency_CredentialStripping(t *testing.T) {
	h := newSeededMCPHandler()

	// Test via list_mcp_servers tool
	resp := mcpRPC(t, h.HandlePost, 1, "tools/call", map[string]interface{}{
		"name":      "list_mcp_servers",
		"arguments": map[string]interface{}{},
	})
	var result mcp.ToolResult
	json.Unmarshal(resp.Result, &result)
	serverJSON := result.Content[0].Text

	if strings.Contains(serverJSON, "super-secret-token") {
		t.Error("MCP server credentials leaked in list_mcp_servers tool response")
	}
	if strings.Contains(serverJSON, "auth_credential") {
		t.Error("auth_credential field should not appear in response")
	}

	// Test via get_discovery tool
	discResp := mcpRPC(t, h.HandlePost, 2, "tools/call", map[string]interface{}{
		"name":      "get_discovery",
		"arguments": map[string]interface{}{},
	})
	var discResult mcp.ToolResult
	json.Unmarshal(discResp.Result, &discResult)
	discJSON := discResult.Content[0].Text

	if strings.Contains(discJSON, "super-secret-token") {
		t.Error("MCP server credentials leaked in discovery response")
	}

	// Model endpoint headers should be redacted
	if strings.Contains(discJSON, "sk-secret") {
		t.Error("model endpoint authorization headers leaked in discovery response")
	}
	if !strings.Contains(discJSON, "***REDACTED***") {
		t.Error("expected redacted headers in model endpoint config")
	}
}

func TestDataConsistency_ResourcesMatchAgents(t *testing.T) {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	modelConfig := seedModelConfigStore()

	provider := NewMCPResourceProvider(agents, prompts, modelConfig)

	resources, err := provider.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources error: %v", err)
	}

	// Count agent and prompt resources
	agentResources := 0
	promptResources := 0
	configResources := 0
	for _, r := range resources {
		switch {
		case strings.HasPrefix(r.URI, "agent://"):
			agentResources++
		case strings.HasPrefix(r.URI, "prompt://"):
			promptResources++
		case strings.HasPrefix(r.URI, "config://"):
			configResources++
		}
	}

	// There should be equal numbers of agent:// and prompt:// resources
	if agentResources != promptResources {
		t.Errorf("agent resources (%d) != prompt resources (%d)", agentResources, promptResources)
	}

	// Should be exactly 2 config resources (model + context)
	if configResources != 2 {
		t.Errorf("expected 2 config resources, got %d", configResources)
	}

	// Each agent resource should have a corresponding prompt resource
	for _, r := range resources {
		if strings.HasPrefix(r.URI, "agent://") {
			agentID := strings.TrimPrefix(r.URI, "agent://")
			promptURI := "prompt://" + agentID + "/active"
			found := false
			for _, pr := range resources {
				if pr.URI == promptURI {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("agent %s has no corresponding prompt resource", agentID)
			}
		}
	}
}

func TestDataConsistency_ContextConfigDerivedCorrectly(t *testing.T) {
	modelConfig := seedModelConfigStore()
	provider := NewMCPResourceProvider(nil, nil, modelConfig)

	// Read model config
	modelContent, rpcErr := provider.ReadResource(context.Background(), "config://model")
	if rpcErr != nil {
		t.Fatalf("read config://model error: %+v", rpcErr)
	}

	// Read context config
	contextContent, rpcErr := provider.ReadResource(context.Background(), "config://context")
	if rpcErr != nil {
		t.Fatalf("read config://context error: %+v", rpcErr)
	}

	var modelData map[string]interface{}
	json.Unmarshal([]byte(modelContent.Text), &modelData)

	var contextData contextConfig
	json.Unmarshal([]byte(contextContent.Text), &contextData)

	// Context config values should match model config
	modelCtxWindow, _ := modelData["default_context_window"].(float64)
	if contextData.DefaultContextWindow != int(modelCtxWindow) {
		t.Errorf("context_window: model=%v, context=%d", modelCtxWindow, contextData.DefaultContextWindow)
	}

	modelMaxOutput, _ := modelData["default_max_output_tokens"].(float64)
	if contextData.DefaultMaxOutputTokens != int(modelMaxOutput) {
		t.Errorf("max_output: model=%v, context=%d", modelMaxOutput, contextData.DefaultMaxOutputTokens)
	}

	// Context config should NOT contain model-specific fields
	var rawContext map[string]interface{}
	json.Unmarshal([]byte(contextContent.Text), &rawContext)
	modelOnlyFields := []string{"default_model", "temperature", "max_tokens", "max_tool_rounds", "embedding_model"}
	for _, field := range modelOnlyFields {
		if _, exists := rawContext[field]; exists {
			t.Errorf("context config should not contain %q", field)
		}
	}
}

func TestDataConsistency_ManifestMatchesTools(t *testing.T) {
	h := newSeededMCPHandler()

	// Get manifest
	manifestReq := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	manifestRec := httptest.NewRecorder()
	h.ServeManifest(manifestRec, manifestReq)

	var manifest MCPManifest
	json.NewDecoder(manifestRec.Body).Decode(&manifest)

	// Get tools via protocol
	resp := mcpRPC(t, h.HandlePost, 1, "tools/list", nil)
	var toolsList map[string]interface{}
	json.Unmarshal(resp.Result, &toolsList)
	tools := toolsList["tools"].([]interface{})

	// Manifest tool count should match
	if len(manifest.Tools) != len(tools) {
		t.Errorf("manifest tools (%d) != protocol tools (%d)", len(manifest.Tools), len(tools))
	}

	// Verify names match
	manifestNames := make(map[string]bool)
	for _, mt := range manifest.Tools {
		manifestNames[mt.Name] = true
	}
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		name := toolMap["name"].(string)
		if !manifestNames[name] {
			t.Errorf("tool %q in protocol but not in manifest", name)
		}
	}
}

func TestDataValidation_PromptResultMessageStructure(t *testing.T) {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	provider := NewMCPPromptProvider(agents, prompts)

	result, rpcErr := provider.GetPrompt(context.Background(), "search-agent", nil)
	if rpcErr != nil {
		t.Fatalf("GetPrompt error: %+v", rpcErr)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	msg := result.Messages[0]
	if msg.Role != "user" {
		t.Errorf("role = %q, want user", msg.Role)
	}
	if msg.Content.Type != "text" {
		t.Errorf("content type = %q, want text", msg.Content.Type)
	}
	if msg.Content.Text == "" {
		t.Error("content text should not be empty")
	}

	// Verify description includes agent name and version
	if result.Description == "" {
		t.Error("expected non-empty description")
	}
	if !strings.Contains(result.Description, "Search Agent") {
		t.Errorf("description should mention agent name: %q", result.Description)
	}
}

func TestDataConsistency_AllToolsExecutable(t *testing.T) {
	// Verify that every tool returned by ListTools can actually be called
	exec := NewMCPToolExecutor(seedAgentStore(), seedPromptStore(), seedMCPServerStore(), seedModelConfigStore(), seedModelEndpointStore(), "https://registry.example.com")
	tools := exec.ListTools()

	toolArgs := map[string]json.RawMessage{
		"list_agents":      json.RawMessage(`{}`),
		"get_agent":        json.RawMessage(`{"agent_id":"search-agent"}`),
		"get_discovery":    json.RawMessage(`{}`),
		"list_mcp_servers": json.RawMessage(`{}`),
		"get_model_config": json.RawMessage(`{}`),
	}

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			args, ok := toolArgs[tool.Name]
			if !ok {
				t.Fatalf("no test args for tool %s", tool.Name)
			}

			result, rpcErr := exec.CallTool(context.Background(), tool.Name, args)
			if rpcErr != nil {
				t.Fatalf("tool %s returned JSON-RPC error: %+v", tool.Name, rpcErr)
			}
			if result == nil {
				t.Fatalf("tool %s returned nil result", tool.Name)
			}
			if result.IsError {
				t.Fatalf("tool %s returned error: %s", tool.Name, result.Content[0].Text)
			}

			// Verify result content is valid JSON
			if len(result.Content) == 0 {
				t.Fatalf("tool %s returned empty content", tool.Name)
			}
			if result.Content[0].Type != "text" {
				t.Errorf("tool %s content type = %q, want text", tool.Name, result.Content[0].Type)
			}
			var parsed interface{}
			if err := json.Unmarshal([]byte(result.Content[0].Text), &parsed); err != nil {
				t.Errorf("tool %s content is not valid JSON: %v", tool.Name, err)
			}
		})
	}
}

func TestDataConsistency_PromptsListMatchesGet(t *testing.T) {
	agents := seedAgentStore()
	prompts := seedPromptStore()
	provider := NewMCPPromptProvider(agents, prompts)

	defs, err := provider.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("ListPrompts error: %v", err)
	}

	for _, def := range defs {
		t.Run(def.Name, func(t *testing.T) {
			result, rpcErr := provider.GetPrompt(context.Background(), def.Name, nil)
			if rpcErr != nil {
				t.Fatalf("GetPrompt(%q) error: %+v", def.Name, rpcErr)
			}
			if result == nil {
				t.Fatal("GetPrompt returned nil result")
			}
			if len(result.Messages) == 0 {
				t.Error("GetPrompt returned no messages")
			}
		})
	}
}
