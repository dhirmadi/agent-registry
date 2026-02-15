# Agentic Registry Server — Implementation Specification

> **Purpose:** Drop this file into a new repository and instruct Claude Code (Opus 4.6) to implement. It contains everything needed to build a standalone Go microservice that serves as the single source of truth for all AI agent configuration artifacts.

---

## Table of Contents

1. [Overview and Goals](#1-overview-and-goals)
2. [Architecture](#2-architecture)
3. [Authentication and Authorization](#3-authentication-and-authorization)
4. [Resource Model](#4-resource-model)
5. [API Specification](#5-api-specification)
6. [Admin GUI](#6-admin-gui)
7. [Database Schema](#7-database-schema)
8. [Security Hardening](#8-security-hardening)
9. [Configuration and Environment](#9-configuration-and-environment)
10. [Client Integration Guide](#10-client-integration-guide)
11. [Observability](#11-observability)
12. [Deployment](#12-deployment)
13. [Implementation Roadmap](#13-implementation-roadmap)
- [Appendix A: Webhook Event Reference](#appendix-a-webhook-event-reference)
- [Appendix B: Authentication, Authorization, and API Key Reference](#appendix-b-authentication-authorization-and-api-key-reference)
- [Appendix C: Dependencies](#appendix-c-dependencies)

---

## 1. Overview and Goals

### 1.1 Problem

The Agent Smit platform currently hardcodes all agent-related artifacts inside the Go BFF container:

- **6 agent definitions** (ID, name, tools, system prompts, trust overrides) in `internal/agents/registry.go`
- **MCP server endpoints** passed as env vars and wired in `main.go`
- **Trust classification patterns** hardcoded in `internal/trust/classifier.go`
- **Model parameters** (model name, temperature, max tokens, context window) from env vars or constants in `internal/runs/service.go`
- **Context assembly budgets** hardcoded in the 6-layer pipeline in `internal/context/assembler.go`
- **Signal polling intervals** hardcoded per-publisher in `internal/signals/manager.go`

Any change to agent configuration, prompts, tools, or guardrails requires a container rebuild and redeployment.

### 1.2 Solution

Build a standalone **Agentic Registry** microservice that:

1. Stores and serves all agent-related configuration via a REST API.
2. Provides a **built-in admin GUI** (React + PatternFly 5) for managing all artifacts visually.
3. Supports **three authentication methods**: username/password, Google OAuth, and API keys for service-to-service.
4. Versions agent definitions and prompts with rollback support.
5. Notifies consumers (BFF, future services) of changes via webhooks.
6. Seeds itself on first boot with the 16 existing product agents, a default admin account (`admin/admin`), and full configuration.

### 1.3 Goals

| # | Goal |
|---|------|
| G1 | **Single source of truth** — All agent artifacts in one service with full CRUD API |
| G2 | **Versioning and rollback** — Prompts and agent configs are versioned; any version can be rolled back to |
| G3 | **Decoupled BFF** — BFF reads config at startup via one discovery call, then stays current via webhooks |
| G4 | **Standalone admin GUI** — Built-in web UI for managing all artifacts without touching code or API calls |
| G5 | **Multi-method authentication** — Username/password, Google OAuth, and API keys; smooth account linking |
| G6 | **Audit trail** — Every mutation records who changed what, when |
| G7 | **Security-first** — OWASP best practices, bcrypt passwords, encrypted secrets, rate limiting, CSRF protection |
| G8 | **Pattern consistency** — Same Go 1.24, chi/v5, pgx/v5, OTel stack as the BFF |

### 1.4 Non-Goals

- The registry does NOT execute agent runs, manage conversations, or process events.
- The registry does NOT manage workspace data, Forgejo repos, or Git content.
- The registry does NOT run MCP tool discovery against external servers — it stores their configuration.
- The registry does NOT replace the BFF's runtime orchestration; it only provides configuration.

### 1.5 Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Go 1.24 + chi/v5 | Matches the BFF stack exactly; shared patterns reduce cognitive load |
| PostgreSQL 16 + pgx/v5 | Same driver, pool config, and migration tooling as the BFF |
| golang-migrate/v4 | Same migration runner with embedded SQL |
| Three auth methods | Password (bootstrap), Google OAuth (production SSO), API keys (service-to-service) |
| bcrypt for passwords | Industry standard; cost factor 12; constant-time comparison |
| Secure session cookies | HttpOnly, Secure, SameSite=Lax; server-side session store in PostgreSQL |
| React + PatternFly 5 admin GUI | Matches the main frontend stack; embedded in Go binary via `embed.FS` |
| Webhook notifications | Push-based cache invalidation; avoids polling from consumers |
| Optimistic concurrency | `updated_at` as ETag prevents lost updates on concurrent edits |
| Separate database | Own schema in the same PostgreSQL instance; clean service boundary |

---

## 2. Architecture

### 2.1 Component Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                    Agentic Registry                           │
│  ┌─────────────────────┐    ┌─────────────────────────────┐  │
│  │  Admin GUI           │    │  REST API                    │  │
│  │  React + PatternFly 5│    │  /api/v1/*                   │  │
│  │  (embedded via Go    │    │  /auth/*                     │  │
│  │   embed.FS, served   │    │  /healthz, /readyz, /metrics │  │
│  │   at / by Go server) │    │                              │  │
│  └──────────┬──────────┘    └──────────┬──────────────────┘  │
│             │ fetch /api/v1/*           │                     │
│             └──────────┬───────────────┘                     │
│                        │                                     │
│  ┌─────────────────────▼───────────────────────────────────┐ │
│  │  Go Server (chi + pgx + OTel)                            │ │
│  │  • Authentication (password, Google OAuth, API keys)     │ │
│  │  • Session management (PostgreSQL-backed)                │ │
│  │  • CRUD handlers + webhook dispatcher                    │ │
│  └─────────────────────┬───────────────────────────────────┘ │
└────────────────────────┼─────────────────────────────────────┘
                         │ pgx
                         ▼
                  ┌─────────────┐
                  │ PostgreSQL  │
                  │ (pg16)      │
                  └─────────────┘

Consumers:
┌──────────────┐  GET /api/v1/discovery     ┌───────────────────┐
│ Agent Smit   │──────────────────────────> │ Agentic Registry  │
│ BFF (Go)     │ <── webhook POST           │                   │
│              │     on mutation             │                   │
└──────────────┘                            └───────────────────┘
                                              ▲
┌──────────────┐  (optional: direct access)   │
│ Agent Smit   │──────────────────────────────┘
│ Frontend     │  via BFF proxy or direct
└──────────────┘
```

### 2.2 Project Layout

```
agentic-registry/
├── cmd/
│   ├── server/
│   │   └── main.go                 # entrypoint: config, DB, router, OTel, seeder, graceful shutdown
│   └── healthcheck/
│       └── main.go                 # binary for container HEALTHCHECK
├── internal/
│   ├── api/
│   │   ├── router.go               # chi mux setup: API routes, auth routes, static file serving
│   │   ├── middleware.go            # auth (session + API key), request ID, logging, recovery, rate limit
│   │   ├── respond.go              # JSON response helpers
│   │   ├── health.go               # GET /healthz, /readyz
│   │   ├── agents.go               # agent CRUD + versions + rollback
│   │   ├── prompts.go              # prompt CRUD + activate + rollback
│   │   ├── mcp_servers.go          # MCP server config CRUD
│   │   ├── trust_rules.go          # workspace-scoped trust rule CRUD
│   │   ├── trust_defaults.go       # system-wide default trust patterns
│   │   ├── trigger_rules.go        # workspace-scoped trigger rule CRUD
│   │   ├── model_config.go         # global + workspace model config
│   │   ├── context_config.go       # context assembly config
│   │   ├── signal_config.go        # signal polling config
│   │   ├── webhooks.go             # webhook subscription CRUD
│   │   ├── api_keys.go             # API key management endpoints
│   │   ├── users.go                # user management endpoints
│   │   └── discovery.go            # GET /api/v1/discovery (composite)
│   ├── auth/
│   │   ├── handler.go              # login, logout, OAuth callback, password reset, account linking
│   │   ├── session.go              # server-side session store (PostgreSQL-backed)
│   │   ├── password.go             # bcrypt hash + verify, password policy validation
│   │   ├── oauth.go                # Google OAuth2 flow (PKCE), token exchange, account linking
│   │   ├── apikey.go               # API key validation middleware
│   │   └── csrf.go                 # CSRF token generation + validation (double-submit cookie)
│   ├── config/
│   │   └── config.go               # env var loading into Config struct
│   ├── db/
│   │   ├── pool.go                 # pgxpool wrapper
│   │   └── migrate.go              # migration runner
│   ├── errors/
│   │   └── errors.go               # APIError struct + common error constructors
│   ├── store/
│   │   ├── agents.go               # agent + agent_versions DB ops
│   │   ├── prompts.go              # prompt DB ops (versioned, transactional activate)
│   │   ├── mcp_servers.go          # MCP server DB ops
│   │   ├── trust_rules.go          # trust rule DB ops
│   │   ├── trust_defaults.go       # trust default DB ops
│   │   ├── trigger_rules.go        # trigger rule DB ops
│   │   ├── model_config.go         # model config DB ops (scope inheritance)
│   │   ├── context_config.go       # context config DB ops
│   │   ├── signal_config.go        # signal config DB ops
│   │   ├── webhooks.go             # webhook subscription DB ops
│   │   ├── api_keys.go             # API key DB ops
│   │   ├── users.go                # user + OAuth connection DB ops
│   │   ├── sessions.go             # session DB ops (create, get, delete, cleanup)
│   │   └── audit.go                # audit log DB ops
│   ├── notify/
│   │   └── dispatcher.go           # async webhook dispatcher (worker pool, HMAC signing, retry)
│   ├── ratelimit/
│   │   └── limiter.go              # in-memory + DB-backed rate limiter (login, API)
│   └── telemetry/
│       └── telemetry.go            # OTel trace + metric + log provider init
├── web/                            # Admin GUI (React + PatternFly 5)
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.tsx                # React entrypoint
│       ├── App.tsx                 # Router + auth context + layout shell
│       ├── api/
│       │   └── client.ts           # fetch wrapper with CSRF + session cookie
│       ├── auth/
│       │   ├── LoginPage.tsx       # username/password + "Sign in with Google" button
│       │   ├── AuthContext.tsx      # current user context + logout
│       │   └── ProtectedRoute.tsx  # redirect to login if not authenticated
│       ├── pages/
│       │   ├── DashboardPage.tsx   # overview: agent count, recent changes, system health
│       │   ├── AgentsPage.tsx      # agent list + create/edit/version/rollback
│       │   ├── AgentDetailPage.tsx # single agent: edit form, prompt editor, tool config, versions
│       │   ├── PromptsPage.tsx     # prompt version browser with diff view
│       │   ├── MCPServersPage.tsx  # MCP server list + create/edit + health status
│       │   ├── TrustPage.tsx       # trust defaults + workspace rules
│       │   ├── TriggersPage.tsx    # trigger rule list + create/edit
│       │   ├── ModelConfigPage.tsx # model parameters (global + workspace)
│       │   ├── ContextConfigPage.tsx # context layer budgets
│       │   ├── SignalsPage.tsx     # signal polling config
│       │   ├── WebhooksPage.tsx    # webhook subscriptions
│       │   ├── APIKeysPage.tsx     # API key management (create, revoke, view scopes)
│       │   ├── UsersPage.tsx       # user management (list, create, reset auth, roles)
│       │   ├── AuditLogPage.tsx    # searchable audit trail
│       │   └── MyAccountPage.tsx   # current user: change password, link/unlink Google, API keys
│       ├── components/
│       │   ├── AppLayout.tsx       # PatternFly Page + Masthead + Nav sidebar
│       │   ├── ConfirmDialog.tsx   # reusable confirmation modal
│       │   ├── DiffViewer.tsx      # side-by-side prompt diff
│       │   ├── JsonEditor.tsx      # JSON editor for tools, conditions
│       │   ├── VersionTimeline.tsx # visual version history with rollback buttons
│       │   └── StatusBadge.tsx     # active/inactive/healthy/unhealthy badges
│       └── types/
│           └── index.ts            # TypeScript interfaces matching API types
├── migrations/
│   ├── embed.go                    # //go:embed *.sql
│   ├── 001_extensions.up.sql       ... through 016
│   └── 016_seed_agents.up.sql
├── Dockerfile                      # multi-stage: Node build (web/) → Go build → alpine runtime
├── go.mod
├── go.sum
└── README.md
```

**Admin GUI serving:** The React app is built at Docker build time (`npm run build` in `web/`), then the `dist/` output is embedded into the Go binary via `//go:embed web/dist/*`. The Go server serves it at `/` as a single-page app with a catch-all fallback to `index.html`. API routes at `/api/v1/*` and `/auth/*` take priority over the SPA fallback.

### 2.3 Service Boundaries — Resources Owned

| Resource | Description |
|----------|-------------|
| **Agent** | Agent definition: ID, name, description, tools, trust overrides, example prompts |
| **AgentVersion** | Immutable snapshot of an agent at a point in time (for rollback) |
| **Prompt** | Versioned system prompt per agent, with template variables and mode |
| **MCPServer** | MCP server endpoint config: label, URL, auth, circuit breaker, discovery interval |
| **TrustRule** | Workspace-scoped tool trust override (glob pattern → tier) |
| **TrustDefault** | System-wide default trust classification patterns (the classifier's built-in rules) |
| **TriggerRule** | Workspace-scoped event → agent dispatch rule with rate limits and cron |
| **ModelConfig** | LLM parameters scoped globally, per-workspace, or per-user |
| **ContextConfig** | Context assembly layer budgets and enabled layers |
| **SignalConfig** | Signal source polling intervals (gmail, calendar, drive, slack) |
| **WebhookSubscription** | Consumer callback URL with event filter and HMAC secret |

---

## 3. Authentication and Authorization

The registry supports three authentication methods. Each serves a different use case; they coexist cleanly.

### 3.1 Authentication Methods

| Method | Use Case | How It Works |
|--------|----------|--------------|
| **Username + Password** | Bootstrap, local dev, fallback | User submits credentials to `POST /auth/login`; server verifies bcrypt hash; sets secure session cookie |
| **Google OAuth 2.0** | Production SSO for admin users | User clicks "Sign in with Google"; PKCE flow; server links Google account to existing or new user |
| **API Key** | Service-to-service (BFF → Registry) | Caller sends `Authorization: Bearer <key>` header; server verifies SHA-256 hash in DB |

### 3.2 User Account Model

```go
type User struct {
    ID              uuid.UUID  `json:"id" db:"id"`
    Username        string     `json:"username" db:"username"`            // unique, lowercase
    Email           string     `json:"email" db:"email"`                 // unique; may come from Google profile
    DisplayName     string     `json:"display_name" db:"display_name"`
    PasswordHash    string     `json:"-" db:"password_hash"`             // bcrypt; empty if OAuth-only
    Role            string     `json:"role" db:"role"`                   // "admin" | "editor" | "viewer"
    AuthMethod      string     `json:"auth_method" db:"auth_method"`     // "password" | "google" | "both"
    IsActive        bool       `json:"is_active" db:"is_active"`
    MustChangePass  bool       `json:"must_change_password" db:"must_change_pass"` // true for default admin
    FailedLogins    int        `json:"-" db:"failed_logins"`             // brute-force counter
    LockedUntil     *time.Time `json:"-" db:"locked_until"`              // account lockout
    LastLoginAt     *time.Time `json:"last_login_at" db:"last_login_at"`
    CreatedAt       time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type OAuthConnection struct {
    ID           uuid.UUID `json:"id" db:"id"`
    UserID       uuid.UUID `json:"user_id" db:"user_id"`
    Provider     string    `json:"provider" db:"provider"`        // "google"
    ProviderUID  string    `json:"-" db:"provider_uid"`           // Google sub claim
    Email        string    `json:"email" db:"email"`              // Google email
    DisplayName  string    `json:"display_name" db:"display_name"`
    AccessToken  string    `json:"-" db:"access_token"`           // encrypted
    RefreshToken string    `json:"-" db:"refresh_token"`          // encrypted
    ExpiresAt    time.Time `json:"-" db:"expires_at"`
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
```

### 3.3 Roles and Permissions

| Role | Can Read | Can Write | Can Manage Users | Can Manage API Keys |
|------|----------|-----------|------------------|---------------------|
| `viewer` | All resources | Nothing | No | Own keys only |
| `editor` | All resources | All resources (CRUD) | No | Own keys only |
| `admin` | All resources | All resources (CRUD) | Yes | All keys |

### 3.4 Default Admin Account (First Boot)

On first boot (when the `users` table is empty), the server creates:

```
Username:  admin
Password:  admin
Role:      admin
AuthMethod: password
MustChangePass: true
```

The admin GUI forces a password change on first login when `must_change_password` is `true`. The password policy requires: minimum 12 characters, at least 1 uppercase, 1 lowercase, 1 digit, 1 special character.

### 3.5 Password Authentication Flow

```
POST /auth/login
Content-Type: application/json

{ "username": "admin", "password": "new-secure-password-123!" }

→ 200 OK  (sets session cookie: __Host-session=<id>; HttpOnly; Secure; SameSite=Lax; Path=/)
→ 401     (invalid credentials; increments failed_logins)
→ 423     (account locked after 5 failed attempts; locked for 15 minutes)
```

**Brute-force protection:**
- After 5 consecutive failed login attempts, the account is locked for 15 minutes.
- `failed_logins` resets to 0 on successful login.
- Lockout duration doubles on each subsequent lockout (15m, 30m, 60m, max 24h).
- Response for locked accounts is intentionally identical to "invalid credentials" (no information leakage).

### 3.6 Google OAuth Flow

```
1. User clicks "Sign in with Google" on login page
2. Browser redirects to GET /auth/google/start
3. Server generates PKCE code_verifier + code_challenge
4. Server stores state + code_verifier in a short-lived cookie (5 min, encrypted)
5. Server redirects to Google authorization URL with:
   - response_type=code
   - scope=openid email profile
   - code_challenge_method=S256
   - code_challenge=<hash>
   - state=<random>
6. User consents at Google
7. Google redirects to GET /auth/google/callback?code=<code>&state=<state>
8. Server validates state against cookie
9. Server exchanges code for tokens using PKCE code_verifier
10. Server reads Google ID token: { sub, email, name }
11. Account linking (see below)
12. Server creates session, sets cookie, redirects to /
```

### 3.7 Account Linking Rules

When a Google OAuth callback arrives:

| Scenario | Behavior |
|----------|----------|
| `provider_uid` matches existing `oauth_connections` row | Log in as that user |
| No match, but Google email matches an existing user's email | **Link** Google to that user; set `auth_method = "google"` (disabling password login) |
| No match, no email match | Create new user with `auth_method = "google"`, `role = "viewer"` (admin must promote) |

**When `auth_method = "google"`:**
- Password login is **disabled** for that user.
- The login form shows a message: "This account uses Google sign-in."
- The user's `password_hash` is cleared.

### 3.8 Authentication Reset (Recovery)

If a user has `auth_method = "google"` and loses access to Google, an admin must reset them:

```
POST /api/v1/users/{userId}/reset-auth
Authorization: Bearer <admin_api_key_or_session>

{ "new_password": "temporary-password-123!", "force_change": true }

→ Sets auth_method = "password"
→ Removes all oauth_connections for this user
→ Sets must_change_pass = true
→ Sets the new temporary password (bcrypt hashed)
```

This endpoint requires `admin` role. It cannot be called by the user themselves (prevents social engineering).

Additionally, there is an **emergency CLI reset** for when the admin account itself is locked out:

```bash
# Run inside the container
/registry --reset-admin --new-password "new-admin-pass-123!"
```

This directly updates the database, bypasses all auth, and is only usable with shell access to the container. It logs an audit entry with `actor = "cli-reset"`.

### 3.9 API Key Authentication

API keys are for service-to-service communication (BFF → Registry). They bypass session/CSRF requirements.

```
GET /api/v1/agents
Authorization: Bearer rk_live_abc123def456...

→ Server computes SHA-256(key), looks up in api_keys table
→ If valid and not expired: proceed with scopes
→ If invalid: 401 Unauthorized
```

**API key format:** `rk_live_<32 hex chars>` (prefix helps identify the key type in logs).

**Key management endpoints (admin or own keys):**

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/api-keys` | Create new key; returns the key **once** (never stored in plaintext) |
| `GET /api/v1/api-keys` | List keys (shows name, scopes, last_used, prefix — never full key) |
| `DELETE /api/v1/api-keys/{keyId}` | Revoke a key |

### 3.10 Session Management

- Sessions are stored in PostgreSQL (not in-memory; survives restarts).
- Session ID is a cryptographically random 32-byte value, hex-encoded.
- Cookie name: `__Host-session` (the `__Host-` prefix enforces Secure + Path=/ + no Domain).
- Session TTL: 8 hours; refreshed on activity (sliding window).
- Idle timeout: 30 minutes of no requests → session invalidated.
- On logout (`POST /auth/logout`): session row deleted, cookie cleared.
- Background cleanup job: delete expired sessions every 10 minutes.

### 3.11 CSRF Protection

All state-changing requests from the GUI (POST, PUT, PATCH, DELETE) require a CSRF token:

- **Double-submit cookie pattern:**
  1. On login, server sets a second cookie: `__Host-csrf=<random token>; HttpOnly=false; Secure; SameSite=Lax`
  2. The React app reads this cookie and sends it as `X-CSRF-Token` header on every mutation request.
  3. The server validates that the header matches the cookie.
- API key requests are **exempt** from CSRF (they don't use cookies).
- GET/HEAD/OPTIONS requests are exempt.

### 3.12 Auth Middleware Priority

The middleware checks authentication in this order:

1. **API key** — if `Authorization: Bearer rk_live_...` header present → validate API key, skip session/CSRF
2. **Session cookie** — if `__Host-session` cookie present → look up session in DB, validate CSRF for mutations
3. **None** — return 401 Unauthorized

---

## 4. Resource Model

### 3.1 Agent

```go
type Agent struct {
    ID             string            `json:"id" db:"id"`                 // e.g. "pmo", "router" (primary key)
    Name           string            `json:"name" db:"name"`
    Description    string            `json:"description" db:"description"`
    SystemPrompt   string            `json:"system_prompt" db:"system_prompt"`
    Tools          []AgentTool       `json:"tools"`                      // JSONB
    TrustOverrides map[string]string `json:"trust_overrides"`            // JSONB: tool_name → "auto"|"review"|"block"
    Capabilities   []string          `json:"capabilities"`               // derived from internal tool names
    ExamplePrompts []string          `json:"example_prompts"`            // JSONB
    RequiredConns  []string          `json:"required_connections"`       // derived from MCP server labels
    IsActive       bool              `json:"is_active" db:"is_active"`
    Version        int               `json:"version" db:"version"`       // auto-incremented on each update
    CreatedBy      string            `json:"created_by" db:"created_by"`
    CreatedAt      time.Time         `json:"created_at" db:"created_at"`
    UpdatedAt      time.Time         `json:"updated_at" db:"updated_at"`
}

type AgentTool struct {
    Name        string `json:"name"`                       // tool function name
    Source      string `json:"source"`                     // "internal" or "mcp"
    ServerLabel string `json:"server_label,omitempty"`     // MCP server label; empty for internal tools
    Description string `json:"description"`
}
```

**Derived fields** (computed on read, not stored):
- `capabilities` = names of tools where `source == "internal"`
- `required_connections` = unique `server_label` values where `source == "mcp"` (excluding `"mcp-git"` which is always available)

### 3.2 AgentVersion

Immutable snapshot created on every agent update. Enables rollback.

```go
type AgentVersion struct {
    ID             uuid.UUID         `json:"id" db:"id"`
    AgentID        string            `json:"agent_id" db:"agent_id"`
    Version        int               `json:"version" db:"version"`
    Name           string            `json:"name" db:"name"`
    Description    string            `json:"description" db:"description"`
    SystemPrompt   string            `json:"system_prompt" db:"system_prompt"`
    Tools          []AgentTool       `json:"tools"`
    TrustOverrides map[string]string `json:"trust_overrides"`
    ExamplePrompts []string          `json:"example_prompts"`
    IsActive       bool              `json:"is_active" db:"is_active"`
    CreatedBy      string            `json:"created_by" db:"created_by"`
    CreatedAt      time.Time         `json:"created_at" db:"created_at"`
}
```

### 3.3 Prompt

```go
type Prompt struct {
    ID           uuid.UUID         `json:"id" db:"id"`
    AgentID      string            `json:"agent_id" db:"agent_id"`
    Version      int               `json:"version" db:"version"`
    SystemPrompt string            `json:"system_prompt" db:"system_prompt"`
    TemplateVars map[string]string `json:"template_vars"`               // JSONB: known variable names → default values
    Mode         string            `json:"mode" db:"mode"`              // "rag_readonly" | "toolcalling_safe" | "toolcalling_auto"
    IsActive     bool              `json:"is_active" db:"is_active"`
    CreatedBy    string            `json:"created_by" db:"created_by"`
    CreatedAt    time.Time         `json:"created_at" db:"created_at"`
}
```

**Prompt modes:**

| Mode | Behavior |
|------|----------|
| `rag_readonly` | Read-only RAG queries; no tool calling |
| `toolcalling_safe` | Tool calling with human-in-the-loop confirmation for Review/Block tiers |
| `toolcalling_auto` | Auto-execute all tools (used by meeting-processor and similar autonomous agents) |

**Template variable substitution:**
- Variables use `{{key}}` syntax in the `system_prompt` text.
- The `template_vars` map declares which variables the prompt expects and their defaults.
- Known variables from the existing system: `{{current_date}}`, `{{workspace_name}}`, `{{user_display_name}}`, `{{user_google_email}}`, `{{slack_channel_id}}`.
- The consuming BFF performs the actual substitution at runtime using `strings.ReplaceAll`.

### 3.4 MCPServer

```go
type MCPServer struct {
    ID                uuid.UUID `json:"id" db:"id"`
    Label             string    `json:"label" db:"label"`                         // unique key e.g. "mcp-git"
    Endpoint          string    `json:"endpoint" db:"endpoint"`                   // JSON-RPC or SSE URL
    AuthType          string    `json:"auth_type" db:"auth_type"`                 // "none" | "bearer" | "basic"
    AuthCredential    string    `json:"-" db:"auth_credential"`                   // encrypted; NEVER returned in API
    HealthEndpoint    string    `json:"health_endpoint" db:"health_endpoint"`     // optional
    CircuitBreakerCfg CBConfig  `json:"circuit_breaker"`                          // JSONB
    DiscoveryInterval string    `json:"discovery_interval" db:"discovery_interval"` // Go duration e.g. "5m"
    IsEnabled         bool      `json:"is_enabled" db:"is_enabled"`
    CreatedAt         time.Time `json:"created_at" db:"created_at"`
    UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type CBConfig struct {
    FailThreshold  int `json:"fail_threshold"`   // failures before opening circuit (default: 5)
    OpenDurationS  int `json:"open_duration_s"`  // seconds to stay open before half-open (default: 30)
}
```

**Credential encryption:** `auth_credential` is encrypted at rest using AES-256-GCM with the `CREDENTIAL_ENCRYPTION_KEY` env var. The API never returns this field.

### 3.5 TrustRule (workspace-scoped)

```go
type TrustRule struct {
    ID          uuid.UUID `json:"id" db:"id"`
    WorkspaceID uuid.UUID `json:"workspace_id" db:"workspace_id"`
    ToolPattern string    `json:"tool_pattern" db:"tool_pattern"`  // glob pattern: "get_*", "delete_event", "*"
    Tier        string    `json:"tier" db:"tier"`                  // "auto" | "review" | "block"
    CreatedBy   string    `json:"created_by" db:"created_by"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
```

**Trust tier semantics:**

| Tier | Behavior | Examples |
|------|----------|---------|
| `auto` | Execute immediately, no user prompt | `git_read_file`, `list_tasks`, `get_events` |
| `review` | Pause and show tool call details; require explicit user approval | `git_write_commit_push`, `create_task`, `modify_event` |
| `block` | Reject with explanation; require explicit confirmation + warning | `delete_event`, `send_email` (final), `force_push` |

### 3.6 TrustDefault (system-wide classification patterns)

These replace the hardcoded pattern-matching logic currently in `internal/trust/classifier.go`.

```go
type TrustDefault struct {
    ID        uuid.UUID `json:"id" db:"id"`
    Tier      string    `json:"tier" db:"tier"`
    Patterns  []string  `json:"patterns"`            // JSONB: substring/glob patterns
    Priority  int       `json:"priority" db:"priority"` // lower number = higher precedence
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
```

**Classification chain (priority order):**
1. MCP tool annotations (`ReadOnlyHint` → auto, `DestructiveHint` → block) — handled by BFF at runtime, not stored in registry
2. Agent-level `trust_overrides` — stored in Agent resource
3. Workspace-level `TrustRule` — stored in registry, matched by exact then glob then wildcard
4. System-wide `TrustDefault` — stored in registry, matched by substring containment
5. Fallback: `review` (safe-by-default)

**Seed data (matches current BFF `classifyByPattern`):**

| Tier | Patterns | Priority |
|------|----------|----------|
| `auto` | `["_read", "_list", "_search", "_get", "read_", "list_", "search_", "get_", "delegate_to_agent"]` | 1 |
| `block` | `["_delete", "_send", "_force", "delete_", "send_", "force_"]` | 2 |
| `review` | `["_write", "_create", "_update", "_commit", "write_", "create_", "update_", "commit_"]` | 3 |

### 3.7 TriggerRule (workspace-scoped)

```go
type TriggerRule struct {
    ID               uuid.UUID       `json:"id" db:"id"`
    WorkspaceID      uuid.UUID       `json:"workspace_id" db:"workspace_id"`
    Name             string          `json:"name" db:"name"`
    EventType        string          `json:"event_type" db:"event_type"`
    Condition        json.RawMessage `json:"condition" db:"condition"`          // JSONB: arbitrary filter criteria
    AgentID          string          `json:"agent_id" db:"agent_id"`           // which agent to invoke
    PromptTemplate   string          `json:"prompt_template" db:"prompt_template"` // input template with {{payload}}
    Enabled          bool            `json:"enabled" db:"enabled"`
    RateLimitPerHour int             `json:"rate_limit_per_hour" db:"rate_limit_per_hour"`
    Schedule         string          `json:"schedule" db:"schedule"`           // cron expression; empty for event-driven
    RunAsUserID      *uuid.UUID      `json:"run_as_user_id" db:"run_as_user_id"`
    CreatedAt        time.Time       `json:"created_at" db:"created_at"`
    UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}
```

**Well-known event types** (validated on create/update):

```
push, scheduled_tick, webhook, email.ingested, calendar.change,
transcript.available, drive.file_changed, slack.message, file_changed,
nudge_created
```

### 3.8 ModelConfig

```go
type ModelConfig struct {
    ID                   uuid.UUID `json:"id" db:"id"`
    Scope                string    `json:"scope" db:"scope"`                          // "global" | "workspace" | "user"
    ScopeID              string    `json:"scope_id" db:"scope_id"`                    // workspace_id or user_id; empty for global
    DefaultModel         string    `json:"default_model" db:"default_model"`
    Temperature          float64   `json:"temperature" db:"temperature"`
    MaxTokens            int       `json:"max_tokens" db:"max_tokens"`
    MaxToolRounds        int       `json:"max_tool_rounds" db:"max_tool_rounds"`
    DefaultContextWindow int       `json:"default_context_window" db:"default_context_window"`
    DefaultMaxOutput     int       `json:"default_max_output_tokens" db:"default_max_output_tokens"`
    HistoryTokenBudget   int       `json:"history_token_budget" db:"history_token_budget"`
    MaxHistoryMessages   int       `json:"max_history_messages" db:"max_history_messages"`
    EmbeddingModel       string    `json:"embedding_model" db:"embedding_model"`
    UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}
```

**Scope inheritance:** When reading model config, the registry merges: global → workspace → user. Non-zero values at narrower scopes override broader ones.

**Default seed values (from existing BFF constants):**

| Field | Default | Source |
|-------|---------|--------|
| `default_model` | `qwen3:8b` | `config.go` |
| `temperature` | `0.70` | `ModelConfigPanel.tsx` |
| `max_tokens` | `8192` | `runs/service.go:defaultMaxOutputTokens` |
| `max_tool_rounds` | `10` | `runs/service.go:maxToolRounds` |
| `default_context_window` | `128000` | `runs/service.go:defaultContextWindow` |
| `default_max_output_tokens` | `8192` | `runs/service.go:defaultMaxOutputTokens` |
| `history_token_budget` | `4000` | `runs/service.go:historyTokenBudget` |
| `max_history_messages` | `20` | `runs/service.go:maxHistoryMessages` |
| `embedding_model` | `nomic-embed-text:latest` | `config.go` |

### 3.9 ContextConfig

```go
type ContextConfig struct {
    ID             uuid.UUID      `json:"id" db:"id"`
    Scope          string         `json:"scope" db:"scope"`              // "global" | "workspace"
    ScopeID        string         `json:"scope_id" db:"scope_id"`
    MaxTotalTokens int            `json:"max_total_tokens" db:"max_total_tokens"`
    LayerBudgets   map[string]int `json:"layer_budgets"`                 // JSONB: layer_name → token budget
    EnabledLayers  []string       `json:"enabled_layers"`                // JSONB: ordered layer names
    UpdatedAt      time.Time      `json:"updated_at" db:"updated_at"`
}
```

**Default seed values (from existing BFF context assembler):**

| Field | Default |
|-------|---------|
| `max_total_tokens` | `18000` |
| `layer_budgets` | `{"workspace_structure": 500, "ui_context": 200, "semantic_retrieval": 4000, "file_content": 8000, "conversation_memory": 4000, "domain_state": 1000}` |
| `enabled_layers` | `["workspace_structure", "ui_context", "semantic_retrieval", "file_content", "conversation_memory", "domain_state"]` |

### 3.10 SignalConfig

```go
type SignalConfig struct {
    ID           uuid.UUID `json:"id" db:"id"`
    Source       string    `json:"source" db:"source"`               // unique: "gmail", "calendar", "drive", "slack"
    PollInterval string    `json:"poll_interval" db:"poll_interval"` // Go duration: "15m", "30s", "1h"
    IsEnabled    bool      `json:"is_enabled" db:"is_enabled"`
    UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
```

**Default seed values (from existing BFF `signals/manager.go`):**

| Source | Interval |
|--------|----------|
| `gmail` | `15m` |
| `calendar` | `1h` |
| `drive` | `30m` |
| `slack` | `30s` |

### 3.11 WebhookSubscription

```go
type WebhookSubscription struct {
    ID        uuid.UUID `json:"id" db:"id"`
    URL       string    `json:"url" db:"url"`               // callback endpoint
    Secret    string    `json:"-" db:"secret"`              // HMAC-SHA256 signing secret; never returned
    Events    []string  `json:"events"`                     // JSONB: event type filter
    IsActive  bool      `json:"is_active" db:"is_active"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
```

---

## 5. API Specification

**Base path:** `/api/v1` for resource endpoints; `/auth` for authentication.

**Content type:** `application/json` for all requests and responses.

**Error envelope** (consistent across all endpoints):

```json
{
  "code": "VALIDATION_ERROR",
  "message": "agent_id is required",
  "details": { "field": "agent_id" }
}
```

**Status codes:**

| Code | Usage |
|------|-------|
| `200` | OK (read, update) |
| `201` | Created |
| `204` | No Content (successful DELETE) |
| `400` | Validation error |
| `401` | Unauthorized (not authenticated) |
| `403` | Forbidden (authenticated but insufficient role) |
| `404` | Not found |
| `409` | Conflict (concurrent edit or duplicate) |
| `423` | Locked (account locked due to failed logins) |
| `429` | Too Many Requests (rate limited) |
| `500` | Internal error |

**Authentication:** Either a valid session cookie (from GUI login) or `Authorization: Bearer <api_key>` header. See Section 3 for details.

**Optimistic concurrency:** PUT and PATCH operations require an `If-Match` header containing the resource's `updated_at` timestamp. If the value doesn't match the current `updated_at`, the server returns `409 Conflict`.

---

### 5.0 Auth Endpoints

These are outside the `/api/v1` base path.

#### `POST /auth/login`

Username + password login. Sets session cookie on success.

**Request body:**
```json
{ "username": "admin", "password": "my-secure-password-123!" }
```

**Response 200:**
```json
{
  "user": { "id": "uuid", "username": "admin", "email": "admin@example.com", "role": "admin", "auth_method": "password", "must_change_password": false },
  "csrf_token": "random-token-value"
}
```

**Response 401:** `{ "code": "INVALID_CREDENTIALS", "message": "Invalid username or password" }`

**Response 423:** `{ "code": "ACCOUNT_LOCKED", "message": "Account temporarily locked. Try again later." }`

#### `POST /auth/logout`

Destroy session. Clears cookies.

**Response 204:** No content.

#### `GET /auth/me`

Get current authenticated user (from session or API key).

**Response 200:** User object.

**Response 401:** Not authenticated.

#### `POST /auth/change-password`

Change password for the current session user. Required when `must_change_password` is true.

**Request body:**
```json
{ "current_password": "admin", "new_password": "new-secure-password-123!" }
```

**Response 200:** `{ "message": "Password changed successfully" }`

**Response 400:** Password policy violation (returns specific requirements not met).

#### `GET /auth/google/start`

Initiate Google OAuth flow. Redirects to Google consent screen. Sets PKCE state cookie.

#### `GET /auth/google/callback`

Google OAuth callback. Exchanges code, links or creates account, sets session. Redirects to `/`.

#### `POST /auth/unlink-google`

Unlink Google account from current user. Only works if user has a password set. Sets `auth_method = "password"`.

**Response 200:** Updated user object.

**Response 400:** `{ "code": "NO_PASSWORD", "message": "Set a password before unlinking Google" }`

---

### 5.1 Agents

#### `GET /api/v1/agents`

List all agents.

| Query Param | Type | Default | Description |
|-------------|------|---------|-------------|
| `active_only` | bool | `true` | Only return active agents |
| `include_tools` | bool | `true` | Include the `tools` array in response |

**Response 200:**

```json
{
  "agents": [
    {
      "id": "pmo",
      "name": "PMO Agent",
      "description": "Governance, compliance, reporting, repo health.",
      "system_prompt": "You are the PMO Agent...",
      "tools": [
        { "name": "get_project_config", "source": "internal", "server_label": "", "description": "Read workspace project configuration" },
        { "name": "git_read_file", "source": "mcp", "server_label": "mcp-git", "description": "Read a file from the Git repository" }
      ],
      "trust_overrides": { "git_read_file": "auto", "git_list_files": "auto" },
      "capabilities": ["get_project_config", "get_health_score", "update_changelog"],
      "example_prompts": ["Run a health check", "Update the changelog"],
      "required_connections": ["google-workspace-mcp", "slack-mcp"],
      "is_active": true,
      "version": 3,
      "created_by": "system",
      "created_at": "2026-01-15T10:00:00Z",
      "updated_at": "2026-02-10T14:30:00Z"
    }
  ],
  "total": 6
}
```

#### `GET /api/v1/agents/{agentId}`

Get a single agent.

**Response 200:** Single agent object (same shape as list item).

**Response 404:** `{ "code": "NOT_FOUND", "message": "agent 'nonexistent' not found" }`

#### `POST /api/v1/agents`

Create a new agent. Automatically creates version 1 snapshot.

**Request body:**

```json
{
  "id": "knowledge_steward",
  "name": "Knowledge Steward",
  "description": "Manages glossary, ADRs, documentation coherence.",
  "system_prompt": "You are the Knowledge Steward for workspace \"{{workspace_name}}\"...",
  "tools": [
    { "name": "ingest_document", "source": "internal", "description": "Ingest document into RAG index" },
    { "name": "git_read_file", "source": "mcp", "server_label": "mcp-git", "description": "Read a file" }
  ],
  "trust_overrides": { "git_read_file": "auto" },
  "example_prompts": ["Index the latest meeting notes", "Search for budget documents"]
}
```

**Response 201:** Created agent object.

**Response 409:** `{ "code": "CONFLICT", "message": "agent 'knowledge_steward' already exists" }`

#### `PUT /api/v1/agents/{agentId}`

Full update. Creates a new version snapshot. Requires `If-Match` header.

**Request headers:** `If-Match: "2026-02-10T14:30:00Z"`

**Request body:** Same shape as POST (all fields required except `id`).

**Response 200:** Updated agent with incremented version.

**Response 409:** `{ "code": "CONFLICT", "message": "resource was modified by another client" }`

#### `PATCH /api/v1/agents/{agentId}`

Partial update. Only provided fields are changed. Creates a new version snapshot. Requires `If-Match` header.

**Request body (example — only updating description and tools):**

```json
{
  "description": "Updated description",
  "tools": [ ... ]
}
```

**Response 200:** Updated agent object.

#### `DELETE /api/v1/agents/{agentId}`

Soft-delete: sets `is_active = false`.

**Response 204:** No content.

#### `GET /api/v1/agents/{agentId}/versions`

List all version snapshots.

| Query Param | Type | Default |
|-------------|------|---------|
| `limit` | int | `20` |
| `offset` | int | `0` |

**Response 200:**

```json
{
  "versions": [
    { "id": "uuid", "agent_id": "pmo", "version": 3, "name": "PMO Agent", "is_active": true, "created_by": "admin", "created_at": "2026-02-10T..." },
    { "id": "uuid", "agent_id": "pmo", "version": 2, "name": "PMO Agent", "is_active": true, "created_by": "admin", "created_at": "2026-02-08T..." }
  ],
  "total": 3
}
```

#### `GET /api/v1/agents/{agentId}/versions/{version}`

Get a specific version snapshot with full data.

**Response 200:** Full AgentVersion object.

#### `POST /api/v1/agents/{agentId}/rollback`

Rollback to a previous version. Creates version N+1 containing the data from the target version.

**Request body:**

```json
{ "target_version": 2 }
```

**Response 200:** New agent state (version N+1 with data copied from version 2).

---

### 5.2 Prompts

#### `GET /api/v1/agents/{agentId}/prompts`

List all prompt versions for an agent.

| Query Param | Type | Default |
|-------------|------|---------|
| `active_only` | bool | `false` |
| `limit` | int | `20` |
| `offset` | int | `0` |

**Response 200:**

```json
{
  "prompts": [
    {
      "id": "uuid",
      "agent_id": "pmo",
      "version": 4,
      "system_prompt": "You are the PMO Agent for workspace \"{{workspace_name}}\"...",
      "template_vars": { "workspace_name": "", "current_date": "" },
      "mode": "toolcalling_safe",
      "is_active": true,
      "created_by": "admin@example.com",
      "created_at": "2026-02-10T14:30:00Z"
    }
  ],
  "total": 4
}
```

#### `GET /api/v1/agents/{agentId}/prompts/active`

Get the currently active prompt. Returns the agent's `system_prompt` field as fallback if no prompt record exists.

**Response 200:** Single prompt object.

**Response 404:** No active prompt and no fallback.

#### `GET /api/v1/agents/{agentId}/prompts/{promptId}`

Get a specific prompt by ID.

#### `POST /api/v1/agents/{agentId}/prompts`

Create a new prompt version. Automatically deactivates the previous active prompt and assigns the next version number. This is transactional.

**Request body:**

```json
{
  "system_prompt": "You are the PMO Agent for workspace \"{{workspace_name}}\"...",
  "template_vars": { "workspace_name": "", "current_date": "" },
  "mode": "toolcalling_safe",
  "created_by": "admin@example.com"
}
```

**Response 201:** Created prompt with assigned version number.

#### `POST /api/v1/agents/{agentId}/prompts/{promptId}/activate`

Activate a specific prompt version (deactivates the currently active one).

**Response 200:** Activated prompt object.

#### `POST /api/v1/agents/{agentId}/prompts/rollback`

Rollback: creates a new prompt version (N+1) with the content of the target version, and activates it.

**Request body:**

```json
{ "target_version": 2 }
```

**Response 200:** New prompt (version N+1, now active).

---

### 5.3 MCP Servers

#### `GET /api/v1/mcp-servers`

List all MCP servers. The `auth_credential` field is never included.

**Response 200:**

```json
{
  "servers": [
    {
      "id": "uuid",
      "label": "mcp-git",
      "endpoint": "http://mcp-git:8080/mcp",
      "auth_type": "none",
      "health_endpoint": "http://mcp-git:8080/health",
      "circuit_breaker": { "fail_threshold": 5, "open_duration_s": 30 },
      "discovery_interval": "5m",
      "is_enabled": true,
      "created_at": "2026-01-15T...",
      "updated_at": "2026-01-15T..."
    }
  ],
  "total": 4
}
```

#### `GET /api/v1/mcp-servers/{serverId}`

Get a single MCP server config.

#### `POST /api/v1/mcp-servers`

Create a new MCP server.

**Request body:**

```json
{
  "label": "slack-mcp",
  "endpoint": "http://slack-mcp:8080/sse",
  "auth_type": "bearer",
  "auth_credential": "xoxb-...",
  "health_endpoint": "",
  "circuit_breaker": { "fail_threshold": 5, "open_duration_s": 30 },
  "discovery_interval": "5m"
}
```

**Response 201:** Created server (without `auth_credential`).

**Response 409:** `{ "code": "CONFLICT", "message": "MCP server with label 'slack-mcp' already exists" }`

#### `PUT /api/v1/mcp-servers/{serverId}`

Full update. Requires `If-Match`.

#### `DELETE /api/v1/mcp-servers/{serverId}`

Hard delete. Returns 204.

---

### 5.4 Trust Rules (workspace-scoped)

#### `GET /api/v1/workspaces/{workspaceId}/trust-rules`

List all trust rules for a workspace.

**Response 200:**

```json
{
  "rules": [
    { "id": "uuid", "workspace_id": "uuid", "tool_pattern": "get_*", "tier": "auto", "created_by": "admin", "created_at": "...", "updated_at": "..." }
  ],
  "total": 3
}
```

#### `POST /api/v1/workspaces/{workspaceId}/trust-rules`

Create or upsert a trust rule (unique on `workspace_id + tool_pattern`).

**Request body:**

```json
{
  "tool_pattern": "git_write_*",
  "tier": "review",
  "created_by": "admin"
}
```

**Response 201:** Created/updated rule.

#### `DELETE /api/v1/workspaces/{workspaceId}/trust-rules/{ruleId}`

Delete a trust rule. Returns 204.

---

### 5.5 Trust Defaults (system-wide)

#### `GET /api/v1/trust-defaults`

List all default trust classification patterns, ordered by priority.

**Response 200:**

```json
{
  "defaults": [
    { "id": "uuid", "tier": "auto", "patterns": ["_read", "_list", "_search", "_get", "read_", "list_", "search_", "get_", "delegate_to_agent"], "priority": 1, "updated_at": "..." },
    { "id": "uuid", "tier": "block", "patterns": ["_delete", "_send", "_force", "delete_", "send_", "force_"], "priority": 2, "updated_at": "..." },
    { "id": "uuid", "tier": "review", "patterns": ["_write", "_create", "_update", "_commit", "write_", "create_", "update_", "commit_"], "priority": 3, "updated_at": "..." }
  ]
}
```

#### `PUT /api/v1/trust-defaults/{defaultId}`

Update a default entry. Requires `If-Match`.

**Request body:**

```json
{
  "tier": "auto",
  "patterns": ["_read", "_list", "_search", "_get", "read_", "list_", "search_", "get_", "delegate_to_agent", "query_*"],
  "priority": 1
}
```

**Response 200:** Updated entry.

---

### 5.6 Trigger Rules (workspace-scoped)

#### `GET /api/v1/workspaces/{workspaceId}/trigger-rules`

List all trigger rules for a workspace.

**Response 200:**

```json
{
  "triggers": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "name": "Email Triage",
      "event_type": "email.ingested",
      "condition": {},
      "agent_id": "router",
      "prompt_template": "Triage this email: {{payload}}",
      "enabled": true,
      "rate_limit_per_hour": 10,
      "schedule": "",
      "run_as_user_id": null,
      "created_at": "...",
      "updated_at": "..."
    }
  ],
  "total": 5
}
```

#### `GET /api/v1/workspaces/{workspaceId}/trigger-rules/{triggerId}`

Get a single trigger rule.

#### `POST /api/v1/workspaces/{workspaceId}/trigger-rules`

Create a trigger rule.

**Request body:**

```json
{
  "name": "Email Triage",
  "event_type": "email.ingested",
  "condition": {},
  "agent_id": "router",
  "prompt_template": "Triage this email: {{payload}}",
  "enabled": true,
  "rate_limit_per_hour": 10,
  "schedule": "",
  "run_as_user_id": null
}
```

**Validation:**
- `event_type` must be one of the well-known types (section 3.7)
- `agent_id` must reference an existing active agent
- `schedule` is validated as a valid cron expression (using `robfig/cron/v3`)

**Response 201:** Created trigger rule.

#### `PUT /api/v1/workspaces/{workspaceId}/trigger-rules/{triggerId}`

Full update. Requires `If-Match`.

#### `DELETE /api/v1/workspaces/{workspaceId}/trigger-rules/{triggerId}`

Delete. Returns 204.

---

### 5.7 Model Configuration

#### `GET /api/v1/model-config`

Get the global model configuration.

**Response 200:**

```json
{
  "id": "uuid",
  "scope": "global",
  "scope_id": "",
  "default_model": "qwen3:8b",
  "temperature": 0.70,
  "max_tokens": 8192,
  "max_tool_rounds": 10,
  "default_context_window": 128000,
  "default_max_output_tokens": 8192,
  "history_token_budget": 4000,
  "max_history_messages": 20,
  "embedding_model": "nomic-embed-text:latest",
  "updated_at": "..."
}
```

#### `PUT /api/v1/model-config`

Update the global model configuration. Requires `If-Match`.

#### `GET /api/v1/workspaces/{workspaceId}/model-config`

Get workspace-scoped model configuration. Returns the global config merged with workspace overrides. Fields set at workspace scope override global.

#### `PUT /api/v1/workspaces/{workspaceId}/model-config`

Set workspace-scoped overrides. Only fields present in the request body are stored as overrides.

---

### 5.8 Context Assembly Configuration

#### `GET /api/v1/context-config`

Get the global context assembly configuration.

**Response 200:**

```json
{
  "id": "uuid",
  "scope": "global",
  "scope_id": "",
  "max_total_tokens": 18000,
  "layer_budgets": {
    "workspace_structure": 500,
    "ui_context": 200,
    "semantic_retrieval": 4000,
    "file_content": 8000,
    "conversation_memory": 4000,
    "domain_state": 1000
  },
  "enabled_layers": ["workspace_structure", "ui_context", "semantic_retrieval", "file_content", "conversation_memory", "domain_state"],
  "updated_at": "..."
}
```

#### `PUT /api/v1/context-config`

Update the global context configuration. Requires `If-Match`.

#### `GET /api/v1/workspaces/{workspaceId}/context-config`

Get workspace-scoped context config (merged with global).

#### `PUT /api/v1/workspaces/{workspaceId}/context-config`

Set workspace-scoped overrides.

---

### 5.9 Signal Polling Configuration

#### `GET /api/v1/signal-config`

List all signal polling configurations.

**Response 200:**

```json
{
  "signals": [
    { "id": "uuid", "source": "gmail", "poll_interval": "15m", "is_enabled": true, "updated_at": "..." },
    { "id": "uuid", "source": "calendar", "poll_interval": "1h", "is_enabled": true, "updated_at": "..." },
    { "id": "uuid", "source": "drive", "poll_interval": "30m", "is_enabled": true, "updated_at": "..." },
    { "id": "uuid", "source": "slack", "poll_interval": "30s", "is_enabled": true, "updated_at": "..." }
  ]
}
```

#### `PUT /api/v1/signal-config/{signalId}`

Update a signal config. Requires `If-Match`.

**Request body:**

```json
{
  "poll_interval": "10m",
  "is_enabled": true
}
```

---

### 5.10 Webhook Subscriptions

#### `GET /api/v1/webhooks`

List all webhook subscriptions (secret is never returned).

#### `POST /api/v1/webhooks`

Register a new webhook.

**Request body:**

```json
{
  "url": "http://bff:8082/internal/registry-webhook",
  "secret": "whsec_random_secret_here",
  "events": [
    "agent.created", "agent.updated", "agent.deleted", "agent.rolled_back",
    "prompt.created", "prompt.activated", "prompt.rolled_back",
    "mcp_server.created", "mcp_server.updated", "mcp_server.deleted",
    "trust_rule.changed", "trust_default.changed",
    "trigger_rule.changed",
    "model_config.updated", "context_config.updated", "signal_config.updated"
  ]
}
```

**Response 201:** Created subscription (without `secret`).

#### `DELETE /api/v1/webhooks/{webhookId}`

Remove a subscription. Returns 204.

**Webhook delivery payload (sent to subscriber URL):**

```json
{
  "event": "agent.updated",
  "resource_type": "agent",
  "resource_id": "pmo",
  "timestamp": "2026-02-14T12:00:00Z",
  "actor": "admin"
}
```

**Webhook delivery headers:**

```
Content-Type: application/json
X-Webhook-Signature: sha256=<HMAC-SHA256(secret, body)>
X-Webhook-Event: agent.updated
X-Registry-Delivery: <unique delivery UUID>
```

**Delivery behavior:**
- Async dispatch via worker pool (configurable worker count)
- Retry with exponential backoff: 1s, 2s, 4s (configurable max retries)
- Timeout per delivery (configurable, default 5s)
- Failed deliveries logged with delivery ID, status code, and error

---

### 5.11 Discovery Endpoint (composite)

#### `GET /api/v1/discovery`

Returns all active configuration in a single response. Designed for BFF cold-start — one call to hydrate all caches.

**Response 200:**

```json
{
  "agents": [ ... ],
  "mcp_servers": [ ... ],
  "trust_defaults": [ ... ],
  "model_config": { ... },
  "context_config": { ... },
  "signal_config": [ ... ],
  "fetched_at": "2026-02-14T12:00:00Z"
}
```

This endpoint returns only active/enabled resources. Prompts are not included (they are agent-scoped and resolved on demand). Trust rules and trigger rules are workspace-scoped and fetched separately.

---

### 5.12 Health Endpoints

#### `GET /healthz`

Liveness probe. Returns 200 if the process is running.

```json
{ "status": "ok" }
```

#### `GET /readyz`

Readiness probe. Returns 200 only if the database is reachable (executes `SELECT 1`).

```json
{ "status": "ready" }
```

#### `GET /metrics`

Prometheus metrics endpoint (when metrics are enabled).

---

## 6. Admin GUI

The registry ships with a built-in admin GUI — a React + PatternFly 5 single-page application embedded in the Go binary. It is the primary way humans interact with the registry. No separate frontend deployment is needed.

### 6.1 Technology Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Framework | React 18 | Matches Agent Smit frontend; team already knows it |
| UI Library | PatternFly 5 | Red Hat design system; consistent with Agent Smit |
| Build Tool | Vite 5 | Fast builds; same as Agent Smit frontend |
| TypeScript | 5.6+ strict | Type safety |
| Testing | Vitest + Testing Library | Same as Agent Smit frontend |
| Embedding | Go `embed.FS` | Single binary deployment; no separate static file server |

### 6.2 Pages and Navigation

**Sidebar navigation (PatternFly Nav):**

```
Dashboard
─────────────
Agents
  └─ Agent Detail (dynamic route)
Prompts
MCP Servers
─────────────
Trust Rules
  ├─ Defaults
  └─ Workspace Rules
Trigger Rules
─────────────
Configuration
  ├─ Model Config
  ├─ Context Config
  └─ Signal Polling
─────────────
System
  ├─ Webhooks
  ├─ API Keys
  ├─ Users
  └─ Audit Log
─────────────
My Account
```

### 6.3 Page Specifications

#### Dashboard
- **Cards:** Total agents (active/inactive), total prompt versions, MCP servers (healthy/unhealthy), pending webhook deliveries.
- **Recent Activity:** Last 20 audit log entries (resource type, action, actor, timestamp) with link to affected resource.
- **System Health:** Registry uptime, DB connection pool status, session count.

#### Agents Page
- **Table:** ID, Name, Version, Tools count, Active status, Last Updated.
- **Actions:** Create New (button), Edit (row click), Toggle Active, Delete (with confirmation).
- **Bulk actions:** None (agents are individually managed to prevent accidents).
- **Search/filter:** Filter by name, active status.

#### Agent Detail Page
- **Tabs:**
  1. **General** — Edit name, description, example prompts (form fields).
  2. **Tools** — Add/remove tools with source selector (internal/MCP), server label picker (dropdown from MCP servers), description. Drag-to-reorder.
  3. **Trust Overrides** — Table of tool_name → tier mappings; add/remove rows.
  4. **System Prompt** — Full-screen code editor (monospace) with `{{variable}}` syntax highlighting. Live preview of template variable substitution.
  5. **Prompt Versions** — Timeline of all prompt versions with diff view (side-by-side). Activate button per version. Rollback button.
  6. **Version History** — Timeline of agent config versions with diff. Rollback button.
- **Save:** Creates new version + dispatches webhook. Shows "Saved as version N" toast.

#### Prompts Page
- **Table:** Agent ID, Version, Mode, Active, Created By, Created At.
- **Filters:** By agent, by mode, active only.
- **Actions:** View diff between any two versions, activate a version, create new version.
- **Diff viewer:** Side-by-side with red/green highlighting (like GitHub diff).

#### MCP Servers Page
- **Table:** Label, Endpoint, Auth Type, Health Status, Discovery Interval, Enabled.
- **Health indicator:** Green/red/yellow badge. Health is checked client-side by calling `GET /api/v1/mcp-servers/{id}/health` (the registry proxies to the server's health endpoint).
- **Create/Edit modal:** Label, Endpoint URL, Auth type dropdown, Credential (masked input, only settable not readable), Health URL, Circuit breaker config, Discovery interval.
- **Test Connection button:** Calls the MCP server's health endpoint and shows result.

#### Trust Page
- **Two tabs:**
  1. **System Defaults** — Editable table of tier → patterns. Priority ordering.
  2. **Workspace Rules** — Workspace selector, then table of pattern → tier. Add/delete rows.

#### Trigger Rules Page
- **Table:** Name, Event Type, Agent, Enabled, Rate Limit, Schedule, Last Triggered.
- **Create/Edit form:** Name, event type dropdown (well-known types), agent selector (dropdown from active agents), condition JSON editor, prompt template textarea, rate limit, cron schedule, enabled toggle.
- **Cron helper:** Human-readable description next to cron input (e.g., "Every day at 9:00 AM").

#### Model Config Page
- **Form (global):** Default model (text), temperature (slider 0.0–1.0), max tokens (number), max tool rounds (number), context window (number), max output tokens, history budget, max history messages, embedding model.
- **Workspace overrides:** Workspace selector, then same form with "inherit from global" toggle per field.

#### Context Config Page
- **Visual layer editor:** Draggable list of layers with token budget sliders. Enable/disable toggle per layer. Total budget bar chart showing allocation.

#### Signal Polling Page
- **Table:** Source, Poll Interval, Enabled.
- **Edit:** Inline interval input with duration validation (e.g., "30s", "15m", "1h").

#### Webhooks Page
- **Table:** URL, Events (tag list), Active, Created At.
- **Create modal:** URL, event checkboxes (grouped by resource type), secret (auto-generated, shown once).
- **Test button:** Sends a `ping` event to the webhook URL and shows response.

#### API Keys Page
- **Table:** Name, Key Prefix (`rk_live_abc1...`), Scopes, Created At, Last Used, Expires At.
- **Create modal:** Name, scope checkboxes, optional expiry. Shows the full key **once** in a copy-to-clipboard dialog. Warning: "This key will not be shown again."
- **Revoke:** Confirmation dialog.

#### Users Page (admin only)
- **Table:** Username, Email, Role, Auth Method, Active, Last Login.
- **Create modal:** Username, email, role selector, initial password.
- **Actions per user:** Edit role, Toggle active, Reset auth (set new password + clear OAuth), Force password change.

#### Audit Log Page
- **Table:** Timestamp, Actor (username or API key name), Action, Resource Type, Resource ID, Details.
- **Filters:** Date range, actor, resource type, action.
- **Pagination:** 50 entries per page.

#### My Account Page
- **Password section:** Change password form (current + new + confirm). Hidden if `auth_method = "google"`.
- **Google section:** "Connected as user@gmail.com" with Unlink button, or "Connect Google Account" button.
- **API Keys section:** Personal API keys (viewer/editor can manage their own).
- **Sessions section:** List active sessions with "Sign out everywhere" button.

### 6.4 UX Principles

- **Confirmation for destructive actions:** Delete, rollback, deactivate, revoke always show a PatternFly `Modal` with the resource name and a "Type the name to confirm" input for critical operations (delete agent, revoke API key).
- **Toast notifications:** All mutations show a success/error toast (PatternFly `Alert` in `AlertGroup`).
- **Optimistic concurrency in UI:** On 409 Conflict, show a "This resource was modified by someone else. Reload?" dialog.
- **Loading states:** Every page shows a `Spinner` or `Skeleton` during fetch.
- **Empty states:** Every table shows a PatternFly `EmptyState` with helpful message and create button.
- **Error states:** API errors show an inline `Alert` with the error message and a retry button.
- **Responsive:** Works on desktop and tablet (min 768px width). No mobile-specific layout needed (admin tool).
- **Dark mode:** Respects PatternFly `prefers-color-scheme` automatically.
- **Force password change:** If `must_change_password` is true, the only accessible page is the change-password form. All other routes redirect there.

---

## 7. Database Schema

All migrations use the `golang-migrate` format: `NNN_name.up.sql` / `NNN_name.down.sql`, embedded via `//go:embed`.

### Migration 001: Extensions

```sql
-- 001_extensions.up.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
```

```sql
-- 001_extensions.down.sql
DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
```

### Migration 002: Users

```sql
-- 002_users.up.sql
CREATE TABLE users (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username         VARCHAR(100) NOT NULL UNIQUE,
    email            VARCHAR(255) NOT NULL UNIQUE,
    display_name     VARCHAR(200) NOT NULL DEFAULT '',
    password_hash    TEXT NOT NULL DEFAULT '',            -- bcrypt; empty if OAuth-only
    role             VARCHAR(20) NOT NULL DEFAULT 'viewer'
                     CHECK (role IN ('admin', 'editor', 'viewer')),
    auth_method      VARCHAR(20) NOT NULL DEFAULT 'password'
                     CHECK (auth_method IN ('password', 'google', 'both')),
    is_active        BOOLEAN NOT NULL DEFAULT true,
    must_change_pass BOOLEAN NOT NULL DEFAULT false,
    failed_logins    INT NOT NULL DEFAULT 0,
    locked_until     TIMESTAMPTZ,
    last_login_at    TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_username ON users(username) WHERE is_active = true;
CREATE INDEX idx_users_email ON users(email) WHERE is_active = true;
```

### Migration 003: OAuth Connections

```sql
-- 003_oauth_connections.up.sql
CREATE TABLE oauth_connections (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      VARCHAR(20) NOT NULL DEFAULT 'google',
    provider_uid  VARCHAR(200) NOT NULL,                   -- Google 'sub' claim
    email         VARCHAR(255) NOT NULL,
    display_name  VARCHAR(200) NOT NULL DEFAULT '',
    access_token  TEXT NOT NULL DEFAULT '',                 -- encrypted with CREDENTIAL_ENCRYPTION_KEY
    refresh_token TEXT NOT NULL DEFAULT '',                 -- encrypted
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_uid)
);

CREATE INDEX idx_oauth_user ON oauth_connections(user_id);
CREATE INDEX idx_oauth_provider ON oauth_connections(provider, provider_uid);
```

### Migration 004: Sessions

```sql
-- 004_sessions.up.sql
CREATE TABLE sessions (
    id          VARCHAR(64) PRIMARY KEY,                   -- hex-encoded 32 random bytes
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token  VARCHAR(64) NOT NULL,
    ip_address  INET,
    user_agent  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '8 hours')
);

CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);
```

### Migration 005: API Keys

```sql
-- 005_api_keys.up.sql
CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,  -- null = system key
    name         VARCHAR(100) NOT NULL,
    key_prefix   VARCHAR(20) NOT NULL DEFAULT '',               -- first 12 chars for display
    key_hash     VARCHAR(64) NOT NULL UNIQUE,                   -- SHA-256 hex digest
    scopes       TEXT[] NOT NULL DEFAULT '{}',                   -- {"read", "write", "admin"}
    is_active    BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_hash ON api_keys(key_hash) WHERE is_active = true;
CREATE INDEX idx_api_keys_user ON api_keys(user_id);
```

### Migration 006: Audit Log

```sql
-- 006_audit_log.up.sql
CREATE TABLE audit_log (
    id            BIGSERIAL PRIMARY KEY,
    actor         VARCHAR(200) NOT NULL,                -- username, API key name, or "system"
    actor_id      UUID,                                 -- user_id or api_key_id
    action        VARCHAR(50) NOT NULL,                 -- "create", "update", "delete", "login", "logout", etc.
    resource_type VARCHAR(50) NOT NULL,                 -- "agent", "prompt", "user", "session", etc.
    resource_id   VARCHAR(200) NOT NULL DEFAULT '',
    details       JSONB NOT NULL DEFAULT '{}',          -- additional context (changed fields, IP, etc.)
    ip_address    INET,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_time ON audit_log(created_at DESC);
CREATE INDEX idx_audit_log_actor ON audit_log(actor);
CREATE INDEX idx_audit_log_resource ON audit_log(resource_type, resource_id);
```

### Migration 007: Agents

```sql
-- 007_agents.up.sql
CREATE TABLE agents (
    id              VARCHAR(50) PRIMARY KEY,
    name            VARCHAR(200) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    system_prompt   TEXT NOT NULL DEFAULT '',
    tools           JSONB NOT NULL DEFAULT '[]',
    trust_overrides JSONB NOT NULL DEFAULT '{}',
    example_prompts JSONB NOT NULL DEFAULT '[]',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    version         INT NOT NULL DEFAULT 1,
    created_by      VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agent_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        VARCHAR(50) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version         INT NOT NULL,
    name            VARCHAR(200) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    system_prompt   TEXT NOT NULL DEFAULT '',
    tools           JSONB NOT NULL DEFAULT '[]',
    trust_overrides JSONB NOT NULL DEFAULT '{}',
    example_prompts JSONB NOT NULL DEFAULT '[]',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_by      VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(agent_id, version)
);

CREATE INDEX idx_agent_versions_agent ON agent_versions(agent_id, version DESC);
```

### Migration 008: Prompts

```sql
-- 008_prompts.up.sql
CREATE TABLE prompts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      VARCHAR(50) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version       INT NOT NULL DEFAULT 1,
    system_prompt TEXT NOT NULL,
    template_vars JSONB NOT NULL DEFAULT '{}',
    mode          VARCHAR(30) NOT NULL DEFAULT 'toolcalling_safe'
                  CHECK (mode IN ('rag_readonly', 'toolcalling_safe', 'toolcalling_auto')),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_by    VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(agent_id, version)
);

CREATE INDEX idx_prompts_active ON prompts(agent_id) WHERE is_active = true;
```

### Migration 009: MCP Servers

```sql
-- 009_mcp_servers.up.sql
CREATE TABLE mcp_servers (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label              VARCHAR(100) NOT NULL UNIQUE,
    endpoint           TEXT NOT NULL,
    auth_type          VARCHAR(20) NOT NULL DEFAULT 'none'
                       CHECK (auth_type IN ('none', 'bearer', 'basic')),
    auth_credential    TEXT NOT NULL DEFAULT '',
    health_endpoint    TEXT NOT NULL DEFAULT '',
    circuit_breaker    JSONB NOT NULL DEFAULT '{"fail_threshold": 5, "open_duration_s": 30}',
    discovery_interval VARCHAR(20) NOT NULL DEFAULT '5m',
    is_enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Migration 010: Trust Rules

```sql
-- 010_trust_rules.up.sql
CREATE TABLE trust_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    tool_pattern VARCHAR(100) NOT NULL,
    tier         VARCHAR(10) NOT NULL CHECK (tier IN ('auto', 'review', 'block')),
    created_by   VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(workspace_id, tool_pattern)
);

CREATE INDEX idx_trust_rules_ws ON trust_rules(workspace_id);
```

### Migration 011: Trust Defaults

```sql
-- 011_trust_defaults.up.sql
CREATE TABLE trust_defaults (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tier       VARCHAR(10) NOT NULL CHECK (tier IN ('auto', 'review', 'block')),
    patterns   JSONB NOT NULL DEFAULT '[]',
    priority   INT NOT NULL DEFAULT 100,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed: matches the current BFF classifier.go pattern lists
INSERT INTO trust_defaults (tier, patterns, priority) VALUES
    ('auto',   '["_read", "_list", "_search", "_get", "read_", "list_", "search_", "get_", "delegate_to_agent"]', 1),
    ('block',  '["_delete", "_send", "_force", "delete_", "send_", "force_"]', 2),
    ('review', '["_write", "_create", "_update", "_commit", "write_", "create_", "update_", "commit_"]', 3);
```

### Migration 012: Trigger Rules

```sql
-- 012_trigger_rules.up.sql
CREATE TABLE trigger_rules (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       UUID NOT NULL,
    name               TEXT NOT NULL,
    event_type         TEXT NOT NULL,
    condition          JSONB NOT NULL DEFAULT '{}',
    agent_id           VARCHAR(50) NOT NULL REFERENCES agents(id),
    prompt_template    TEXT NOT NULL DEFAULT '',
    enabled            BOOLEAN NOT NULL DEFAULT true,
    rate_limit_per_hour INT NOT NULL DEFAULT 10,
    schedule           VARCHAR(100) NOT NULL DEFAULT '',
    run_as_user_id     UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_trigger_rules_ws_event ON trigger_rules(workspace_id, event_type) WHERE enabled = true;
```

### Migration 013: Model Config

```sql
-- 013_model_config.up.sql
CREATE TABLE model_config (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope                     VARCHAR(20) NOT NULL CHECK (scope IN ('global', 'workspace', 'user')),
    scope_id                  VARCHAR(100) NOT NULL DEFAULT '',
    default_model             VARCHAR(100) NOT NULL DEFAULT 'qwen3:8b',
    temperature               NUMERIC(3,2) NOT NULL DEFAULT 0.70,
    max_tokens                INT NOT NULL DEFAULT 8192,
    max_tool_rounds           INT NOT NULL DEFAULT 10,
    default_context_window    INT NOT NULL DEFAULT 128000,
    default_max_output_tokens INT NOT NULL DEFAULT 8192,
    history_token_budget      INT NOT NULL DEFAULT 4000,
    max_history_messages      INT NOT NULL DEFAULT 20,
    embedding_model           VARCHAR(100) NOT NULL DEFAULT 'nomic-embed-text:latest',
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(scope, scope_id)
);

-- Seed global defaults
INSERT INTO model_config (scope, scope_id) VALUES ('global', '');
```

### Migration 014: Context Config

```sql
-- 014_context_config.up.sql
CREATE TABLE context_config (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope            VARCHAR(20) NOT NULL CHECK (scope IN ('global', 'workspace')),
    scope_id         VARCHAR(100) NOT NULL DEFAULT '',
    max_total_tokens INT NOT NULL DEFAULT 18000,
    layer_budgets    JSONB NOT NULL DEFAULT '{"workspace_structure": 500, "ui_context": 200, "semantic_retrieval": 4000, "file_content": 8000, "conversation_memory": 4000, "domain_state": 1000}',
    enabled_layers   JSONB NOT NULL DEFAULT '["workspace_structure", "ui_context", "semantic_retrieval", "file_content", "conversation_memory", "domain_state"]',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(scope, scope_id)
);

-- Seed global defaults
INSERT INTO context_config (scope, scope_id) VALUES ('global', '');
```

### Migration 015: Signal Config

```sql
-- 015_signal_config.up.sql
CREATE TABLE signal_config (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source        VARCHAR(50) NOT NULL UNIQUE,
    poll_interval VARCHAR(20) NOT NULL DEFAULT '15m',
    is_enabled    BOOLEAN NOT NULL DEFAULT true,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed defaults (from BFF signals/manager.go DefaultManagerConfig)
INSERT INTO signal_config (source, poll_interval) VALUES
    ('gmail',    '15m'),
    ('calendar', '1h'),
    ('drive',    '30m'),
    ('slack',    '30s');
```

### Migration 016: Webhook Subscriptions

```sql
-- 016_webhook_subscriptions.up.sql
CREATE TABLE webhook_subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url        TEXT NOT NULL,
    secret     TEXT NOT NULL DEFAULT '',
    events     JSONB NOT NULL DEFAULT '[]',
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Migration 017: Seed Agents

This migration is implemented as a Go-based seeder (not raw SQL) because agent system prompts are multi-paragraph text blocks. The server's `main.go` should call a `SeedAgents()` function on first boot that checks `SELECT count(*) FROM agents` and inserts the 16 product agents if the table is empty.

**Agents to seed:**

| ID | Name | Tools (count) | Mode |
|----|------|---------------|------|
| `router` | Router Agent | 2 (delegate_to_agent, get_gmail_message_content) | toolcalling_safe |
| `pmo` | PMO Agent | 24 (domain + git + slack) | toolcalling_safe |
| `raid_manager` | RAID Manager | 6 (domain + git) | toolcalling_safe |
| `task_manager` | Task Manager | 9 (domain + roster + git) | toolcalling_safe |
| `comms_manager` | Communications Manager | 15 (email + slack + roster + git) | toolcalling_safe |
| `meeting_manager` | Meeting Manager | 15 (domain + calendar + git) | toolcalling_safe |
| `engagement_pm` | Engagement PM | TBD | toolcalling_safe |
| `knowledge_steward` | Knowledge Steward | TBD | toolcalling_safe |
| `document_manager` | Document Manager | TBD | toolcalling_safe |
| `strategist` | Strategist | TBD | toolcalling_safe |
| `backlog_steward` | Backlog Steward | TBD | toolcalling_safe |
| `team_manager` | Team Manager | TBD | toolcalling_safe |
| `slack_manager` | Slack Manager | TBD | toolcalling_safe |
| `initiateproject` | Project Initializer | TBD | toolcalling_safe |
| `meeting_processor` | Meeting Processor | TBD | toolcalling_auto |
| `comms_lead` | Communications Lead | TBD | toolcalling_safe |

For the first 6 agents (router through meeting_manager), copy the exact tool lists, trust overrides, and system prompts from the existing BFF `registry.go`. For the remaining 10, create placeholder entries with the name and description from the product agent profiles — they can be filled in via the API later.

---

## 8. Security Hardening

This section codifies security requirements. The implementation MUST follow these.

### 8.1 Password Security

- **Hashing:** bcrypt with cost factor 12 (minimum). Use `golang.org/x/crypto/bcrypt`.
- **Policy:** Minimum 12 characters, at least 1 uppercase, 1 lowercase, 1 digit, 1 special character. Reject passwords found in the top 10,000 common passwords list (embedded).
- **Storage:** Only the bcrypt hash is stored. Plaintext is never logged or returned.
- **Comparison:** Always use constant-time comparison (`bcrypt.CompareHashAndPassword`).

### 8.2 Secret Encryption at Rest

- MCP `auth_credential` and OAuth `access_token`/`refresh_token` are encrypted using AES-256-GCM before storage.
- The encryption key (`CREDENTIAL_ENCRYPTION_KEY`) is a 32-byte key, base64-encoded in the environment.
- Each encrypted value uses a unique random 12-byte nonce, prepended to the ciphertext.
- Key rotation: if the key changes, a startup migration decrypts with old key and re-encrypts with new. (Future phase; for now, key change requires re-entering credentials.)

### 8.3 HTTP Security Headers

Every response includes:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'
Permissions-Policy: camera=(), microphone=(), geolocation=()
```

### 8.4 Rate Limiting

| Endpoint | Limit | Window | Scope |
|----------|-------|--------|-------|
| `POST /auth/login` | 5 attempts | per 15 minutes | per IP + per username |
| `POST /auth/google/*` | 10 requests | per 15 minutes | per IP |
| `POST /api/v1/*` (mutations) | 60 requests | per minute | per authenticated user/key |
| `GET /api/v1/*` (reads) | 300 requests | per minute | per authenticated user/key |
| `GET /api/v1/discovery` | 10 requests | per minute | per API key |

Rate limit state is stored in-memory (reset on restart). Response includes standard headers:
```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1708000000
Retry-After: 30  (only on 429)
```

### 8.5 Input Validation

- All string inputs are trimmed and length-limited at the handler level.
- Agent IDs: `/^[a-z][a-z0-9_]{1,49}$/` (lowercase, underscore, 2–50 chars).
- Tool patterns: max 100 chars, validated as safe glob patterns (no path traversal).
- URLs (endpoints, webhooks): must be valid HTTP/HTTPS URLs, max 2000 chars. Private IPs allowed (internal services) but logged.
- Cron expressions: validated via `robfig/cron/v3` parser.
- JSON fields (tools, conditions): max 1MB per field.
- System prompts: max 100KB per prompt.

### 8.6 SQL Injection Prevention

- All database queries use parameterized queries via `pgx`. No string concatenation for SQL.
- JSONB fields are passed as `json.RawMessage` parameters, never interpolated.

### 8.7 CORS

CORS is configured to allow only the registry's own origin (same-origin for the embedded GUI). For external access (BFF), API key auth bypasses CORS (server-to-server).

```go
cors.Options{
    AllowedOrigins: []string{},  // same-origin only for GUI
    AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
    AllowedHeaders: []string{"Authorization", "Content-Type", "X-CSRF-Token", "If-Match"},
    MaxAge:         3600,
}
```

### 8.8 Audit Logging

Every mutation (create, update, delete) and authentication event (login, logout, failed login, OAuth link, password change, auth reset) is logged to the `audit_log` table with:
- `actor`: username or API key name
- `action`: what happened
- `resource_type` + `resource_id`: what was affected
- `details`: JSON with changed fields (old and new values for updates; sensitive fields redacted)
- `ip_address`: client IP

The audit log is append-only. There is no API to delete audit entries.

### 8.9 Dependency Security

- Minimal dependencies; only well-maintained, widely-used Go packages.
- `go.sum` is committed for reproducible builds.
- Dockerfile uses specific image tags (not `latest`).
- Non-root user in container (UID 1001).
- Read-only root filesystem where possible.

---

## 9. Configuration and Environment

### 9.1 Config Struct

```go
type Config struct {
    // Database
    DatabaseURL string `env:"DATABASE_URL,required"`

    // Server
    Port       string `env:"PORT" default:"8090"`
    LogLevel   string `env:"LOG_LEVEL" default:"info"`
    ExternalURL string `env:"EXTERNAL_URL" default:"http://localhost:8090"` // for OAuth redirect

    // Auth
    SessionSecret          string `env:"SESSION_SECRET,required"`           // 32+ byte hex for session signing
    CredentialEncryptionKey string `env:"CREDENTIAL_ENCRYPTION_KEY,required"` // base64 32-byte AES key

    // Google OAuth (optional; if not set, Google login button is hidden)
    GoogleOAuthClientID     string `env:"GOOGLE_OAUTH_CLIENT_ID"`
    GoogleOAuthClientSecret string `env:"GOOGLE_OAUTH_CLIENT_SECRET"`

    // Telemetry
    OTELExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    OTELTracesEnabled    bool   `env:"OTEL_TRACES_ENABLED" default:"true"`
    OTELMetricsEnabled   bool   `env:"OTEL_METRICS_ENABLED" default:"true"`
    OTELLogsEnabled      bool   `env:"OTEL_LOGS_ENABLED" default:"true"`

    // Webhook delivery
    WebhookTimeoutS int `env:"WEBHOOK_TIMEOUT" default:"5"`
    WebhookRetries  int `env:"WEBHOOK_RETRIES" default:"3"`
    WebhookWorkers  int `env:"WEBHOOK_WORKERS" default:"4"`
}
```

### 9.2 Environment Variable Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `PORT` | No | `8090` | HTTP listen port |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |
| `EXTERNAL_URL` | No | `http://localhost:8090` | Public URL (used for OAuth redirect_uri) |
| `SESSION_SECRET` | Yes | — | 32+ byte hex string for session cookie signing |
| `CREDENTIAL_ENCRYPTION_KEY` | Yes | — | Base64-encoded 32-byte AES key for MCP/OAuth credentials |
| `GOOGLE_OAUTH_CLIENT_ID` | No | — | Google OAuth client ID (omit to hide Google login) |
| `GOOGLE_OAUTH_CLIENT_SECRET` | No | — | Google OAuth client secret |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | — | OTLP gRPC endpoint; empty disables telemetry export |
| `OTEL_SERVICE_NAME` | No | `agentic-registry` | OTel service name attribute |
| `OTEL_TRACES_ENABLED` | No | `true` | Enable trace export |
| `OTEL_METRICS_ENABLED` | No | `true` | Enable Prometheus metrics |
| `OTEL_LOGS_ENABLED` | No | `true` | Enable log export |
| `WEBHOOK_TIMEOUT` | No | `5` | Webhook delivery timeout (seconds) |
| `WEBHOOK_RETRIES` | No | `3` | Webhook delivery retry count |
| `WEBHOOK_WORKERS` | No | `4` | Concurrent webhook delivery goroutines |

### 9.3 Generating Secrets

```bash
# Session secret (32+ bytes, hex encoded)
openssl rand -hex 32

# Credential encryption key (must be exactly 32 bytes, base64 encoded)
openssl rand -base64 32

# Webhook signing secret
openssl rand -hex 32
```

### 9.4 Google OAuth Setup

1. Go to Google Cloud Console → APIs & Services → Credentials.
2. Create OAuth 2.0 Client ID (type: **Web Application**).
3. Set Authorized redirect URI: `{EXTERNAL_URL}/auth/google/callback` (e.g., `http://localhost:8090/auth/google/callback`).
4. Copy Client ID and Client Secret to `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET`.
5. If these env vars are not set, the "Sign in with Google" button is hidden in the login page.

---

## 10. Client Integration Guide

### 10.1 BFF Startup Flow

```
1. BFF starts
2. BFF calls GET /api/v1/discovery (Registry)
   → receives all agents, MCP servers, trust defaults, model config, context config, signal config
3. BFF hydrates in-memory caches:
   - AgentRegistry.Reload(agents)
   - MCPRouter reconfigured with server endpoints
   - TrustClassifier loaded with trust defaults
   - ContextAssembler budgets updated
   - SignalManager intervals updated
4. BFF registers webhook:
   POST /api/v1/webhooks → url: http://bff:8082/internal/registry-webhook
5. BFF enters ready state
```

### 10.2 Change Notification Flow

```
1. Admin updates agent "pmo" via PUT /api/v1/agents/pmo
2. Registry persists change, creates version snapshot
3. Registry dispatches webhook to all subscribers:
   POST http://bff:8082/internal/registry-webhook
   Body: { "event": "agent.updated", "resource_type": "agent", "resource_id": "pmo", ... }
   Header: X-Webhook-Signature: sha256=<HMAC>
4. BFF validates HMAC signature
5. BFF re-fetches: GET /api/v1/agents/pmo
6. BFF calls AgentRegistry.Reload() with updated agent list
7. Next run using "pmo" picks up new config
```

### 10.3 BFF Registry Client Interface

Add this to the BFF codebase as `internal/registry/client.go`:

```go
type RegistryClient interface {
    // Composite discovery (startup)
    FetchAll(ctx context.Context) (*DiscoveryResponse, error)

    // Agents
    GetAgent(ctx context.Context, agentID string) (*Agent, error)
    ListAgents(ctx context.Context) ([]Agent, error)

    // Prompts
    GetActivePrompt(ctx context.Context, agentID string) (*Prompt, error)

    // MCP Servers
    ListMCPServers(ctx context.Context) ([]MCPServer, error)

    // Trust
    GetTrustRules(ctx context.Context, workspaceID uuid.UUID) ([]TrustRule, error)
    GetTrustDefaults(ctx context.Context) ([]TrustDefault, error)

    // Trigger Rules
    GetTriggerRules(ctx context.Context, workspaceID uuid.UUID) ([]TriggerRule, error)

    // Model Config
    GetModelConfig(ctx context.Context, scope, scopeID string) (*ModelConfig, error)

    // Context Config
    GetContextConfig(ctx context.Context) (*ContextConfig, error)

    // Signal Config
    ListSignalConfig(ctx context.Context) ([]SignalConfig, error)
}
```

Implementation should include:
- HTTP client with base URL from `REGISTRY_URL` env var
- API key from `REGISTRY_API_KEY` env var
- Circuit breaker (5 failures, 30s open)
- Request timeout (10s default)

### 10.4 Frontend Integration

The frontend accesses registry data through the BFF proxy (avoids CORS complexity):

```
Frontend → GET /api/v1/agents → BFF proxies → GET /api/v1/agents (Registry)
```

The BFF already serves `GET /api/v1/agents` from its in-memory cache; no change is needed for the frontend. For admin configuration UIs (editing agents, prompts, trust rules), the BFF can expose proxy endpoints that forward mutations to the registry.

---

## 11. Observability

### 11.1 Structured Logging

- JSON format via `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})`
- Optional dual output to OTel log exporter (same pattern as BFF's `DualHandler`)
- Every request logged with: `request_id`, `method`, `path`, `status`, `duration_ms`
- Mutations additionally log: `resource_type`, `resource_id`, `actor`, `version`

### 11.2 OpenTelemetry Tracing

- Service name: `agentic-registry`
- HTTP instrumentation via `otelhttp.NewMiddleware("agentic-registry")`
- Key span names: `db.query`, `webhook.dispatch`, `api.agents.list`, `api.agents.create`, etc.
- Trace context propagation: W3C TraceContext + Baggage headers
- Exporter: gRPC OTLP to `OTEL_EXPORTER_OTLP_ENDPOINT`

### 11.3 Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `registry_http_requests_total` | Counter | `method`, `path`, `status` | HTTP request count |
| `registry_http_request_duration_seconds` | Histogram | `method`, `path` | Request latency |
| `registry_webhook_deliveries_total` | Counter | `event`, `status` | Webhook delivery count |
| `registry_webhook_delivery_duration_seconds` | Histogram | `event` | Webhook delivery latency |
| `registry_db_pool_connections` | Gauge | `state` | Connection pool (idle/active/total) |
| `registry_resource_mutations_total` | Counter | `resource_type`, `operation` | CRUD counts (create/update/delete) |
| `registry_auth_attempts_total` | Counter | `method`, `result` | Login attempts (password/google/apikey × success/failure/locked) |
| `registry_active_sessions` | Gauge | — | Current active session count |
| `registry_rate_limit_rejections_total` | Counter | `endpoint` | Requests rejected by rate limiter |

---

## 12. Deployment

### 12.1 Dockerfile

```dockerfile
# ── Stage 1: Build Admin GUI ────────────────────────
FROM node:20-alpine AS web-builder

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ .
RUN npm run build          # outputs to /web/dist/

# ── Stage 2: Build Go Server ────────────────────────
FROM golang:1.24-alpine AS go-builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Embed the pre-built GUI into the Go binary
COPY --from=web-builder /web/dist/ /build/web/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /registry ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /healthcheck ./cmd/healthcheck

# ── Stage 3: Runtime ────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1001 registry

COPY --from=go-builder /registry /registry
COPY --from=go-builder /healthcheck /healthcheck

USER 1001
EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/healthcheck"]
ENTRYPOINT ["/registry"]
```

### 12.2 Compose Service

Add to the existing Agent Smit `deployment/compose.yaml`:

```yaml
  agentic-registry:
    build:
      context: ../docker/agentic-registry
      dockerfile: Dockerfile
    container_name: agent-smit-agentic-registry
    ports:
      - "${REGISTRY_PORT:-8090}:8090"
    environment:
      DATABASE_URL: postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${REGISTRY_DB:-agentsmit_registry}?sslmode=disable
      PORT: "8090"
      LOG_LEVEL: ${LOG_LEVEL:-info}
      EXTERNAL_URL: ${REGISTRY_EXTERNAL_URL:-http://localhost:8090}
      SESSION_SECRET: ${REGISTRY_SESSION_SECRET:?set REGISTRY_SESSION_SECRET in .env}
      CREDENTIAL_ENCRYPTION_KEY: ${REGISTRY_CREDENTIAL_ENCRYPTION_KEY:?set REGISTRY_CREDENTIAL_ENCRYPTION_KEY in .env}
      GOOGLE_OAUTH_CLIENT_ID: ${REGISTRY_GOOGLE_OAUTH_CLIENT_ID:-}
      GOOGLE_OAUTH_CLIENT_SECRET: ${REGISTRY_GOOGLE_OAUTH_CLIENT_SECRET:-}
      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-}
      OTEL_SERVICE_NAME: agentic-registry
      OTEL_TRACES_ENABLED: ${OTEL_TRACES_ENABLED:-true}
      OTEL_METRICS_ENABLED: ${OTEL_METRICS_ENABLED:-true}
      OTEL_LOGS_ENABLED: ${OTEL_LOGS_ENABLED:-true}
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "/healthcheck"]
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 10s
    networks:
      - agent-smit-net
```

### 12.3 PostgreSQL Init

Add to `deployment/scripts/init_postgres.sh`:

```bash
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
    CREATE DATABASE agentsmit_registry;
    GRANT ALL PRIVILEGES ON DATABASE agentsmit_registry TO $POSTGRES_USER;
EOSQL
```

### 12.4 BFF Compose Changes

Add to BFF service environment in `deployment/compose.yaml`:

```yaml
  bff:
    environment:
      # ... existing vars ...
      REGISTRY_URL: ${REGISTRY_URL:-http://agentic-registry:8090}
      REGISTRY_API_KEY: ${REGISTRY_API_KEY}
    depends_on:
      # ... existing deps ...
      agentic-registry:
        condition: service_healthy
```

### 12.5 New .env Variables

Add to `deployment/.env.example`:

```bash
# ── Agentic Registry ──────────────────────────────────
REGISTRY_DB=agentsmit_registry
REGISTRY_PORT=8090
REGISTRY_EXTERNAL_URL=http://localhost:8090
REGISTRY_SESSION_SECRET=                      # generate with: openssl rand -hex 32
REGISTRY_CREDENTIAL_ENCRYPTION_KEY=           # generate with: openssl rand -base64 32
REGISTRY_GOOGLE_OAUTH_CLIENT_ID=              # optional: Google OAuth client ID
REGISTRY_GOOGLE_OAUTH_CLIENT_SECRET=          # optional: Google OAuth client secret
REGISTRY_API_KEY=                             # API key for BFF → Registry communication
REGISTRY_URL=http://agentic-registry:8090     # internal URL used by BFF
```

---

## 13. Implementation Roadmap

### Phase 1: Foundation + Authentication

**Slice 1.1 — Project skeleton, database, and health**
- Initialize Go module (`github.com/agent-smit/agentic-registry`)
- `cmd/server/main.go`: config loading, DB pool, migration runner, chi router, OTel init, graceful shutdown
- `cmd/healthcheck/main.go`: HTTP GET to `/healthz`
- `internal/config/config.go`: env var loading
- `internal/db/pool.go` + `internal/db/migrate.go`
- `internal/errors/errors.go`: APIError type with `NotFound()`, `Conflict()`, `Validation()`, `Forbidden()`, `Locked()` constructors
- `internal/api/router.go`, `respond.go`, `health.go`
- `internal/telemetry/telemetry.go`
- Migrations 001 (extensions)
- Dockerfile (3-stage: Node → Go → alpine)
- Tests for health endpoints

**Slice 1.2 — User authentication (password + sessions)**
- Migrations 002 (users), 004 (sessions), 005 (api_keys), 006 (audit_log)
- `internal/auth/password.go`: bcrypt hash, verify, password policy validator
- `internal/auth/session.go`: PostgreSQL session store (create, get, delete, cleanup goroutine)
- `internal/auth/csrf.go`: double-submit cookie generation and validation
- `internal/auth/handler.go`: `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `POST /auth/change-password`
- `internal/auth/apikey.go`: API key validation middleware
- `internal/api/middleware.go`: unified auth middleware (session OR API key), request ID, logging, recovery, rate limiter, security headers
- `internal/ratelimit/limiter.go`: in-memory rate limiter with per-IP and per-user buckets
- `internal/store/users.go`, `internal/store/sessions.go`, `internal/store/api_keys.go`, `internal/store/audit.go`
- Default admin account seeder (admin/admin, must_change_pass=true)
- Tests for login, logout, session, CSRF, rate limiting, password policy

**Slice 1.3 — Google OAuth + account linking**
- Migration 003 (oauth_connections)
- `internal/auth/oauth.go`: Google OAuth2 PKCE flow, token exchange, account linking logic
- `internal/auth/handler.go`: add `GET /auth/google/start`, `GET /auth/google/callback`, `POST /auth/unlink-google`
- `internal/api/users.go`: `POST /api/v1/users/{userId}/reset-auth`, user CRUD for admins
- `internal/api/api_keys.go`: API key management endpoints (create, list, revoke)
- CLI `--reset-admin` flag in `main.go`
- Tests for OAuth flow, account linking, auth reset, API key CRUD

### Phase 2: Resource CRUD

**Slice 2.1 — Agent CRUD**
- Migration 007 (agents + agent_versions)
- `internal/store/agents.go`
- `internal/api/agents.go`: all 8 endpoints (list, get, create, put, patch, delete, versions, rollback)
- Role-based access: viewer=read, editor=write, admin=all
- Audit log integration (every mutation logged)
- Tests for all agent endpoints + authorization

**Slice 2.2 — Prompt CRUD with versioning**
- Migration 008 (prompts)
- `internal/store/prompts.go`
- `internal/api/prompts.go`: all 6 endpoints (list, get active, get by ID, create, activate, rollback)
- Tests for prompt creation, activation, deactivation, rollback

**Slice 2.3 — MCP Server configuration**
- Migration 009 (mcp_servers)
- `internal/store/mcp_servers.go`
- `internal/api/mcp_servers.go`
- AES-256-GCM encryption for `auth_credential`
- Tests

**Slice 2.4 — Trust rules and trust defaults**
- Migrations 010 (trust_rules) and 011 (trust_defaults)
- `internal/store/trust_rules.go` + `internal/store/trust_defaults.go`
- `internal/api/trust_rules.go` + `internal/api/trust_defaults.go`
- Tests

**Slice 2.5 — Trigger rules**
- Migration 012 (trigger_rules)
- `internal/store/trigger_rules.go`
- `internal/api/trigger_rules.go`
- Cron expression validation (using `robfig/cron/v3`)
- Event type validation against well-known set
- Tests

### Phase 3: Global Config + Webhooks

**Slice 3.1 — Model, context, and signal configuration**
- Migrations 013 (model_config), 014 (context_config), 015 (signal_config)
- Stores and API handlers for all three
- Scope inheritance for model config (global → workspace → user)
- Tests

**Slice 3.2 — Webhook subscriptions and notification dispatcher**
- Migration 016 (webhook_subscriptions)
- `internal/store/webhooks.go`
- `internal/notify/dispatcher.go`: async worker pool, HMAC-SHA256 signing, exponential backoff retry, delivery metrics
- `internal/api/webhooks.go`
- Wire dispatcher into all mutation handlers
- Tests

**Slice 3.3 — Discovery endpoint and agent seeder**
- `internal/api/discovery.go`: composite endpoint
- Migration 017 + Go seeder: populate 16 product agents + default config on first boot
- Performance validation: discovery < 100ms
- Tests

### Phase 4: Admin GUI

**Slice 4.1 — GUI skeleton + login**
- Initialize `web/` project (React 18 + PatternFly 5 + Vite + TypeScript)
- `web/src/api/client.ts`: fetch wrapper with CSRF token and cookie handling
- `web/src/auth/`: LoginPage, AuthContext, ProtectedRoute, force-password-change flow
- `web/src/components/AppLayout.tsx`: PatternFly Page shell with sidebar nav
- `web/src/pages/DashboardPage.tsx`: system overview cards
- Go server: embed `web/dist/` via `embed.FS`, serve SPA at `/`, fallback to `index.html`
- Conditional "Sign in with Google" button (hidden if no OAuth config)
- Tests for login flow

**Slice 4.2 — Core artifact pages**
- AgentsPage + AgentDetailPage (with tabs: general, tools, trust overrides, system prompt, prompt versions, version history)
- PromptsPage (version browser, diff viewer)
- MCPServersPage (CRUD + health indicator + test connection)
- DiffViewer component (side-by-side red/green diff)
- VersionTimeline component
- JsonEditor component (for tools array and conditions)
- Tests for key interactions

**Slice 4.3 — Configuration + system pages**
- TrustPage (defaults + workspace rules tabs)
- TriggersPage
- ModelConfigPage, ContextConfigPage, SignalsPage
- WebhooksPage (CRUD + test ping)
- APIKeysPage (create with one-time display, revoke)
- UsersPage (admin: list, create, edit role, reset auth)
- AuditLogPage (searchable, filterable, paginated)
- MyAccountPage (change password, Google link/unlink, personal API keys, active sessions)

### Phase 5: Polish + Integration

**Slice 5.1 — End-to-end validation**
- Compose integration: add registry service to `deployment/compose.yaml`
- Verify startup sequence: postgres → registry (migrations + seed) → BFF (discovery call)
- Verify webhook delivery: mutate agent → BFF receives notification
- Verify admin GUI: login → browse agents → edit prompt → see version history → rollback
- Verify OAuth: link Google → login via Google → unlink → password login works

**Slice 5.2 — Security audit**
- Verify all OWASP Top 10 mitigations are in place
- Verify rate limiting works under load
- Verify session expiry and idle timeout
- Verify CSRF protection on all mutation endpoints
- Verify audit log completeness
- Verify password policy enforcement
- Verify no secrets in API responses
- Penetration test: SQL injection, XSS, CSRF, auth bypass

---

## Appendix A: Webhook Event Reference

| Event | When |
|-------|------|
| `agent.created` | New agent inserted |
| `agent.updated` | Agent definition changed (PUT or PATCH; new version created) |
| `agent.deleted` | Agent soft-deleted (is_active = false) |
| `agent.rolled_back` | Agent rolled back to previous version |
| `prompt.created` | New prompt version inserted |
| `prompt.activated` | Specific prompt version activated |
| `prompt.rolled_back` | Prompt rolled back to previous version |
| `mcp_server.created` | New MCP server added |
| `mcp_server.updated` | MCP server config changed |
| `mcp_server.deleted` | MCP server removed |
| `trust_rule.changed` | Trust rule created, updated, or deleted |
| `trust_default.changed` | Default trust pattern updated |
| `trigger_rule.changed` | Trigger rule created, updated, or deleted |
| `model_config.updated` | Model configuration changed (any scope) |
| `context_config.updated` | Context assembly config changed |
| `signal_config.updated` | Signal polling config changed |

## Appendix B: Authentication, Authorization, and API Key Reference

### B.1 Authentication Methods

| Method | Transport | Use Case | Details |
|--------|-----------|----------|---------|
| **Password** | `POST /auth/login` → session cookie | Human users (admin GUI) | bcrypt cost 12; brute-force protection (5 attempts → 15 min lockout, doubling) |
| **Google OAuth 2.0** | `GET /auth/google/start` → redirect flow → session cookie | Human users (admin GUI) | PKCE; auto-links by email match; once linked, password login disabled for that user |
| **API Key** | `Authorization: Bearer rk_live_…` header | Service-to-service (BFF → Registry) | SHA-256 hashed at rest; `rk_live_` prefix; no session/cookie needed |

### B.2 Role-Based Access Control (RBAC)

| Role | Read Resources | Write Resources | User Management | System Config |
|------|---------------|----------------|-----------------|---------------|
| `viewer` | All GET endpoints | — | — | — |
| `editor` | All GET endpoints | POST, PUT, PATCH, DELETE on agents, prompts, MCP servers, trust, triggers | — | — |
| `admin` | All GET endpoints | All mutations | Create/edit/deactivate users, reset auth, manage API keys | Webhooks, model config, context config, signal config |

### B.3 API Key Scopes

| Scope | Allows | Typical Consumer |
|-------|--------|-----------------|
| `read` | All GET endpoints | Monitoring, dashboards |
| `write` | GET + POST + PUT + PATCH + DELETE on resources | BFF service account |
| `admin` | All above + user management + webhook config + API key management | Automation / CI pipelines |

### B.4 First-Boot Flow

1. Server runs migrations (creates all tables).
2. Checks if any user with role `admin` exists.
3. If not, inserts default admin: `username=admin`, `password=admin` (bcrypt), `role=admin`, `must_change_pass=true`.
4. On first login, admin is forced to change password before accessing the GUI.
5. Admin can then create additional users, generate API keys, and configure Google OAuth.

### B.5 Auth Reset Mechanisms

| Mechanism | When to Use | How |
|-----------|------------|-----|
| **Admin resets another user** | User locked out of OAuth | `POST /api/v1/users/{userId}/reset-auth` — unlinks OAuth, resets password to temporary, sets `must_change_pass=true` |
| **CLI emergency reset** | Admin account itself is locked out | `./registry --reset-admin` — resets admin user to `admin/admin`, clears OAuth links, sets `must_change_pass=true` |

### B.6 Session Lifecycle

| Parameter | Value |
|-----------|-------|
| Cookie name | `__Host-session` |
| Max lifetime | 8 hours |
| Idle timeout | 30 minutes (sliding) |
| Storage | PostgreSQL `sessions` table |
| Cleanup | Background goroutine every 10 minutes deletes expired rows |
| CSRF | Double-submit cookie (`__Host-csrf`) validated on all non-GET requests for session-authenticated users |

## Appendix C: Dependencies

### C.1 Go Module Dependencies

```
# ── HTTP & Routing ────────────────────────────────────
github.com/go-chi/chi/v5                  # HTTP router (matches BFF)

# ── Database ──────────────────────────────────────────
github.com/jackc/pgx/v5                   # PostgreSQL driver (matches BFF)
github.com/golang-migrate/migrate/v4      # Database migrations (matches BFF)

# ── Auth & Security ───────────────────────────────────
golang.org/x/crypto                       # bcrypt password hashing (golang.org/x/crypto/bcrypt)
golang.org/x/oauth2                       # Google OAuth 2.0 client (golang.org/x/oauth2/google)
crypto/aes, crypto/cipher                 # AES-256-GCM encryption (stdlib — no extra dep)
crypto/rand, crypto/sha256, crypto/hmac   # Key generation, API key hashing, webhook signing (stdlib)
encoding/base64, encoding/hex             # Token encoding (stdlib)

# ── Utilities ─────────────────────────────────────────
github.com/google/uuid                    # UUID generation (matches BFF)
github.com/robfig/cron/v3                 # Cron expression validation (matches BFF)

# ── Observability ─────────────────────────────────────
go.opentelemetry.io/otel                  # OTel API (matches BFF)
go.opentelemetry.io/otel/sdk              # OTel SDK (matches BFF)
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc  # OTLP trace exporter
go.opentelemetry.io/otel/exporters/prometheus                     # Prometheus metrics exporter
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp     # HTTP middleware (matches BFF)
github.com/prometheus/client_golang       # Prometheus metrics (matches BFF)
```

**Design constraint:** Do not introduce additional web frameworks (no gin, no echo, no gorm, no gorilla). The registry follows the same minimal-dependency philosophy as the BFF: chi for routing, pgx for database, stdlib for crypto.

### C.2 Admin GUI Dependencies (web/)

```json
{
  "dependencies": {
    "@patternfly/patternfly": "^5.0.0",
    "@patternfly/react-core": "^5.0.0",
    "@patternfly/react-table": "^5.0.0",
    "react": "^18.3.0",
    "react-dom": "^18.3.0",
    "react-router-dom": "^6.26.0"
  },
  "devDependencies": {
    "@vitejs/plugin-react": "^4.0.0",
    "@types/react": "^18.3.0",
    "@types/react-dom": "^18.3.0",
    "typescript": "~5.6.0",
    "vite": "^5.4.0",
    "vitest": "^2.1.0",
    "@testing-library/react": "^16.0.0",
    "@testing-library/jest-dom": "^6.0.0"
  }
}
```

These match the Agent Smit frontend's dependency set. The admin GUI is intentionally a lightweight SPA — no state management library (React context is sufficient), no chart library (unless dashboard metrics are added later).
