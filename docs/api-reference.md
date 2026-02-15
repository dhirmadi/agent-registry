# API Reference

> Complete reference for the Agentic Registry REST API.

---

## Conventions

### Base URL

All API endpoints are prefixed with `/api/v1`. Auth endpoints use `/auth`.

### Response Envelope

Every response follows a consistent envelope:

```json
{
  "success": true,
  "data": { ... },
  "error": null,
  "meta": {
    "timestamp": "2026-02-15T12:00:00Z",
    "request_id": "req_abc123"
  }
}
```

Error responses:

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": "NOT_FOUND",
    "message": "Agent 'unknown_agent' not found"
  },
  "meta": {
    "timestamp": "2026-02-15T12:00:00Z",
    "request_id": "req_abc123"
  }
}
```

### Authentication

All `/api/v1/*` endpoints require authentication via:
- **Session cookie:** `__Host-session` (set by `/auth/login` or Google OAuth)
- **API key:** `Authorization: Bearer rk_live_...`

### Optimistic Concurrency

Update operations (`PUT`, `PATCH`) require an `If-Match` header containing the resource's `updated_at` timestamp. If the resource has been modified since the client last read it, the server returns `409 Conflict`.

```http
PUT /api/v1/agents/my_agent
If-Match: "2026-02-15T10:30:00Z"
Content-Type: application/json
```

### CSRF

All non-GET requests using session cookie auth must include an `X-CSRF-Token` header. API key requests are exempt.

### Pagination

List endpoints support pagination via query parameters:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `limit` | Max items per page | 50 |
| `offset` | Number of items to skip | 0 |

Paginated responses include:

```json
{
  "data": {
    "items": [...],
    "total": 42,
    "limit": 50,
    "offset": 0
  }
}
```

---

## Health

### `GET /healthz` — Liveness

Returns `200 OK` if the server process is running. No auth required.

### `GET /readyz` — Readiness

Returns `200 OK` if the server can reach the database. No auth required.

---

## Authentication

### `POST /auth/login`

Authenticate with username and password. Sets session cookie on success.

**Request:**
```json
{
  "username": "admin",
  "password": "secure-password-123!"
}
```

**Responses:** `200` (session set), `401` (invalid credentials), `423` (locked)

### `POST /auth/logout`

Destroy the current session. Requires session cookie + CSRF token.

### `GET /auth/me`

Return the authenticated user's profile. Works with session or API key.

**Response:**
```json
{
  "data": {
    "id": "uuid",
    "username": "admin",
    "email": "admin@example.com",
    "display_name": "Admin",
    "role": "admin",
    "auth_method": "password",
    "is_active": true,
    "must_change_password": false,
    "last_login_at": "2026-02-15T10:00:00Z"
  }
}
```

### `POST /auth/change-password`

Change the current user's password. Invalidates all other sessions.

**Request:**
```json
{
  "current_password": "old-password",
  "new_password": "new-secure-password-456!"
}
```

### `GET /auth/google/start`

Initiate Google OAuth 2.0 PKCE flow. Redirects to Google consent screen.

### `GET /auth/google/callback`

Google OAuth callback. Creates or links account, then redirects to GUI.

### `POST /auth/unlink-google`

Remove Google account link. Sets `auth_method` to `"password"`.

---

## Agents

Agents are the primary resource — each represents an AI agent with a name, description, system prompt, tools, trust overrides, and example prompts. Agents are versioned: every update creates an immutable version snapshot.

### `GET /api/v1/agents`

List all agents. Supports pagination.

**Query Parameters:** `limit`, `offset`, `search` (name/description filter)

**Required Role:** `viewer`, `editor`, or `admin`

### `GET /api/v1/agents/{agentId}`

Get a single agent by ID.

**Required Role:** `viewer`, `editor`, or `admin`

### `POST /api/v1/agents`

Create a new agent. The `agentId` must match `/^[a-z][a-z0-9_]{1,49}$/`.

**Request:**
```json
{
  "id": "my_agent",
  "name": "My Agent",
  "description": "Does useful things",
  "system_prompt": "You are a helpful agent...",
  "tools": [
    {
      "name": "search",
      "description": "Search the knowledge base",
      "parameters": { "type": "object", "properties": { "query": { "type": "string" } } },
      "source": "mcp",
      "server": "search-server"
    }
  ],
  "trust_overrides": {},
  "example_prompts": ["Help me find documents", "Search for project updates"],
  "is_active": true
}
```

**Required Role:** `editor` or `admin`

### `PUT /api/v1/agents/{agentId}`

Full update of an agent. Requires `If-Match` header. Increments version and creates a version snapshot.

**Required Role:** `editor` or `admin`

### `PATCH /api/v1/agents/{agentId}`

Partial update. Only included fields are modified. Requires `If-Match` header.

**Required Role:** `editor` or `admin`

### `DELETE /api/v1/agents/{agentId}`

Delete an agent and all its versions and prompts.

**Required Role:** `editor` or `admin`

### `GET /api/v1/agents/{agentId}/versions`

List all version snapshots for an agent (newest first).

**Required Role:** `viewer`, `editor`, or `admin`

### `GET /api/v1/agents/{agentId}/versions/{version}`

Get a specific version snapshot.

**Required Role:** `viewer`, `editor`, or `admin`

### `POST /api/v1/agents/{agentId}/rollback`

Roll back an agent to a previous version.

**Request:**
```json
{
  "version": 3
}
```

**Required Role:** `editor` or `admin`

---

## Prompts

Prompts are versioned system prompts attached to agents. Each agent has zero or more prompts, with exactly one marked as active. Creating a new prompt increments the version; activating a prompt is a separate operation.

### `GET /api/v1/agents/{agentId}/prompts`

List all prompts for an agent (newest first).

**Required Role:** `viewer`, `editor`, or `admin`

### `GET /api/v1/agents/{agentId}/prompts/active`

Get the currently active prompt for an agent.

**Required Role:** `viewer`, `editor`, or `admin`

### `GET /api/v1/agents/{agentId}/prompts/{promptId}`

Get a specific prompt by ID.

**Required Role:** `viewer`, `editor`, or `admin`

### `POST /api/v1/agents/{agentId}/prompts`

Create a new prompt version.

**Request:**
```json
{
  "system_prompt": "You are a helpful assistant...",
  "template_vars": { "tone": "professional", "language": "en" },
  "mode": "default"
}
```

**Required Role:** `editor` or `admin`

### `POST /api/v1/agents/{agentId}/prompts/{promptId}/activate`

Set a prompt as the active prompt for its agent. Deactivates the previous active prompt in a single transaction.

**Required Role:** `editor` or `admin`

### `POST /api/v1/agents/{agentId}/prompts/rollback`

Roll back to a previous prompt version.

**Request:**
```json
{
  "version": 2
}
```

**Required Role:** `editor` or `admin`

---

## MCP Servers

MCP server configurations define external tool servers that agents can use. Credentials are encrypted at rest with AES-256-GCM.

### `GET /api/v1/mcp-servers`

List all MCP server configurations.

**Required Role:** `admin`

### `GET /api/v1/mcp-servers/{serverId}`

Get a single MCP server configuration. Credentials are redacted in responses.

**Required Role:** `admin`

### `POST /api/v1/mcp-servers`

Register a new MCP server.

**Request:**
```json
{
  "label": "search-server",
  "endpoint": "https://mcp.example.com/search",
  "auth_type": "bearer",
  "auth_credential": "secret-token",
  "health_endpoint": "https://mcp.example.com/health",
  "circuit_breaker": { "threshold": 5, "timeout": 30 },
  "discovery_interval": 300,
  "is_enabled": true
}
```

**Required Role:** `admin`

### `PUT /api/v1/mcp-servers/{serverId}`

Update an MCP server configuration. Requires `If-Match`.

**Required Role:** `admin`

### `DELETE /api/v1/mcp-servers/{serverId}`

Delete an MCP server configuration.

**Required Role:** `admin`

---

## Trust Rules

Workspace-scoped trust rules override the default trust classification for specific tool patterns. Trust determines what approval level a tool call requires.

### `GET /api/v1/workspaces/{workspaceId}/trust-rules`

List trust rules for a workspace.

**Required Role:** `editor` or `admin`

### `POST /api/v1/workspaces/{workspaceId}/trust-rules`

Create or upsert a trust rule.

**Request:**
```json
{
  "tool_pattern": "search_*",
  "tier": "trusted"
}
```

Tiers: `"trusted"`, `"cautious"`, `"untrusted"`

**Required Role:** `editor` or `admin`

### `DELETE /api/v1/workspaces/{workspaceId}/trust-rules/{ruleId}`

Delete a trust rule.

**Required Role:** `editor` or `admin`

---

## Trust Defaults

System-wide default trust classification patterns. These apply when no agent override or workspace rule matches.

### `GET /api/v1/trust-defaults`

List all trust defaults.

**Required Role:** `admin`

### `PUT /api/v1/trust-defaults/{defaultId}`

Update a trust default. Requires `If-Match`.

**Required Role:** `admin`

---

## Model Configuration

Global and workspace-scoped LLM parameters. Workspace config inherits from global and overrides only the fields it sets.

### `GET /api/v1/model-config`

Get the global model configuration.

**Required Role:** `admin`

### `PUT /api/v1/model-config`

Update the global model configuration. Requires `If-Match`.

**Request:**
```json
{
  "default_model": "gpt-4o",
  "temperature": 0.7,
  "max_tokens": 4096,
  "max_tool_rounds": 10,
  "default_context_window": 128000,
  "default_max_output_tokens": 4096,
  "history_token_budget": 8192,
  "max_history_messages": 50,
  "embedding_model": "text-embedding-3-small"
}
```

**Required Role:** `admin`

### `GET /api/v1/workspaces/{workspaceId}/model-config`

Get the merged model configuration for a workspace (global defaults + workspace overrides).

**Required Role:** `editor` or `admin`

### `PUT /api/v1/workspaces/{workspaceId}/model-config`

Set workspace-level model config overrides. Requires `If-Match`.

**Required Role:** `editor` or `admin`

---

## Webhooks

Webhook subscriptions receive push notifications when resources are mutated. Each delivery is signed with HMAC-SHA256.

### `GET /api/v1/webhooks`

List all webhook subscriptions.

**Required Role:** `admin`

### `POST /api/v1/webhooks`

Create a webhook subscription.

**Request:**
```json
{
  "url": "https://bff.example.com/webhooks/registry",
  "secret": "your-hmac-secret",
  "events": ["agent.created", "agent.updated", "prompt.activated"]
}
```

**Required Role:** `admin`

### `DELETE /api/v1/webhooks/{webhookId}`

Delete a webhook subscription.

**Required Role:** `admin`

### Webhook Delivery Format

```http
POST https://your-endpoint.com/webhook
Content-Type: application/json
X-Webhook-Signature: sha256=abc123...
X-Webhook-Event: agent.updated

{
  "event": "agent.updated",
  "resource_type": "agent",
  "resource_id": "my_agent",
  "actor": "admin",
  "timestamp": "2026-02-15T12:00:00Z"
}
```

### Supported Events

| Event | Trigger |
|-------|---------|
| `agent.created` | New agent created |
| `agent.updated` | Agent modified |
| `agent.deleted` | Agent removed |
| `prompt.created` | New prompt version created |
| `prompt.activated` | Prompt set as active |
| `mcp_server.created` | MCP server registered |
| `mcp_server.updated` | MCP server modified |
| `mcp_server.deleted` | MCP server removed |
| `trust_rule.created` | Trust rule added |
| `trust_rule.deleted` | Trust rule removed |
| `trust_default.updated` | Trust default modified |
| `model_config.updated` | Model config changed |
| `webhook.created` | Webhook subscription added |
| `user.created` | User account created |

---

## Discovery

### `GET /api/v1/discovery`

Returns a composite payload containing all configuration needed to hydrate a consumer's cache in a single call. Rate-limited to 10 requests per minute per user.

**Required Role:** `viewer`, `editor`, or `admin`

**Response:**
```json
{
  "data": {
    "agents": [...],
    "mcp_servers": [...],
    "trust_defaults": [...],
    "model_config": { ... }
  }
}
```

---

## Users

Admin-only user management.

### `GET /api/v1/users`

List all users with pagination.

### `POST /api/v1/users`

Create a new user.

**Request:**
```json
{
  "username": "jane",
  "email": "jane@example.com",
  "display_name": "Jane Smith",
  "password": "secure-password-123!",
  "role": "editor"
}
```

### `GET /api/v1/users/{userId}`

Get a single user.

### `PUT /api/v1/users/{userId}`

Update a user's profile or role.

### `DELETE /api/v1/users/{userId}`

Deactivate a user.

### `POST /api/v1/users/{userId}/reset-auth`

Reset a user's authentication (set a new temporary password with `must_change_password: true`).

---

## API Keys

### `GET /api/v1/api-keys`

List API keys. Admin sees all keys; other users see only their own. Key hashes are never returned.

### `POST /api/v1/api-keys`

Create a new API key. The full key is returned only in this response.

**Request:**
```json
{
  "name": "ci-pipeline",
  "scopes": ["read"]
}
```

### `DELETE /api/v1/api-keys/{keyId}`

Revoke an API key immediately.

---

## Audit Log

### `GET /api/v1/audit-log`

Query the append-only audit log. Supports pagination.

**Required Role:** `admin`

**Response:**
```json
{
  "data": {
    "items": [
      {
        "id": "uuid",
        "actor": "admin",
        "actor_id": "uuid",
        "action": "agent.created",
        "resource_type": "agent",
        "resource_id": "my_agent",
        "details": {},
        "ip_address": "192.168.1.1",
        "created_at": "2026-02-15T12:00:00Z"
      }
    ],
    "total": 1234,
    "limit": 50,
    "offset": 0
  }
}
```
