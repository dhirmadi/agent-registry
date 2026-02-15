# Development Guide

> How to set up, build, test, and contribute to the Agentic Registry.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24+ | Backend |
| Node.js | 20+ | Frontend build and tests |
| PostgreSQL | 16+ | Database |
| Docker / Podman | Latest | Container builds |

---

## Local Setup

### 1. Clone and Install

```bash
git clone https://github.com/dhirmadi/agent-registry.git
cd agentic-registry

# Go dependencies
go mod download

# Frontend dependencies
cd web && npm install && cd ..
```

### 2. Database

Start PostgreSQL locally (or use the provided Compose file for just the database):

```bash
# Option A: Use Docker Compose for just the database
docker compose -f deployment/compose.yaml up db -d

# Option B: Use an existing PostgreSQL instance
createdb agentic_registry
```

### 3. Environment

```bash
cp .env.example .env
```

Edit `.env` with your local settings. At minimum:

```bash
DATABASE_URL=postgres://registry:localdev@localhost:5432/agentic_registry?sslmode=disable
SESSION_SECRET=$(openssl rand -hex 32)
CREDENTIAL_ENCRYPTION_KEY=$(openssl rand -base64 32)
```

### 4. Run

```bash
# Backend (runs migrations automatically)
go run cmd/server/main.go

# Frontend dev server (separate terminal, hot module reload)
cd web && npm run dev
```

The backend serves on `:8090`. The Vite dev server proxies API calls to the backend.

---

## Project Structure

```
cmd/
  server/main.go           # Application entrypoint
  healthcheck/main.go      # Container health binary

internal/
  api/                     # HTTP handlers + middleware
  auth/                    # Authentication modules
  config/                  # Environment variable loading
  db/                      # Database pool + migrations
  errors/                  # APIError type + constructors
  notify/                  # Webhook dispatcher
  ratelimit/               # Rate limiter
  seed/                    # First-boot agent seeder
  store/                   # Database operations
  telemetry/               # OpenTelemetry init

web/
  src/
    auth/                  # Login, auth context, route guards
    pages/                 # All admin GUI pages
    components/            # Shared UI components
    api/client.ts          # API client with CSRF handling
    types/index.ts         # TypeScript interfaces

migrations/                # Embedded SQL migrations
deployment/                # Docker Compose + env examples
docs/                      # Documentation
```

---

## Building

### Backend

```bash
# Build the server binary
go build -o registry cmd/server/main.go

# Build the healthcheck binary
go build -o healthcheck cmd/healthcheck/main.go

# Run
./registry
```

### Frontend

```bash
cd web

# Development server with HMR
npm run dev

# Production build → dist/
npm run build

# Type checking
npx tsc --noEmit
```

### Container

```bash
# Full multi-stage build
docker build -t agentic-registry .

# Run with Compose
docker compose -f deployment/compose.yaml up
```

---

## Testing

### Backend Tests

```bash
# All tests
go test ./...

# With race detector (recommended)
go test -race -count=1 ./...

# Single package
go test ./internal/api/...
go test ./internal/auth/...
go test ./internal/store/...

# Single test
go test -run TestAgentCreate ./internal/api

# Verbose output
go test -v -count=1 ./internal/auth/...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Go vet
go vet ./...
```

Tests require a PostgreSQL database. Set `DATABASE_URL` in your environment or `.env` file.

### Frontend Tests

```bash
cd web

# Run all tests (watch mode)
npm test

# Run once (CI mode)
npm test -- --run

# Run specific test file
npm test -- --run AgentsPage

# Coverage
npm test -- --run --coverage
```

### CI Pipeline

The GitHub Actions CI runs on every push and PR to `main`:

1. **Go job:** `go vet` + `go test -race -count=1 ./...` with PostgreSQL service container
2. **Web job:** `npm ci` + `npx tsc --noEmit` + `npm test -- --run` + `npm run build`

---

## Test Coverage

### Backend (28 test files)

