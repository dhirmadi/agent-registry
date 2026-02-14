# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## WHY — Purpose and Context

The **Agentic Registry** exists to decouple all AI agent configuration from the Agent Smit BFF container. Today, agent definitions, prompts, MCP endpoints, trust rules, model parameters, context budgets, and signal intervals are all hardcoded — requiring a container rebuild for every change.

This microservice becomes the **single source of truth** for agent configuration, serving it via REST API and notifying consumers (BFF) of changes via webhooks. It includes a built-in admin GUI so operators can manage artifacts without touching code.

**The canonical specification is `docs/specification/agentic_registry_spec.md`.** It is the authority on all requirements, API contracts, schemas, auth flows, and phased delivery. Always consult it before implementing.

---

## WHAT — Tech Stack and Architecture

### Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.24 |
| Router | chi/v5 (no gin/echo/gorilla) |
| Database | PostgreSQL 16 via pgx/v5 (no ORM) |
| Migrations | golang-migrate/v4 with embedded SQL |
| Auth | bcrypt (cost 12), Google OAuth 2.0 PKCE, SHA-256 API keys |
| Crypto | stdlib only — AES-256-GCM, HMAC-SHA256, crypto/rand |
| Frontend | React 18 + PatternFly 5 + Vite + TypeScript |
| Tests (Go) | stdlib `testing` package |
| Tests (Frontend) | Vitest + @testing-library/react |
| Observability | OpenTelemetry (traces) + Prometheus (metrics) |
| Deployment | Multi-stage Docker (Node → Go → Alpine) |

### Project Layout

```
cmd/server/main.go           # Entrypoint: config, DB, router, OTel, seeder, graceful shutdown
cmd/healthcheck/main.go       # Container HEALTHCHECK binary
internal/api/                 # HTTP handlers + middleware (one file per resource)
internal/auth/                # Session, password, OAuth, API key, CSRF modules
internal/store/               # Database operations (one file per resource, raw SQL via pgx)
internal/db/                  # pgxpool wrapper + migration runner
internal/notify/              # Async webhook dispatcher (worker pool, HMAC, retry)
internal/config/              # Env var loading into Config struct
internal/errors/              # APIError type + constructors
internal/ratelimit/           # Rate limiting
internal/telemetry/           # OTel init
web/                          # React admin GUI (embedded in Go binary via //go:embed)
migrations/                   # SQL migrations 001–016+ (embedded in binary)
```

### Key Architecture Decisions

- **Layered flow:** Handler → Store → DB. Handlers never touch pgx directly.
- **No ORM.** All SQL is explicit in `internal/store/` files.
- **Embedded SPA.** React app is built at Docker build time, served via `embed.FS` at `/`. API routes (`/api/v1/*`, `/auth/*`) take priority over the SPA fallback.
- **Optimistic concurrency.** `updated_at` is used as an ETag — clients must send `If-Match` to prevent lost updates.
- **Webhook push.** All mutations dispatch async notifications (HMAC-SHA256 signed) to subscribers.
- **Transactional writes.** All store mutations that touch multiple rows use database transactions.
- **Three auth methods coexist:** password (bootstrap), Google OAuth (production SSO), API keys (service-to-service). Session cookie: `__Host-session`, HttpOnly, Secure, SameSite=Lax.
- **Audit everything.** Every mutation is recorded in the `audit_log` table with actor, action, and timestamp.

### API Response Envelope

All endpoints return:
```json
{
  "success": true|false,
  "data": { ... },
  "error": { "code": "...", "message": "..." } | null,
  "meta": { "timestamp": "...", "request_id": "..." }
}
```

---

## HOW — Build, Test, and Run

### Backend (Go)

```bash
go build -o registry cmd/server/main.go       # Build binary
go run cmd/server/main.go                       # Run server
go test ./...                                   # All tests
go test ./internal/store/...                    # Single package
go test -run TestAgentCreate ./internal/api     # Single test
go test -v -count=1 ./internal/auth/...         # Verbose, no cache
go test -race ./...                             # Race detector
go test -coverprofile=coverage.out ./...        # Coverage
```

### Frontend (web/)

```bash
cd web && npm install                           # Install deps
cd web && npm run dev                           # Dev server (HMR)
cd web && npm run build                         # Production build → dist/
cd web && npm test                              # Run Vitest
cd web && npm test -- --run                     # Run once (no watch)
```

### Docker

```bash
docker build -t agentic-registry .              # Full multi-stage build
```

### Database

PostgreSQL 16 required. Migrations run automatically on server startup via golang-migrate. Connection string via `DATABASE_URL` env var.

---

## Rules

### 1. Spec Is Law

The file `docs/specification/agentic_registry_spec.md` is the single source of truth. Do not invent endpoints, fields, or behaviors not described there. If the spec is ambiguous, ask — don't assume.

### 2. Test-Driven Development (TDD)

- **Write the test first.** Before implementing any handler, store function, or auth module, write the test that defines the expected behavior.
- **Red → Green → Refactor.** Confirm the test fails, write the minimal code to pass it, then clean up.
- **Test file placement:** `foo_test.go` lives next to `foo.go` in the same package.
- **Table-driven tests.** Use Go's `t.Run()` with test case slices for endpoints with multiple scenarios (valid input, missing fields, auth failures, role restrictions).
- **Frontend tests:** Use Vitest + @testing-library/react. Test user-visible behavior, not implementation details.
- **No skipping tests.** Every slice must have passing tests before moving to the next.

### 3. Conventional Commits

All commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:** `feat`, `fix`, `test`, `refactor`, `docs`, `chore`, `ci`, `perf`

**Scopes** (match project modules):
- `api`, `auth`, `store`, `db`, `config`, `notify`, `ratelimit`, `telemetry`, `errors`
- `web` (frontend), `migrations`, `docker`, `ci`

**Examples:**
```
feat(api): add agent CRUD endpoints with versioning
test(auth): add table-driven tests for session lifecycle
fix(store): use transaction for prompt activation swap
refactor(notify): extract HMAC signing into helper
chore(docker): add healthcheck binary to final stage
```

### 4. Dependency Discipline

Do not introduce frameworks or libraries beyond what the spec mandates (see Appendix C). No gin, echo, gorm, gorilla, or external crypto libraries. Chi for routing, pgx for database, stdlib for crypto.

### 5. Implementation Order

Follow the phased roadmap in spec Section 13. Each slice builds on the previous. Do not skip ahead — e.g., don't build webhook dispatch (Slice 3.2) before the resources that generate events (Phase 2).

### 6. Security Defaults

- Secrets (MCP credentials) encrypted at rest with AES-256-GCM.
- Passwords hashed with bcrypt cost 12.
- API keys stored as SHA-256 hashes, never plaintext.
- CSRF protection on all non-GET session-authenticated endpoints.
- Rate limiting on login (5 attempts → 15 min lockout) and API endpoints.
- No secrets in API responses. `password_hash` fields always use `json:"-"`.

---

## Implementation Roadmap Summary

| Phase | Slices | Focus |
|-------|--------|-------|
| 1 | 1.1–1.3 | Server skeleton, DB, health, auth (password + OAuth + API keys) |
| 2 | 2.1–2.5 | Resource CRUD: agents, prompts, MCP, trust, triggers |
| 3 | 3.1–3.3 | Global config, webhooks, discovery endpoint, agent seeding |
| 4 | 4.1–4.3 | Admin GUI: login, all resource pages, audit log |
| 5 | 5.1–5.2 | E2E validation + security audit |
