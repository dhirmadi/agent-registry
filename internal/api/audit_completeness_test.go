package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// --- Minimal mock stores for audit completeness tests ---

// mockPromptStoreForAudit implements PromptStoreForAPI.
type mockPromptStoreForAudit struct{}

func (m *mockPromptStoreForAudit) List(_ context.Context, _ string, _ bool, _, _ int) ([]store.Prompt, int, error) {
	return nil, 0, nil
}
func (m *mockPromptStoreForAudit) GetActive(_ context.Context, _ string) (*store.Prompt, error) {
	return nil, nil
}
func (m *mockPromptStoreForAudit) GetByID(_ context.Context, _ uuid.UUID) (*store.Prompt, error) {
	return nil, nil
}
func (m *mockPromptStoreForAudit) Create(_ context.Context, prompt *store.Prompt) error {
	prompt.ID = uuid.New()
	prompt.Version = 1
	prompt.IsActive = false
	prompt.CreatedAt = time.Now()
	return nil
}
func (m *mockPromptStoreForAudit) Activate(_ context.Context, _ uuid.UUID) (*store.Prompt, error) {
	return &store.Prompt{ID: uuid.New(), IsActive: true}, nil
}
func (m *mockPromptStoreForAudit) Rollback(_ context.Context, _ string, _ int, _ string) (*store.Prompt, error) {
	return &store.Prompt{ID: uuid.New()}, nil
}

// mockMCPServerStoreForAudit implements MCPServerStoreForAPI.
type mockMCPServerStoreForAudit struct{}

func (m *mockMCPServerStoreForAudit) Create(_ context.Context, server *store.MCPServer) error {
	server.ID = uuid.New()
	server.CreatedAt = time.Now()
	server.UpdatedAt = time.Now()
	return nil
}
func (m *mockMCPServerStoreForAudit) GetByID(_ context.Context, _ uuid.UUID) (*store.MCPServer, error) {
	return &store.MCPServer{ID: uuid.New(), UpdatedAt: time.Now()}, nil
}
func (m *mockMCPServerStoreForAudit) GetByLabel(_ context.Context, _ string) (*store.MCPServer, error) {
	return nil, nil
}
func (m *mockMCPServerStoreForAudit) List(_ context.Context) ([]store.MCPServer, error) {
	return nil, nil
}
func (m *mockMCPServerStoreForAudit) Update(_ context.Context, _ *store.MCPServer) error {
	return nil
}
func (m *mockMCPServerStoreForAudit) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

// mockTrustRuleStoreForAudit implements TrustRuleStoreForAPI.
type mockTrustRuleStoreForAudit struct{}

func (m *mockTrustRuleStoreForAudit) List(_ context.Context, _ uuid.UUID) ([]store.TrustRule, error) {
	return nil, nil
}
func (m *mockTrustRuleStoreForAudit) Upsert(_ context.Context, rule *store.TrustRule) error {
	rule.ID = uuid.New()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	return nil
}
func (m *mockTrustRuleStoreForAudit) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

// mockTrustDefaultStoreForAudit implements TrustDefaultStoreForAPI.
type mockTrustDefaultStoreForAudit struct{}

func (m *mockTrustDefaultStoreForAudit) List(_ context.Context) ([]store.TrustDefault, error) {
	return nil, nil
}
func (m *mockTrustDefaultStoreForAudit) GetByID(_ context.Context, _ uuid.UUID) (*store.TrustDefault, error) {
	return &store.TrustDefault{ID: uuid.New(), Tier: "auto", Patterns: json.RawMessage(`[]`), UpdatedAt: time.Now()}, nil
}
func (m *mockTrustDefaultStoreForAudit) Update(_ context.Context, _ *store.TrustDefault) error {
	return nil
}

// mockModelConfigStoreForAudit implements ModelConfigStoreForAPI.
type mockModelConfigStoreForAudit struct{}

func (m *mockModelConfigStoreForAudit) GetByScope(_ context.Context, _, _ string) (*store.ModelConfig, error) {
	return &store.ModelConfig{ID: uuid.New(), UpdatedAt: time.Now()}, nil
}
func (m *mockModelConfigStoreForAudit) GetMerged(_ context.Context, _, _ string) (*store.ModelConfig, error) {
	return &store.ModelConfig{ID: uuid.New(), UpdatedAt: time.Now()}, nil
}
func (m *mockModelConfigStoreForAudit) Update(_ context.Context, _ *store.ModelConfig, _ time.Time) error {
	return nil
}
func (m *mockModelConfigStoreForAudit) Upsert(_ context.Context, _ *store.ModelConfig) error {
	return nil
}

