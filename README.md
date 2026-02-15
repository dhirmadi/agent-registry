<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24" />
  <img src="https://img.shields.io/badge/PostgreSQL-16-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL 16" />
  <img src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=black" alt="React 18" />
  <img src="https://img.shields.io/badge/PatternFly-5-004080?style=flat-square&logo=redhat&logoColor=white" alt="PatternFly 5" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="MIT License" />
  <img src="https://img.shields.io/badge/Security-67%2F67%20OWASP-brightgreen?style=flat-square&logo=owasp&logoColor=white" alt="Security 67/67" />
</p>

<h1 align="center">Agentic Registry</h1>

<p align="center">
  <strong>The single source of truth for AI agent configuration.</strong>
  <br />
  Manage agents, prompts, MCP servers, trust rules, model parameters, and webhooks — all from one service with a built-in admin GUI.
  <br />
  <br />
  <a href="docs/architecture.md">Architecture</a> &nbsp;&middot;&nbsp;
  <a href="docs/api-reference.md">API Reference</a> &nbsp;&middot;&nbsp;
  <a href="docs/deployment.md">Deploy</a> &nbsp;&middot;&nbsp;
  <a href="docs/development.md">Develop</a> &nbsp;&middot;&nbsp;
  <a href="docs/roadmap/README.md">Roadmap</a>
</p>

---

## Why Agentic Registry?

AI agent platforms grow fast. Suddenly you have agent definitions, system prompts, MCP server endpoints, trust policies, model parameters, and API keys scattered across environment variables, config files, and hardcoded constants. Changing a prompt requires a container rebuild. Adding an MCP server means a redeployment.

**Agentic Registry fixes this.** One microservice. One database. One admin GUI. Every agent artifact versioned, audited, and served via REST API — with webhook notifications for real-time cache invalidation.

### What It Manages

| Artifact | Capabilities |
|----------|-------------|
| **Agents** | Full CRUD with automatic versioning and one-click rollback |
| **Prompts** | Version history, activation, rollback, side-by-side diff |
| **MCP Servers** | Endpoint configuration with AES-256-GCM encrypted credentials |
| **Trust Rules** | Workspace-scoped tool trust classification (trusted / cautious / untrusted) |
| **Trust Defaults** | System-wide default trust patterns with priority ordering |
| **Model Config** | LLM parameters (model, temperature, token limits) with global/workspace inheritance |
| **Webhooks** | Push notifications with HMAC-SHA256 signing and automatic retry |
| **Users & API Keys** | Role-based access with three auth methods |

---

## Quick Start

```bash
# Clone
git clone https://github.com/agent-smit/agentic-registry.git
cd agentic-registry

# Configure
cp deployment/.env.example deployment/.env
# Set SESSION_SECRET and CREDENTIAL_ENCRYPTION_KEY in deployment/.env

# Run
docker compose -f deployment/compose.yaml up -d

# Verify
curl http://localhost:8090/healthz
# → {"status":"ok"}
```

Open `http://localhost:8090` in your browser. Login with `admin` / `admin` and set a new password.

---

## Architecture at a Glance

```
                    ┌──────────────────────────────┐
                    │      Agentic Registry        │
                    │                              │
   Browser ────────▶│  React + PatternFly GUI      │
                    │         │                     │
                    │         ▼                     │
   BFF / Services ─▶│  REST API (chi/v5)           │──── Webhook Push
                    │    │          │               │     (HMAC-SHA256)
                    │    ▼          ▼               │
                    │  Auth    Store (pgx/v5)       │
                    │  (3 methods)  │               │
                    └──────────────┼───────────────┘
                                   │
                                   ▼
                            ┌─────────────┐
                            │ PostgreSQL  │
                            │ 16          │
                            └─────────────┘
```

**Single binary.** The Go server, React admin GUI, database migrations, and healthcheck are compiled into one container image. No Nginx, no CDN, no sidecar.

**Three layers, no shortcuts.** Every request flows through Handler → Store → Database. Handlers never touch pgx directly. Store functions never write HTTP responses.

**Three auth methods.** Password for bootstrap. Google OAuth for production SSO. API keys for service-to-service.

> [Full architecture documentation](docs/architecture.md)

