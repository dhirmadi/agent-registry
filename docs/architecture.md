# Architecture

> How the Agentic Registry is designed, why it works this way, and how the pieces fit together.

---

## Design Philosophy

The Agentic Registry follows three guiding principles:

1. **Single binary, zero external dependencies at runtime.** The Go server, admin GUI, database migrations, and healthcheck are all compiled into one container image. Drop it next to PostgreSQL and you're running.

2. **Explicit over magic.** No ORM, no code generation, no reflection-based routing. Every SQL query is visible in `internal/store/`, every route is declared in `router.go`, every middleware is applied explicitly.

3. **Security is structural, not bolted on.** Encryption, authentication, audit logging, CSRF protection, and rate limiting are wired into the architecture — not optional middleware you might forget to apply.

---

## System Overview

```
┌──────────────────────────────────────────────────────────────┐
│                    Agentic Registry                          │
│                                                              │
│  ┌─────────────────────┐    ┌──────────────────────────────┐ │
│  │  Admin GUI           │    │  REST API                    │ │
│  │  React + PatternFly  │    │  /api/v1/*                   │ │
│  │  (embedded via Go    │    │  /auth/*                     │ │
│  │   embed.FS)          │    │  /healthz, /readyz           │ │
│  └──────────┬──────────┘    └──────────┬───────────────────┘ │
│             │ fetch /api/v1/*           │                     │
│             └──────────┬───────────────┘                     │
│                        │                                     │
│  ┌─────────────────────▼───────────────────────────────────┐ │
│  │  Go Server (chi/v5 + pgx/v5)                            │ │
│  │                                                          │ │
│  │  Middleware:  RequestID → RealIP → Logger → Recoverer   │ │
│  │              → CORS → SecurityHeaders → RateLimit       │ │
│  │              → Auth (session | API key) → CSRF          │ │
│  │                                                          │ │
│  │  Handlers ──→ Store ──→ pgx queries                     │ │
│  │       │                                                  │ │
│  │       ├──→ Audit Log (every mutation)                   │ │
│  │       └──→ Webhook Dispatcher (async, HMAC-signed)      │ │
│  └─────────────────────┬───────────────────────────────────┘ │
└────────────────────────┼─────────────────────────────────────┘
                         │ pgx/v5
                         ▼
                  ┌─────────────┐
                  │ PostgreSQL  │
                  │ 16          │
                  └─────────────┘
```

---

## Layered Architecture

Every request flows through the same three layers. No shortcuts allowed.

### Layer 1: Handlers (`internal/api/`)

HTTP handlers parse requests, validate input, enforce RBAC, call the store, record audit entries, dispatch webhooks, and write JSON responses. Each resource gets its own file:

| File | Responsibility |
|------|---------------|
| `agents.go` | Agent CRUD, versioning, rollback |
| `prompts.go` | Prompt CRUD, activation, rollback |
| `mcp_servers.go` | MCP server configuration CRUD |
| `trust_rules.go` | Workspace-scoped trust rule CRUD |
| `trust_defaults.go` | System-wide trust classification defaults |
| `model_config.go` | Global and workspace model parameters (legacy) |
| `model_endpoints.go` | Model endpoint CRUD, versioning, activation, rollback |
| `webhooks.go` | Webhook subscription management |
| `users.go` | User administration |
| `api_keys.go` | API key lifecycle |
| `audit.go` | Audit log queries (read-only) |
| `discovery.go` | Composite discovery endpoint |

Handlers never import `pgx` directly. They receive store interfaces at construction time, making them fully testable with mock stores.

### Layer 2: Store (`internal/store/`)

Each store file contains raw SQL queries executed via `pgx/v5`. No query builders, no ORM, no string concatenation — every query uses `$1`-style parameterized placeholders.

Key patterns:
- **Optimistic concurrency** — Updates include `WHERE updated_at = $N` and return `Conflict()` on mismatch
- **Transactional writes** — Multi-row mutations (prompt activation, agent rollback) use `pgx.BeginTx`
- **Explicit scanning** — Results are scanned into Go structs field by field

### Layer 3: Database (PostgreSQL 16)

Migrations are embedded in the binary via `//go:embed` and run automatically on startup using `golang-migrate/v4`. The schema includes:

| Table | Purpose |
|-------|---------|
| `users` | User accounts with bcrypt password hashes |
| `oauth_connections` | Google OAuth link records |
| `sessions` | Server-side session store |
| `api_keys` | SHA-256 hashed API keys |
| `audit_log` | Append-only mutation history |
| `agents` | Agent definitions (current state) |
| `agent_versions` | Immutable version snapshots for rollback |
| `prompts` | Versioned system prompts per agent |
| `mcp_servers` | MCP server configurations (credentials AES-256-GCM encrypted) |
| `trust_rules` | Workspace-scoped tool trust overrides |
| `trust_defaults` | System-wide trust classification patterns |
| `model_config` | LLM parameters — global and workspace-scoped (legacy) |
| `model_endpoints` | Versioned model provider endpoints with slug-based addressing |
| `model_endpoint_versions` | Immutable config snapshots per model endpoint (activation/rollback) |
| `webhook_subscriptions` | Webhook consumer registrations |

---

## Cross-Cutting Concerns

### Authentication

Three methods coexist — password, Google OAuth 2.0 (PKCE), and API keys. All session-based auth uses `__Host-session` cookies (HttpOnly, Secure, SameSite=Lax). API keys use `Authorization: Bearer rk_live_...` headers. See the [Authentication Guide](authentication.md) for details.

