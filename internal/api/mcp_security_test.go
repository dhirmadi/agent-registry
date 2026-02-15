package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/mcp"
)

// =============================================================================
// RED TEAM SECURITY TESTS — MCP Handler (API Layer)
// =============================================================================
//
// Task #3: Authentication and Authorization bypass via router
// Task #4: Input validation/injection at handler dispatch level
// Task #5: Resource exhaustion via handler-level amplification
// =============================================================================

// --- Mock auth middleware for security testing ---

func withAuthContext(r *http.Request, userID uuid.UUID, role string) *http.Request {
	ctx := auth.ContextWithUser(r.Context(), userID, role, "apikey")
	return r.WithContext(ctx)
}

func mcpSecurityRouter(mcpHandler *MCPHandler, requireAuth bool, role string) chi.Router {
	r := chi.NewRouter()

	if requireAuth {
		r.Route("/mcp", func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					userRole, ok := auth.UserRoleFromContext(req.Context())
					if !ok {
						http.Error(w, `{"error":"auth required"}`, http.StatusUnauthorized)
						return
					}
					// Check role
					allowed := map[string]bool{"viewer": true, "editor": true, "admin": true}
					if !allowed[userRole] {
						http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
						return
					}
					next.ServeHTTP(w, req)
				})
			})
			r.Post("/", mcpHandler.HandlePost)
			r.Get("/", mcpHandler.HandleSSE)
			r.Delete("/", mcpHandler.HandleDelete)
		})
	} else {
		r.Post("/mcp", mcpHandler.HandlePost)
		r.Get("/mcp", mcpHandler.HandleSSE)
		r.Delete("/mcp", mcpHandler.HandleDelete)
	}

	r.Get("/mcp.json", mcpHandler.ServeManifest)
	return r
}

// --- Helper: post to router with optional auth ---

func mcpSecPost(t *testing.T, router chi.Router, body string, userID *uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userID != nil {
		req = withAuthContext(req, *userID, role)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// =============================================================================
// TASK #3: Auth/Authz at Router Level
// =============================================================================

func TestSecurityMCP_NoAuth_PostReturns401(t *testing.T) {
	h := newTestMCPHandler()
	router := mcpSecurityRouter(h, true, "viewer")

	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	w := mcpSecPost(t, router, body, nil, "")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}
}

func TestSecurityMCP_InvalidRole_PostReturnsForbidden(t *testing.T) {
	h := newTestMCPHandler()
	router := mcpSecurityRouter(h, true, "viewer")

	uid := uuid.New()
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`

	// Set an invalid role in context
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.ContextWithUser(req.Context(), uid, "unknown_role", "apikey")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unknown role, got %d", w.Code)
	}
}

func TestSecurityMCP_ViewerRole_CanAccessMCP(t *testing.T) {
	h := newTestMCPHandler()
	router := mcpSecurityRouter(h, true, "viewer")

	uid := uuid.New()
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	w := mcpSecPost(t, router, body, &uid, "viewer")

	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("viewer should access MCP, got %d", w.Code)
	}
}

func TestSecurityMCP_ManifestNoAuth(t *testing.T) {
	h := newTestMCPHandler()
	router := mcpSecurityRouter(h, true, "viewer")

	// GET /mcp.json should work without auth
	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Error("manifest should be accessible without auth")
	}
}

func TestSecurityMCP_SSE_Returns405(t *testing.T) {
	h := newTestMCPHandler()
	router := mcpSecurityRouter(h, true, "viewer")

	uid := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req = withAuthContext(req, uid, "viewer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("SSE endpoint should return 405, got %d", w.Code)
	}
}

func TestSecurityMCP_Delete_NoAuth(t *testing.T) {
	h := newTestMCPHandler()
	router := mcpSecurityRouter(h, true, "viewer")

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("DELETE without auth should return 401, got %d", w.Code)
	}
}

func TestSecurityMCP_RoleEscalation_ViewerCallingAdminTools(t *testing.T) {
	// Viewer calls tools that read data — this should succeed because
	// MCP tools are read-only. But we verify no write operations are possible.
	executedTool := ""
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, name string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				executedTool = name
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: `{"ok":true}`}},
				}, nil
			},
		},
		nil, nil, nil,
	)
	router := mcpSecurityRouter(h, true, "viewer")

	uid := uuid.New()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_discovery","arguments":{}}}`
	w := mcpSecPost(t, router, body, &uid, "viewer")

	if w.Code == http.StatusForbidden {
		t.Log("[OK] Viewer cannot call tools (strict role enforcement)")
	} else if w.Code == http.StatusOK {
		t.Logf("[INFO] Viewer can call tool %q — verify all MCP tools are read-only", executedTool)
	}
}

// =============================================================================
// TASK #4: Injection through MCP handler dispatch
// =============================================================================

func TestSecurityMCP_ToolCallInjection_SQLInAgentID(t *testing.T) {
	var receivedID string
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, name string, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				if name == "get_agent" {
					var p struct{ AgentID string `json:"agent_id"` }
					json.Unmarshal(args, &p)
					receivedID = p.AgentID
				}
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: `{"error":"not found"}`}},
					IsError: true,
				}, nil
			},
		},
		nil, nil, nil,
	)

	injections := []string{
		"' OR '1'='1",
		"1; DROP TABLE agents; --",
		"1 UNION SELECT * FROM users",
		"' AND extractvalue(1,concat(0x7e,version())) --",
		"1' AND sleep(5) --",
	}

	for _, injection := range injections {
		t.Run("sql_"+injection[:min(20, len(injection))], func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"agent_id": injection})
			params, _ := json.Marshal(map[string]interface{}{
				"name":      "get_agent",
				"arguments": json.RawMessage(args),
			})
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + string(params) + `}`
			resp := mcpPost(t, h.HandlePost, body)

			// The tool should process without crashing
			if resp.Error != nil && resp.Error.Code == mcp.InternalError {
				t.Errorf("[CRITICAL] SQL injection caused internal error: %s", resp.Error.Message)
			}

			// Verify the raw injection string was passed through (parameterized query safety depends on store layer)
			if receivedID != injection {
				t.Logf("[INFO] agent_id was transformed: sent=%q, received=%q", injection, receivedID)
			}
		})
	}
}