---

## Features

### Agent Lifecycle Management

- **Create, read, update, delete** agents with a clean REST API
- **Automatic versioning** — every update creates an immutable version snapshot
- **One-click rollback** — restore any previous version from the GUI or API
- **Regex-validated IDs** — `^[a-z][a-z0-9_]{1,49}$` for clean, URL-safe identifiers
- **16 product agents seeded** on first boot (6 with full tool definitions)

### Prompt Engineering Workflow

- **Versioned prompts** per agent with activation and rollback
- **Side-by-side diff viewer** in the admin GUI
- **Template variables** for parameterized prompts
- **Transactional activation** — previous prompt deactivated atomically

### Security-First Design

- **67/67 OWASP security checklist items verified** ([security checklist](docs/security-checklist.md))
- **bcrypt cost 12** for password hashing with constant-time comparison
- **AES-256-GCM** encryption for MCP server credentials at rest
- **SHA-256 hashed API keys** — plaintext shown once at creation, never stored
- **CSRF double-submit cookie** with constant-time validation
- **Brute-force protection** — escalating lockout (15 min → 24 hours)
- **Security headers** on every response (HSTS, CSP, X-Frame-Options, etc.)
- **Rate limiting** on login, OAuth, API mutations, reads, and discovery
- **Append-only audit log** — every mutation recorded with actor and IP

### Built-In Admin GUI

A full React + PatternFly 5 admin interface embedded in the Go binary:

- **Dashboard** with agent counts and system overview
- **Agent management** — create, edit, version history, rollback
- **Prompt editor** — version browser with diff view
- **MCP server configuration** — register, edit, monitor
- **Trust management** — defaults and workspace-scoped rules
- **Model configuration** — global and workspace parameters
- **Webhook management** — create subscriptions, view events
- **User administration** — create, edit roles, reset auth
- **API key management** — create, revoke, scope control
- **Audit log viewer** — searchable, paginated history
- **My Account** — password change, Google link/unlink, personal API keys

### Webhook Push Notifications

- **HMAC-SHA256 signed** deliveries with `X-Webhook-Signature` header
- **Event filtering** — subscribers choose which events to receive
- **Automatic retry** with configurable attempts and backoff
- **Worker pool** — configurable concurrent delivery goroutines

### Composite Discovery Endpoint

```http
GET /api/v1/discovery
Authorization: Bearer rk_live_...
```

Returns agents, MCP servers, trust defaults, and model config in a single response. Designed for consumers (like a BFF) to hydrate their cache on startup.

---

## Tech Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Language | **Go 1.24** | Performance, single binary, strong stdlib |
| Router | **chi/v5** | Lightweight, stdlib-compatible, composable middleware |
| Database | **PostgreSQL 16** | JSONB for flexible fields, proven reliability |
| DB Driver | **pgx/v5** | Native Go, connection pooling, prepared statements |
| Migrations | **golang-migrate/v4** | Embedded SQL, automatic on startup |
| Auth | **bcrypt + OAuth 2.0 PKCE + SHA-256 API keys** | Three methods for three use cases |
| Encryption | **AES-256-GCM (stdlib)** | Authenticated encryption, no external crypto libs |
| Frontend | **React 18 + PatternFly 5** | Enterprise UI kit, accessible, themeable |
| Build | **Vite + TypeScript** | Fast builds, type safety |
| Tests (Go) | **stdlib `testing`** | No third-party test frameworks |
| Tests (Web) | **Vitest + Testing Library** | Fast, React-focused |
| Container | **Multi-stage Docker** | Node build → Go build → Alpine runtime |

---

## API Overview