| Area | Test Files |
|------|-----------|
| API handlers | `agents_test.go`, `prompts_test.go`, `mcp_servers_test.go`, `trust_rules_test.go`, `trust_defaults_test.go`, `model_config_test.go`, `webhooks_test.go`, `users_test.go`, `api_keys_test.go`, `discovery_test.go`, `audit_completeness_test.go` |
| Auth | `handler_test.go`, `session_test.go`, `password_test.go`, `csrf_test.go`, `oauth_test.go`, `apikey_test.go`, `crypto_test.go` |
| Middleware | `middleware_test.go`, `health_test.go`, `respond_test.go` |
| Integration | `integration_test.go`, `security_test.go` |
| Infrastructure | `config_test.go`, `dispatcher_test.go`, `limiter_test.go`, `errors_test.go`, `agents_test.go` (seed) |

### Frontend (19 test files)

| Area | Test Files |
|------|-----------|
| Auth | `AuthContext.test.tsx`, `LoginPage.test.tsx` |
| Pages | `DashboardPage.test.tsx`, `AgentsPage.test.tsx`, `AgentDetailPage.test.tsx`, `PromptsPage.test.tsx`, `MCPServersPage.test.tsx`, `TrustPage.test.tsx`, `ModelConfigPage.test.tsx`, `WebhooksPage.test.tsx`, `APIKeysPage.test.tsx`, `UsersPage.test.tsx`, `AuditLogPage.test.tsx`, `MyAccountPage.test.tsx` |
| Components | `AppLayout.test.tsx`, `ErrorBoundary.test.tsx`, `ConfirmDialog.test.tsx`, `DiffViewer.test.tsx` |
| E2E | `smoke.test.tsx` |

---

## Code Conventions

### Go

- **No ORM.** All SQL is explicit in `internal/store/` using `$1` parameterized queries.
- **No third-party test frameworks.** Use stdlib `testing` with `t.Run()` table-driven tests.
- **Handler pattern:** Parse → Validate → Store call → Audit → Webhook → Respond.
- **Error handling:** Use `internal/errors.APIError` constructors (`NotFound()`, `Conflict()`, `Validation()`, `Forbidden()`).
- **Secrets:** Always `json:"-"` on hash fields. Never log encryption keys.
- **Every mutation must call `store.InsertAuditLog()`.**

### Frontend

- **React 18** with functional components and hooks.
- **PatternFly 5** for all UI components — no custom CSS for standard patterns.
- **TypeScript** strict mode.
- **Testing Library** — test user-visible behavior, not implementation details.

### Commits

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

**Types:** `feat`, `fix`, `test`, `refactor`, `docs`, `chore`, `ci`, `perf`

**Scopes:** `api`, `auth`, `store`, `db`, `config`, `notify`, `ratelimit`, `telemetry`, `errors`, `web`, `migrations`, `docker`, `ci`

Examples:
```
feat(api): add agent CRUD endpoints with versioning
test(auth): add table-driven tests for session lifecycle
fix(store): use transaction for prompt activation swap
refactor(notify): extract HMAC signing into helper
docs(web): update component documentation
```

---

## Adding a New Resource

Follow this pattern when adding a new API resource:

### 1. Migration

Create `migrations/NNN_resource_name.up.sql` and `migrations/NNN_resource_name.down.sql`.

### 2. Store

Create `internal/store/resource_name.go` with CRUD functions using raw pgx queries.

### 3. Handler

Create `internal/api/resource_name.go` implementing the handler struct with methods for each endpoint. Follow the existing pattern: parse, validate, store, audit, webhook, respond.

### 4. Router

Register routes in `internal/api/router.go` following the existing pattern with role-based middleware.

### 5. Frontend

Create `web/src/pages/ResourceNamePage.tsx` using PatternFly components.

### 6. Tests

- Backend: `internal/api/resource_name_test.go` with table-driven tests
- Frontend: `web/src/pages/ResourceNamePage.test.tsx`

### 7. Verify Audit Coverage

Run `go test -run TestAuditCompleteness ./internal/api` to confirm your mutation handlers record audit entries.