func TestSecurityMCP_ToolCallInjection_PathTraversal(t *testing.T) {
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, _ string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: `{"error":"not found"}`}},
					IsError: true,
				}, nil
			},
		},
		nil, nil, nil,
	)

	paths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"....//....//....//etc/passwd",
		"/proc/self/environ",
		"/dev/null",
	}

	for _, p := range paths {
		t.Run("path_"+p[:min(20, len(p))], func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"agent_id": p})
			params, _ := json.Marshal(map[string]interface{}{
				"name":      "get_agent",
				"arguments": json.RawMessage(args),
			})
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + string(params) + `}`
			resp := mcpPost(t, h.HandlePost, body)

			if resp.Error != nil && resp.Error.Code == mcp.InternalError {
				t.Errorf("[CRITICAL] Path traversal caused internal error: %s", resp.Error.Message)
			}
		})
	}
}

func TestSecurityMCP_ResourceRead_URISchemeInjection(t *testing.T) {
	h := NewMCPHandler(
		nil,
		&mockMCPResourceProviderForHandler{},
		nil, nil,
	)

	maliciousURIs := []string{
		"file:///etc/passwd",
		"file:///proc/self/environ",
		"ftp://evil.com/malware.bin",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"http://169.254.169.254/latest/meta-data/",
		"http://0.0.0.0:8080/admin",
		"ldap://evil.com/dc=evil,dc=com",
		"gopher://evil.com:70/1",
		"dict://evil.com:2628/",
		"",
		"agent://",
		"agent://" + strings.Repeat("x", 100000),
		"config://../../secret",
	}

	for _, uri := range maliciousURIs {
		t.Run("uri_"+uri[:min(30, len(uri))], func(t *testing.T) {
			params, _ := json.Marshal(map[string]string{"uri": uri})
			body := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":` + string(params) + `}`
			resp := mcpPost(t, h.HandlePost, body)

			if resp.Error == nil {
				// Check that we didn't get sensitive data back
				var result map[string]interface{}
				json.Unmarshal(resp.Result, &result)
				t.Logf("[INFO] URI %q was processed (check result for data leak)", uri[:min(50, len(uri))])
			}
		})
	}
}

func TestSecurityMCP_PromptGet_TemplateInjection(t *testing.T) {
	h := NewMCPHandler(
		nil, nil,
		&mockMCPPromptProviderForHandler{},
		nil,
	)

	// Template injection in prompt arguments
	injections := []struct {
		name string
		args map[string]string
	}{
		{"go_template", map[string]string{"topic": "{{.Env.SECRET_KEY}}"}},
		{"jinja", map[string]string{"topic": "{{ config.items() }}"}},
		{"ssti_python", map[string]string{"topic": "{{7*7}}"}},
		{"mustache", map[string]string{"topic": "{{#evil}}pwned{{/evil}}"}},
		{"shell_expansion", map[string]string{"topic": "$(cat /etc/passwd)"}},
		{"backtick", map[string]string{"topic": "`cat /etc/passwd`"}},
		{"xss", map[string]string{"topic": "<img src=x onerror=alert(1)>"}},
		{"large_value", map[string]string{"topic": strings.Repeat("A", 100000)}},
	}

	for _, tc := range injections {
		t.Run(tc.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tc.args)
			params := `{"name":"test-agent","arguments":` + string(argsJSON) + `}`
			body := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":` + params + `}`
			resp := mcpPost(t, h.HandlePost, body)

			if resp.Error != nil && resp.Error.Code == mcp.InternalError {
				t.Errorf("[CRITICAL] Template injection caused internal error: %s", resp.Error.Message)
			}
		})
	}
}

