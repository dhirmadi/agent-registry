package mcp

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequestSerialization(t *testing.T) {
	tests := []struct {
		name string
		req  JSONRPCRequest
		want string
	}{
		{
			name: "basic request with params",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Method:  "initialize",
				Params:  json.RawMessage(`{"protocolVersion":"2025-03-26"}`),
			},
			want: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
		},
		{
			name: "request with string ID",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`"abc-123"`),
				Method:  "tools/list",
			},
			want: `{"jsonrpc":"2.0","id":"abc-123","method":"tools/list"}`,
		},
		{
			name: "request with null ID",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`null`),
				Method:  "tools/list",
			},
			want: `{"jsonrpc":"2.0","id":null,"method":"tools/list"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.req)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			if string(data) != tc.want {
				t.Errorf("got  %s\nwant %s", string(data), tc.want)
			}

			var decoded JSONRPCRequest
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if decoded.Method != tc.req.Method {
				t.Errorf("method: got %q, want %q", decoded.Method, tc.req.Method)
			}
			if decoded.JSONRPC != "2.0" {
				t.Errorf("jsonrpc: got %q, want %q", decoded.JSONRPC, "2.0")
			}
		})
	}
}

func TestJSONRPCResponseSerialization(t *testing.T) {
	tests := []struct {
		name string
		resp JSONRPCResponse
		want string
	}{
		{
			name: "success response",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Result:  json.RawMessage(`{"tools":[]}`),
			},
			want: `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`,
		},
		{
			name: "error response",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Error: &JSONRPCError{
					Code:    MethodNotFound,
					Message: "method not found",
				},
			},
			want: `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`,
		},
		{
			name: "error with data",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`"req-1"`),
				Error: &JSONRPCError{
					Code:    InvalidParams,
					Message: "missing required field",
					Data:    json.RawMessage(`{"field":"name"}`),
				},
			},
			want: `{"jsonrpc":"2.0","id":"req-1","error":{"code":-32602,"message":"missing required field","data":{"field":"name"}}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.resp)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			if string(data) != tc.want {
				t.Errorf("got  %s\nwant %s", string(data), tc.want)
			}
		})
	}
}

func TestJSONRPCNotificationHasNoID(t *testing.T) {
	notif := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	data, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if _, ok := m["id"]; ok {
		t.Error("notification should not have an id field")
	}
	if m["method"] != "notifications/initialized" {
		t.Errorf("method: got %v, want %q", m["method"], "notifications/initialized")
	}
}

func TestJSONRPCErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
		want int
	}{
		{"ParseError", ParseError, -32700},
		{"InvalidRequest", InvalidRequest, -32600},
		{"MethodNotFound", MethodNotFound, -32601},
		{"InvalidParams", InvalidParams, -32602},
		{"InternalError", InternalError, -32603},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.code != tc.want {
				t.Errorf("got %d, want %d", tc.code, tc.want)
			}
		})
	}
}

func TestJSONRPCErrorImplementsErrorInterface(t *testing.T) {
	e := &JSONRPCError{
		Code:    MethodNotFound,
		Message: "method not found: foo/bar",
	}
	var err error = e
	if err.Error() != "method not found: foo/bar" {
		t.Errorf("Error() = %q, want %q", err.Error(), "method not found: foo/bar")
	}
}

func TestNewJSONRPCErrorHelpers(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(string) *JSONRPCError
		msg     string
		wantCode int
	}{
		{"NewParseError", NewParseError, "bad json", ParseError},
		{"NewInvalidRequest", NewInvalidRequest, "missing method", InvalidRequest},
		{"NewMethodNotFound", NewMethodNotFound, "no such method", MethodNotFound},
		{"NewInvalidParams", NewInvalidParams, "bad param", InvalidParams},
		{"NewInternalError", NewInternalError, "oops", InternalError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.fn(tc.msg)
			if e.Code != tc.wantCode {
				t.Errorf("code: got %d, want %d", e.Code, tc.wantCode)
			}
			if e.Message != tc.msg {
				t.Errorf("message: got %q, want %q", e.Message, tc.msg)
			}
		})
	}
}