All endpoints return a consistent envelope: `{ success, data, error, meta }`.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/agents` | List agents |
| `POST` | `/api/v1/agents` | Create agent |
| `GET` | `/api/v1/agents/{id}` | Get agent |
| `PUT` | `/api/v1/agents/{id}` | Update agent (versioned) |
| `PATCH` | `/api/v1/agents/{id}` | Partial update |
| `DELETE` | `/api/v1/agents/{id}` | Delete agent |
| `POST` | `/api/v1/agents/{id}/rollback` | Rollback to version |
| `GET` | `/api/v1/agents/{id}/prompts` | List prompts |
| `POST` | `/api/v1/agents/{id}/prompts` | Create prompt |
| `POST` | `/api/v1/agents/{id}/prompts/{pid}/activate` | Activate prompt |
| `GET` | `/api/v1/mcp-servers` | List MCP servers |
| `POST` | `/api/v1/mcp-servers` | Register MCP server |
| `GET` | `/api/v1/discovery` | Composite discovery |
| `GET` | `/api/v1/webhooks` | List webhooks |
| `POST` | `/api/v1/webhooks` | Subscribe to events |
| `GET` | `/api/v1/audit-log` | Query audit trail |

> [Full API reference with request/response examples](docs/api-reference.md)

---

## Configuration

All configuration via environment variables. No config files.

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `SESSION_SECRET` | Yes | 64-char hex for session signing |
| `CREDENTIAL_ENCRYPTION_KEY` | Yes | 32-byte base64 for AES-256-GCM |
| `PORT` | No | HTTP port (default: `8090`) |
| `LOG_LEVEL` | No | `debug` / `info` / `warn` / `error` |
| `EXTERNAL_URL` | No | Public URL for OAuth redirects |
| `GOOGLE_OAUTH_CLIENT_ID` | No | Google OAuth client ID |
| `GOOGLE_OAUTH_CLIENT_SECRET` | No | Google OAuth secret |
| `WEBHOOK_TIMEOUT` | No | Delivery timeout in seconds (default: `5`) |
| `WEBHOOK_RETRIES` | No | Retry attempts (default: `3`) |
| `WEBHOOK_WORKERS` | No | Concurrent delivery goroutines (default: `4`) |

> [Full deployment guide](docs/deployment.md)

---

## Testing

```bash
# Backend — 28 test files, race detection
go test -race -count=1 ./...

# Frontend — 19 test files
cd web && npm test -- --run
```

CI runs on every push and PR: `go vet` + `go test -race` with PostgreSQL, plus TypeScript type checking, Vitest, and production build verification.

---

## Project Structure

```
cmd/server/main.go           Application entrypoint
cmd/healthcheck/main.go      Container health binary
internal/api/                HTTP handlers + middleware (one file per resource)
internal/auth/               Session, password, OAuth, API key, CSRF modules
internal/store/              Database operations (raw SQL via pgx)
internal/db/                 Connection pool + migration runner
internal/notify/             Async webhook dispatcher (worker pool, HMAC, retry)
internal/config/             Environment variable loading
internal/errors/             APIError type + constructors
internal/ratelimit/          Sliding-window rate limiter
internal/seed/               First-boot agent seeder (16 product agents)
web/src/                     React + PatternFly 5 admin GUI
migrations/                  SQL migrations (embedded in binary)
deployment/                  Docker Compose + env examples
docs/                        Architecture, API, auth, deployment, dev guides
```

---

## Roadmap

The core product is shipped. The roadmap focuses on protocol interoperability and platform capabilities.

| Phase | Focus | Status |
|-------|-------|--------|
| **Phase 6** | MCP Server Facade, A2A Agent Cards, MCP Gateway Mode | Planned |
| **Phase 7** | Multi-Tenancy, Semantic Discovery, Real-Time Streaming, Observability | Planned |
| **Phase 8** | Package Ecosystem (`.agentpkg.json` format) | Planned |

> [Full roadmap with detailed proposals](docs/roadmap/README.md)

---

## Documentation

| Guide | Description |
|-------|-------------|
| [Architecture](docs/architecture.md) | System design, layers, technology decisions |
| [API Reference](docs/api-reference.md) | Every endpoint with examples |
| [Authentication](docs/authentication.md) | Three auth methods, RBAC, CSRF, sessions |
| [Deployment](docs/deployment.md) | Docker, Compose, config, security hardening |
| [Development](docs/development.md) | Local setup, testing, conventions, contributing |
| [Security Checklist](docs/security-checklist.md) | 67-point OWASP verification |
| [Roadmap](docs/roadmap/README.md) | Protocol interop, platform, ecosystem plans |

---

## License

MIT

---

<p align="center">
  Built for the <a href="https://github.com/agent-smit">Agent Smit</a> platform.
</p>