### Authorization (RBAC)

Three roles with strictly increasing permissions:

| Role | Read | Write | Manage Users | Manage Keys |
|------|------|-------|-------------|-------------|
| `viewer` | All resources | — | — | Own keys |
| `editor` | All resources | All resources | — | Own keys |
| `admin` | All resources | All resources | Yes | All keys |

Role checks happen in handler middleware via `RequireRole("editor", "admin")`.

### Audit Logging

Every mutation — create, update, delete, login, logout, password change — writes to the `audit_log` table with actor, action, resource type, resource ID, and IP address. The audit log is append-only with no delete API. Tests verify audit coverage via `audit_completeness_test.go`.

### Webhook Notifications

All mutations dispatch async notifications to registered webhook subscribers. The dispatcher uses:

- **Worker pool** — Configurable concurrency (default 4 goroutines)
- **HMAC-SHA256 signing** — Each delivery includes a `X-Webhook-Signature` header
- **Automatic retry** — Failed deliveries retry with backoff (configurable attempts)
- **Event filtering** — Subscribers choose which event types they receive

### Rate Limiting

| Scope | Limit | Window |
|-------|-------|--------|
| Login | 5 attempts | 15 min per IP |
| Google OAuth | 10 requests | 15 min per IP |
| API mutations | 60 requests | 1 min per user |
| API reads | 300 requests | 1 min per user |
| Discovery | 10 requests | 1 min per user |

Rate limit headers (`X-RateLimit-Limit`, `X-RateLimit-Remaining`, `Retry-After`) are included on all rate-limited responses.

### Optimistic Concurrency

Resources include an `updated_at` timestamp that serves as an ETag. Clients must send `If-Match` on update requests. If the resource was modified since the client last read it, the server returns `409 Conflict`. This prevents lost updates from concurrent editors.

---

## Embedded SPA

The React admin GUI is built at Docker image build time, then embedded into the Go binary via `//go:embed web/dist/*`. The Go server serves it at `/` with a catch-all fallback to `index.html` for client-side routing.

Route priority:
1. `/healthz`, `/readyz` — Health checks
2. `/auth/*` — Authentication endpoints
3. `/api/v1/*` — REST API
4. `/*` — SPA fallback (serves static assets or `index.html`)

---

## Project Layout

```
agentic-registry/
├── cmd/
│   ├── server/main.go           # Entrypoint: config, DB, router, seeder, graceful shutdown
│   └── healthcheck/main.go      # Container HEALTHCHECK binary
├── internal/
│   ├── api/                     # HTTP handlers + middleware
│   ├── auth/                    # Session, password, OAuth, API key, CSRF modules
│   ├── config/                  # Environment variable loading
│   ├── db/                      # pgxpool wrapper + migration runner
│   ├── errors/                  # APIError type + constructors
│   ├── notify/                  # Async webhook dispatcher
│   ├── ratelimit/               # Sliding-window rate limiter
│   ├── seed/                    # Agent seeder (16 product agents on first boot)
│   ├── store/                   # Database operations (one file per resource)
│   └── telemetry/               # OpenTelemetry initialization
├── web/                         # React + PatternFly 5 admin GUI
│   └── src/
│       ├── auth/                # Login, auth context, protected routes
│       ├── pages/               # Dashboard, agents, prompts, MCP, trust, etc.
│       └── components/          # AppLayout, DiffViewer, JsonEditor, etc.
├── migrations/                  # SQL migrations (embedded in binary)
├── deployment/                  # Docker Compose + env examples
├── docs/                        # Project documentation
├── Dockerfile                   # Multi-stage: Node → Go → Alpine
├── go.mod
└── go.sum
```

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Go + chi/v5 | Matches the consuming BFF stack; shared patterns reduce cognitive load |
| PostgreSQL + pgx/v5 | Same driver and migration tooling as the BFF; proven at scale |
| No ORM | Explicit SQL is easier to audit, optimize, and debug |
| Embedded SPA | Single binary deployment; no Nginx/CDN required |
| Three auth methods | Password for bootstrap, OAuth for production SSO, API keys for machines |
| bcrypt cost 12 | Industry standard; sufficient for offline attack resistance |
| AES-256-GCM for secrets | Authenticated encryption; unique nonce per operation |
| Webhook push (not polling) | Real-time cache invalidation; consumers don't need a polling loop |
| Optimistic concurrency | Prevents lost updates without pessimistic locks |
| Append-only audit log | Tamper-evident history; no delete API |

---

## Technology Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Language | Go 1.25 | Performance, single binary, strong stdlib |
| Router | chi/v5 | Lightweight, stdlib-compatible, middleware chains |
| Database | PostgreSQL 16 | JSONB for flexible fields, proven reliability |
| DB driver | pgx/v5 | Native Go, connection pooling, prepared statements |
| Migrations | golang-migrate/v4 | Embedded SQL, automatic on startup |
| Passwords | bcrypt (cost 12) | Industry standard, constant-time comparison |
| Encryption | AES-256-GCM (stdlib) | Authenticated encryption, no external crypto libs |
| API keys | SHA-256 hashed | Fast lookup, irreversible storage |
| Frontend | React 18 + PatternFly 5 | Enterprise UI kit, accessible, themeable |
| Build | Vite + TypeScript | Fast HMR, type safety |
| Tests (Go) | stdlib `testing` | No third-party test frameworks |
| Tests (Web) | Vitest + Testing Library | Fast, React-focused |
| Container | Multi-stage Docker | Node build → Go build → Alpine runtime |