func TestServerInfoSerialization(t *testing.T) {
	info := ServerInfo{
		Name:    "agentic-registry",
		Version: "1.0.0",
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	want := `{"name":"agentic-registry","version":"1.0.0"}`
	if string(data) != want {
		t.Errorf("got  %s\nwant %s", string(data), want)
	}
}

func TestServerCapabilitiesSerialization(t *testing.T) {
	caps := ServerCapabilities{
		Tools:     &ToolsCapability{ListChanged: true},
		Resources: &ResourcesCapability{Subscribe: false, ListChanged: true},
		Prompts:   &PromptsCapability{ListChanged: false},
	}
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := m["tools"]; !ok {
		t.Error("expected tools capability")
	}
	if _, ok := m["resources"]; !ok {
		t.Error("expected resources capability")
	}
}

func TestInitializeParamsRoundTrip(t *testing.T) {
	raw := `{
		"protocolVersion": "2025-03-26",
		"capabilities": {},
		"clientInfo": {"name": "test-client", "version": "0.1.0"}
	}`

	var params InitializeParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if params.ProtocolVersion != "2025-03-26" {
		t.Errorf("protocolVersion: got %q, want %q", params.ProtocolVersion, "2025-03-26")
	}
	if params.ClientInfo.Name != "test-client" {
		t.Errorf("clientInfo.name: got %q, want %q", params.ClientInfo.Name, "test-client")
	}
}

func TestInitializeResultSerialization(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: "2025-03-26",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    "agentic-registry",
			Version: "1.0.0",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded InitializeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.ProtocolVersion != "2025-03-26" {
		t.Errorf("protocolVersion: got %q", decoded.ProtocolVersion)
	}
	if decoded.ServerInfo.Name != "agentic-registry" {
		t.Errorf("serverInfo.name: got %q", decoded.ServerInfo.Name)
	}
}

func TestToolDefinitionSerialization(t *testing.T) {
	tool := ToolDefinition{
		Name:        "list_agents",
		Description: "List all active agents",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"active_only":{"type":"boolean"}}}`),
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.Name != "list_agents" {
		t.Errorf("name: got %q", decoded.Name)
	}
}

func TestToolCallParamsRoundTrip(t *testing.T) {
	raw := `{"name":"get_agent","arguments":{"agent_id":"my-agent"}}`
	var params ToolCallParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if params.Name != "get_agent" {
		t.Errorf("name: got %q", params.Name)
	}
	if params.Arguments == nil {
		t.Fatal("arguments should not be nil")
	}
}

func TestToolResultSerialization(t *testing.T) {
	result := ToolResult{
		Content: []ToolResultContent{
			{Type: "text", Text: "found 5 agents"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(data) == "" {
		t.Fatal("expected non-empty output")
	}

	var decoded ToolResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(decoded.Content) != 1 {
		t.Fatalf("content length: got %d, want 1", len(decoded.Content))
	}
	if decoded.Content[0].Text != "found 5 agents" {
		t.Errorf("text: got %q", decoded.Content[0].Text)
	}
}

func TestResourceTemplateSerialization(t *testing.T) {
	tmpl := ResourceTemplate{
		URITemplate: "agent://{agentId}",
		Name:        "Agent Definition",
		Description: "Agent definition by ID",
		MimeType:    "application/json",
	}

	data, err := json.Marshal(tmpl)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ResourceTemplate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.URITemplate != "agent://{agentId}" {
		t.Errorf("uriTemplate: got %q", decoded.URITemplate)
	}
}

func TestResourceContentSerialization(t *testing.T) {
	content := ResourceContent{
		URI:      "agent://my-agent",
		MimeType: "application/json",
		Text:     `{"id":"my-agent","name":"Test Agent"}`,
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ResourceContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.URI != "agent://my-agent" {
		t.Errorf("uri: got %q", decoded.URI)
	}
}

func TestPromptDefinitionSerialization(t *testing.T) {
	prompt := PromptDefinition{
		Name:        "agent_system_prompt",
		Description: "System prompt for an agent",
		Arguments: []PromptArgument{
			{Name: "agent_id", Description: "The agent ID", Required: true},
		},
	}

	data, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded PromptDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.Name != "agent_system_prompt" {
		t.Errorf("name: got %q", decoded.Name)
	}
	if len(decoded.Arguments) != 1 {
		t.Fatalf("arguments count: got %d", len(decoded.Arguments))
	}
	if !decoded.Arguments[0].Required {
		t.Error("argument should be required")
	}
}

func TestPromptResultSerialization(t *testing.T) {
	result := PromptResult{
		Description: "System prompt for agent",
		Messages: []PromptMessage{
			{Role: "assistant", Content: PromptContent{Type: "text", Text: "You are an agent."}},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded PromptResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(decoded.Messages) != 1 {
		t.Fatalf("messages: got %d", len(decoded.Messages))
	}
	if decoded.Messages[0].Role != "assistant" {
		t.Errorf("role: got %q", decoded.Messages[0].Role)
	}
}

func TestRequestIDTypes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"integer id", `{"jsonrpc":"2.0","id":42,"method":"test"}`},
		{"string id", `{"jsonrpc":"2.0","id":"abc","method":"test"}`},
		{"null id", `{"jsonrpc":"2.0","id":null,"method":"test"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req JSONRPCRequest
			if err := json.Unmarshal([]byte(tc.raw), &req); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if req.Method != "test" {
				t.Errorf("method: got %q", req.Method)
			}
			if req.ID == nil {
				t.Error("ID should not be nil after unmarshal")
			}
		})
	}
}