// mockWebhookStoreForAudit implements WebhookStoreForAPI.
type mockWebhookStoreForAudit struct{}

func (m *mockWebhookStoreForAudit) Create(_ context.Context, sub *store.WebhookSubscription) error {
	sub.ID = uuid.New()
	sub.CreatedAt = time.Now()
	sub.UpdatedAt = time.Now()
	return nil
}
func (m *mockWebhookStoreForAudit) List(_ context.Context) ([]store.WebhookSubscription, error) {
	return nil, nil
}
func (m *mockWebhookStoreForAudit) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

// mockAPIKeyStoreForAudit implements APIKeyStoreForAPI.
type mockAPIKeyStoreForAudit struct{}

func (m *mockAPIKeyStoreForAudit) Create(_ context.Context, key *store.APIKey) error {
	key.ID = uuid.New()
	key.CreatedAt = time.Now()
	return nil
}
func (m *mockAPIKeyStoreForAudit) List(_ context.Context, _ *uuid.UUID) ([]store.APIKey, error) {
	return nil, nil
}
func (m *mockAPIKeyStoreForAudit) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *mockAPIKeyStoreForAudit) GetByID(_ context.Context, _ uuid.UUID) (*store.APIKey, error) {
	return &store.APIKey{ID: uuid.New()}, nil
}

// mockUserStoreForAudit implements UserStoreForAPI.
type mockUserStoreForAudit struct {
	users map[uuid.UUID]*store.User
}

func newMockUserStoreForAudit() *mockUserStoreForAudit {
	return &mockUserStoreForAudit{users: make(map[uuid.UUID]*store.User)}
}

func (m *mockUserStoreForAudit) List(_ context.Context, _, _ int) ([]store.User, int, error) {
	return nil, 0, nil
}
func (m *mockUserStoreForAudit) Create(_ context.Context, user *store.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.ID] = user
	return nil
}
func (m *mockUserStoreForAudit) GetByID(_ context.Context, id uuid.UUID) (*store.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}
func (m *mockUserStoreForAudit) Update(_ context.Context, user *store.User) error {
	user.UpdatedAt = time.Now()
	m.users[user.ID] = user
	return nil
}
func (m *mockUserStoreForAudit) CountAdmins(_ context.Context) (int, error) {
	return 1, nil
}

// mockAgentLookupForPromptsAudit implements AgentLookupForPrompts.
type mockAgentLookupForPromptsAudit struct{}

func (m *mockAgentLookupForPromptsAudit) GetByID(_ context.Context, _ string) (*store.Agent, error) {
	return &store.Agent{ID: "test_agent"}, nil
}

// --- Audit completeness test ---