func TestSecurityMCP_ErrorMessageInfoLeak(t *testing.T) {
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, _ string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				return nil, &MCPJSONRPCError{Code: -32601, Message: "unknown tool: test"}
			},
		},
		nil, nil, nil,
	)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nonexistent"}}`
	resp := mcpPost(t, h.HandlePost, body)

	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}

	// Check error message doesn't leak internal details
	msg := resp.Error.Message
	sensitivePatterns := []string{
		"/home/",
		"/Users/",
		"panic",
		"goroutine",
		"stack trace",
		"sql:",
		"postgres",
		"password",
		"secret",
		"internal error at",
		".go:",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(pattern)) {
			t.Errorf("[FINDING] Error message leaks sensitive info: pattern=%q, message=%q", pattern, msg)
		}
	}
}

func TestSecurityMCP_ManifestExternalURL_NoSecretLeak(t *testing.T) {
	// The manifest handler uses externalURL — verify it doesn't expose internals
	manifest := NewMCPManifestHandler("https://registry.example.com")
	h := NewMCPHandler(nil, nil, nil, manifest)

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	w := httptest.NewRecorder()
	h.ServeManifest(w, req)

	body, _ := io.ReadAll(w.Body)
	bodyStr := string(body)

	// Check manifest doesn't expose internal URLs or secrets
	sensitiveURLs := []string{
		"127.0.0.1",
		"localhost",
		"0.0.0.0",
		"internal",
		"192.168.",
		"10.0.",
		"172.16.",
	}

	for _, su := range sensitiveURLs {
		if strings.Contains(bodyStr, su) {
			t.Errorf("[FINDING] Manifest contains internal URL pattern: %s", su)
		}
	}
}

// =============================================================================
// TASK #5: Resource exhaustion at handler level
// =============================================================================

func TestSecurityMCP_ConcurrentToolCalls(t *testing.T) {
	var callCount int64
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, _ string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				atomic.AddInt64(&callCount, 1)
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: `{"agents":[]}`}},
				}, nil
			},
		},
		nil, nil, nil,
	)

	var wg sync.WaitGroup
	concurrency := 100
	var errors int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandlePost(w, req)
			if w.Code >= 500 {
				atomic.AddInt64(&errors, 1)
			}
		}()
	}

	wg.Wait()

	if errors > 0 {
		t.Errorf("[FINDING] %d/%d concurrent tool calls caused server errors", errors, concurrency)
	}
	t.Logf("[INFO] %d tool calls processed concurrently without errors", callCount)
}

func TestSecurityMCP_BatchAmplification(t *testing.T) {
	var callCount int64
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, _ string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				atomic.AddInt64(&callCount, 1)
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: `{"ok":true}`}},
				}, nil
			},
		},
		nil, nil, nil,
	)

	// Single HTTP request with 500 tool calls in batch
	var batchBody strings.Builder
	batchBody.WriteString("[")
	for i := 0; i < 500; i++ {
		if i > 0 {
			batchBody.WriteString(",")
		}
		batchBody.WriteString(`{"jsonrpc":"2.0","id":`)
		batchBody.WriteString(json.Number(strings.Repeat("1", 1)).String())
		batchBody.WriteString(`,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}`)
	}
	batchBody.WriteString("]")

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(batchBody.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandlePost(w, req)

	if w.Code >= 500 {
		t.Errorf("[FINDING] Batch of 500 tool calls caused server error: %d", w.Code)
	}

	t.Logf("[INFO] Batch of 500 requests processed. Total handler calls: %d. No batch size limit enforced.", callCount)
	if callCount >= 500 {
		t.Log("[FINDING] No batch size limit — a single request can trigger 500+ handler calls. Consider adding a max batch size.")
	}
}

func TestSecurityMCP_LargeToolArguments(t *testing.T) {
	var receivedSize int
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, _ string, args json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				receivedSize = len(args)
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: `{"ok":true}`}},
				}, nil
			},
		},
		nil, nil, nil,
	)

	// 500KB of arguments (under the 1MB transport limit but very large)
	bigValue := strings.Repeat("X", 500000)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_agents","arguments":{"data":"` + bigValue + `"}}}`

	resp := mcpPost(t, h.HandlePost, body)
	if resp.Error != nil && resp.Error.Code == mcp.InternalError {
		t.Errorf("[FINDING] Large arguments caused internal error: %s", resp.Error.Message)
	}

	t.Logf("[INFO] Large tool arguments (%d bytes) were accepted. No per-argument size limit.", receivedSize)
}

func TestSecurityMCP_ResponseSizeAmplification(t *testing.T) {
	// A tool that returns a very large response
	h := NewMCPHandler(
		&mockMCPToolExecutorForHandler{
			callToolFn: func(_ context.Context, _ string, _ json.RawMessage) (*MCPToolResult, *MCPJSONRPCError) {
				bigResponse := strings.Repeat("DATA", 250000) // 1MB response
				return &MCPToolResult{
					Content: []MCPToolResultContent{{Type: "text", Text: bigResponse}},
				}, nil
			},
		},
		nil, nil, nil,
	)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandlePost(w, req)

	respBody, _ := io.ReadAll(w.Body)
	t.Logf("[INFO] Response size amplification: request=%d bytes, response=%d bytes (ratio: %.1fx)",
		len(body), len(respBody), float64(len(respBody))/float64(len(body)))

	if len(respBody) > 10*1024*1024 {
		t.Log("[FINDING] Response exceeded 10MB — no response size limit enforced")
	}
}