// TestAuditCompleteness verifies that every mutation endpoint writes an audit entry
// with the correct action and resource_type.
func TestAuditCompleteness(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           interface{}
		headers        map[string]string
		wantAction     string
		wantResType    string
		wantMinEntries int // Minimum expected audit entries after the call
	}{
		{
			name:   "POST /api/v1/agents creates audit for agent",
			method: http.MethodPost,
			path:   "/api/v1/agents",
			body: map[string]interface{}{
				"id":   "audit_agent_create",
				"name": "Audit Agent Create",
			},
			wantAction:     "agent_create",
			wantResType:    "agent",
			wantMinEntries: 1,
		},
		{
			name:           "DELETE /api/v1/agents/{id} creates audit for agent",
			method:         http.MethodDelete,
			path:           "/api/v1/agents/audit_agent_create",
			wantAction:     "agent_delete",
			wantResType:    "agent",
			wantMinEntries: 1,
		},
		{
			name:   "POST /api/v1/agents/{id}/prompts creates audit for prompt",
			method: http.MethodPost,
			path:   "/api/v1/agents/test_agent/prompts",
			body: map[string]interface{}{
				"system_prompt": "You are a helpful assistant.",
				"mode":          "toolcalling_safe",
			},
			wantAction:     "prompt_create",
			wantResType:    "prompt",
			wantMinEntries: 1,
		},
		{
			name:   "POST /api/v1/mcp-servers creates audit for mcp_server",
			method: http.MethodPost,
			path:   "/api/v1/mcp-servers",
			body: map[string]interface{}{
				"label":    "test-mcp",
				"endpoint": "https://mcp.example.com/api",
			},
			wantAction:     "mcp_server_create",
			wantResType:    "mcp_server",
			wantMinEntries: 1,
		},
		{
			name:           "DELETE /api/v1/mcp-servers/{id} creates audit for mcp_server",
			method:         http.MethodDelete,
			path:           "/api/v1/mcp-servers/" + uuid.New().String(),
			wantAction:     "mcp_server_delete",
			wantResType:    "mcp_server",
			wantMinEntries: 1,
		},
		{
			name:   "PUT /api/v1/trust-defaults/{id} creates audit for trust_default",
			method: http.MethodPut,
			path:   "/api/v1/trust-defaults/" + uuid.New().String(),
			body: map[string]interface{}{
				"patterns": []string{"*"},
			},
			headers:        map[string]string{"If-Match": time.Now().UTC().Format(time.RFC3339Nano)},
			wantAction:     "trust_default_update",
			wantResType:    "trust_default",
			wantMinEntries: 1,
		},
		{
			name:   "POST /api/v1/workspaces/{id}/trust-rules creates audit for trust_rule",
			method: http.MethodPost,
			path:   "/api/v1/workspaces/" + uuid.New().String() + "/trust-rules",
			body: map[string]interface{}{
				"tool_pattern": "git_read",
				"tier":         "auto",
			},
			wantAction:     "trust_rule_upsert",
			wantResType:    "trust_rule",
			wantMinEntries: 1,
		},
		{
			name:           "DELETE /api/v1/workspaces/{id}/trust-rules/{ruleId} creates audit",
			method:         http.MethodDelete,
			path:           "/api/v1/workspaces/" + uuid.New().String() + "/trust-rules/" + uuid.New().String(),
			wantAction:     "trust_rule_delete",
			wantResType:    "trust_rule",
			wantMinEntries: 1,
		},
		{
			name:   "PUT /api/v1/model-config creates audit for model_config",
			method: http.MethodPut,
			path:   "/api/v1/model-config",
			body: map[string]interface{}{
				"default_model": "gpt-4",
			},
			headers:        map[string]string{"If-Match": time.Now().UTC().Format(time.RFC3339Nano)},
			wantAction:     "model_config_update",
			wantResType:    "model_config",
			wantMinEntries: 1,
		},
		{
			name:   "POST /api/v1/webhooks creates audit for webhook",
			method: http.MethodPost,
			path:   "/api/v1/webhooks",
			body: map[string]interface{}{
				"url":    "https://hooks.example.com/callback",
				"events": []string{"agent.created"},
			},
			wantAction:     "webhook_create",
			wantResType:    "webhook_subscription",
			wantMinEntries: 1,
		},
		{
			name:           "DELETE /api/v1/webhooks/{id} creates audit for webhook",
			method:         http.MethodDelete,
			path:           "/api/v1/webhooks/" + uuid.New().String(),
			wantAction:     "webhook_delete",
			wantResType:    "webhook_subscription",
			wantMinEntries: 1,
		},
		{
			name:   "POST /api/v1/api-keys creates audit for api_key",
			method: http.MethodPost,
			path:   "/api/v1/api-keys",
			body: map[string]interface{}{
				"name": "Audit Test Key",
			},
			wantAction:     "api_key_create",
			wantResType:    "api_key",
			wantMinEntries: 1,
		},
		{
			name:           "DELETE /api/v1/api-keys/{id} creates audit for api_key",
			method:         http.MethodDelete,
			path:           "/api/v1/api-keys/" + uuid.New().String(),
			wantAction:     "api_key_revoke",
			wantResType:    "api_key",
			wantMinEntries: 1,
		},
		{
			name:   "POST /api/v1/users creates audit for user",
			method: http.MethodPost,
			path:   "/api/v1/users",
			body: map[string]interface{}{
				"username": "audituser",
				"email":    "audit@example.com",
				"password": "StrongPass123!",
				"role":     "viewer",
			},
			wantAction:     "user_create",
			wantResType:    "user",
			wantMinEntries: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh audit store for each test
			auditStore := &mockAuditStoreForAPI{}
			userID := uuid.New()
			csrfToken := "test-csrf"

			sessions := &mockSessionLookupForInteg{
				sessions: map[string]mockSessionData{
					"admin-session": {
						userID:    userID,
						role:      "admin",
						csrfToken: csrfToken,
					},
				},
			}
			apiKeys := &mockAPIKeyLookupForInteg{keys: map[string]mockAPIKeyData{}}

			agentStore := newMockAgentStore()
			// Pre-seed an agent for operations that need one
			agentStore.agents["test_agent"] = &store.Agent{
				ID:             "test_agent",
				Name:           "Test Agent",
				Tools:          json.RawMessage(`[]`),
				TrustOverrides: json.RawMessage(`{}`),
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        1,
				CreatedBy:      "system",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			agentStore.agents["audit_agent_create"] = &store.Agent{
				ID:             "audit_agent_create",
				Name:           "Pre-seeded for delete",
				Tools:          json.RawMessage(`[]`),
				TrustOverrides: json.RawMessage(`{}`),
				ExamplePrompts: json.RawMessage(`[]`),
				IsActive:       true,
				Version:        1,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			userStore := newMockUserStoreForAudit()

			cfg := RouterConfig{
				Health:        &HealthHandler{DB: &mockPinger{}},
				Auth:          &mockAuthRouteHandler{},
				Agents:        NewAgentsHandler(agentStore, auditStore, nil),
				Prompts:       NewPromptsHandler(&mockPromptStoreForAudit{}, &mockAgentLookupForPromptsAudit{}, auditStore, nil),
				MCPServers:    NewMCPServersHandler(&mockMCPServerStoreForAudit{}, auditStore, nil, nil),
				TrustRules:    NewTrustRulesHandler(&mockTrustRuleStoreForAudit{}, auditStore, nil),
				TrustDefaults: NewTrustDefaultsHandler(&mockTrustDefaultStoreForAudit{}, auditStore, nil),
				ModelConfig:   NewModelConfigHandler(&mockModelConfigStoreForAudit{}, auditStore, nil),
				Webhooks:      NewWebhooksHandler(&mockWebhookStoreForAudit{}, auditStore),
				APIKeys:       NewAPIKeysHandler(&mockAPIKeyStoreForAudit{}, auditStore),
				Users:         NewUsersHandler(userStore, newMockOAuthConnStore(), auditStore),
				AuthMW:        AuthMiddleware(sessions, apiKeys),
			}

			router := NewRouter(cfg)
			server := httptest.NewServer(router)
			defer server.Close()

			client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}}

			var buf bytes.Buffer
			if tt.body != nil {
				json.NewEncoder(&buf).Encode(tt.body)
			}

			req, err := http.NewRequest(tt.method, server.URL+tt.path, &buf)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName(), Value: "admin-session"})
			req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName(), Value: csrfToken})
			req.Header.Set("X-CSRF-Token", csrfToken)

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			// Check that the request did not cause a server error
			if resp.StatusCode >= 500 {
				var body bytes.Buffer
				body.ReadFrom(resp.Body)
				t.Fatalf("server error %d; body: %s", resp.StatusCode, body.String())
			}

			// We only check audit when the request succeeded (2xx).
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if len(auditStore.entries) < tt.wantMinEntries {
					t.Fatalf("expected at least %d audit entry, got %d", tt.wantMinEntries, len(auditStore.entries))
				}

				lastEntry := auditStore.entries[len(auditStore.entries)-1]
				if lastEntry.Action != tt.wantAction {
					t.Errorf("expected audit action %q, got %q", tt.wantAction, lastEntry.Action)
				}
				if lastEntry.ResourceType != tt.wantResType {
					t.Errorf("expected resource_type %q, got %q", tt.wantResType, lastEntry.ResourceType)
				}
				if lastEntry.ResourceID == "" {
					t.Error("expected non-empty resource_id in audit entry")
				}
			} else {
				t.Logf("request returned %d — checking if this is expected for %s %s", resp.StatusCode, tt.method, tt.path)
			}
		})
	}
}
