# Deployment Guide

> How to build, configure, and run the Agentic Registry in production.

---

## Quick Start (Docker Compose)

The fastest way to get running. This starts PostgreSQL and the registry together.

```bash
# Clone the repository
git clone https://github.com/dhirmadi/agent-registry.git
cd agentic-registry

# Configure
cp deployment/.env.example deployment/.env
# Edit deployment/.env — set SESSION_SECRET and CREDENTIAL_ENCRYPTION_KEY

# Generate secrets
openssl rand -hex 32    # → SESSION_SECRET
openssl rand -base64 32 # → CREDENTIAL_ENCRYPTION_KEY

# Start
docker compose -f deployment/compose.yaml up -d

# Verify
curl http://localhost:8090/healthz
```

The admin GUI is available at `http://localhost:8090`. Default login: `admin` / `admin` (you'll be prompted to change the password).

---

## Container Image

### Building

The Dockerfile uses a multi-stage build:

1. **Stage 1 (Node 20):** Builds the React admin GUI → `web/dist/`
2. **Stage 2 (Go):** Compiles the server and healthcheck binaries, embedding the GUI
3. **Stage 3 (Alpine 3.19):** Minimal runtime image with just the two binaries

```bash
docker build -t agentic-registry .
```

### Image Characteristics

| Property | Value |
|----------|-------|
| Base image | `alpine:3.19` |
| User | `registry` (UID 1001, non-root) |
| Port | `8090` |
| Healthcheck | Built-in (`/healthcheck` binary hits `/healthz`) |
| Filesystem | Read-only recommended (`read_only: true`) |
| Temp storage | `tmpfs` at `/tmp` (10 MB) |

### Running Standalone

```bash
docker run -d \
  --name registry \
  -p 8090:8090 \
  -e DATABASE_URL="postgres://user:pass@host:5432/agentic_registry?sslmode=require" \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -e CREDENTIAL_ENCRYPTION_KEY="$(openssl rand -base64 32)" \
  --read-only \
  --tmpfs /tmp:size=10M \
  agentic-registry
```

---

## Configuration

All configuration is via environment variables. No config files.

### Required

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@host:5432/db?sslmode=require` |
| `SESSION_SECRET` | 64-char hex string for session signing | `openssl rand -hex 32` |
| `CREDENTIAL_ENCRYPTION_KEY` | 32-byte base64 key for AES-256-GCM | `openssl rand -base64 32` |

### Server

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP listen port | `8090` |
| `LOG_LEVEL` | Logging level: `debug`, `info`, `warn`, `error` | `info` |
| `EXTERNAL_URL` | Public URL (used for OAuth redirect URI) | `http://localhost:8090` |

### Google OAuth (Optional)

| Variable | Description |
|----------|-------------|
| `GOOGLE_OAUTH_CLIENT_ID` | OAuth 2.0 client ID from Google Cloud Console |
| `GOOGLE_OAUTH_CLIENT_SECRET` | OAuth 2.0 client secret |

Omit both to disable Google login entirely. The admin GUI automatically hides the "Sign in with Google" button when OAuth is not configured.

**Google Cloud Console setup:**
1. Create or select a project
2. Enable the Google+ API
3. Create OAuth 2.0 credentials (Web application)
4. Add authorized redirect URI: `{EXTERNAL_URL}/auth/google/callback`

### Webhooks

| Variable | Description | Default |
|----------|-------------|---------|
| `WEBHOOK_TIMEOUT` | Delivery timeout in seconds | `5` |
| `WEBHOOK_RETRIES` | Retry attempts on failure | `3` |
| `WEBHOOK_WORKERS` | Concurrent delivery goroutines | `4` |

### OpenTelemetry (Optional)

| Variable | Description | Default |
|----------|-------------|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint | (disabled) |
| `OTEL_SERVICE_NAME` | Service name in traces | `agentic-registry` |

---

## Database

### Requirements

- PostgreSQL 16 or later
- A dedicated database (e.g., `agentic_registry`)
- A user with full privileges on that database

### Migrations

Migrations run automatically on server startup. They are embedded in the binary — no external migration files needed. The server uses `golang-migrate/v4` with advisory locks to prevent concurrent migration runs.

### Connection Pool

The server uses `pgx/v5` connection pooling. Default pool settings are suitable for most deployments. For high-traffic deployments, PostgreSQL connection limits should be set appropriately.

### Backup

Standard PostgreSQL backup practices apply:

```bash
# Logical backup
pg_dump -Fc agentic_registry > backup.dump

# Restore
pg_restore -d agentic_registry backup.dump
```

---

## Docker Compose Reference

The provided `deployment/compose.yaml` is production-ready with:

- **PostgreSQL 16** with health checking and persistent volume
- **Registry** with read-only filesystem, tmpfs, and health checking
- **Dependency ordering** — Registry waits for PostgreSQL to be healthy

```yaml
services:
  db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-registry}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-registry}
      POSTGRES_DB: ${POSTGRES_DB:-agentic_registry}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U registry"]
      interval: 5s

  registry:
    build:
      context: ..
      dockerfile: Dockerfile
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    read_only: true
    tmpfs:
      - /tmp:size=10M
    ports:
      - "${PORT:-8090}:${PORT:-8090}"
    healthcheck:
      test: ["CMD", "/healthcheck"]
      interval: 30s

volumes:
  pgdata:
```

---

## Security Hardening

### Container

- Run as non-root (UID 1001)
- Read-only root filesystem
- Minimal tmpfs for temporary files
- No shell in the final image (Alpine minimal)

### Network

- Enable TLS termination via a reverse proxy (Nginx, Caddy, Traefik, cloud LB)
- Set `EXTERNAL_URL` to the HTTPS URL
- HSTS header is set automatically (`max-age=63072000; includeSubDomains`)
- CORS is restricted to same-origin

### Secrets

- `SESSION_SECRET` — Rotate by generating a new value and restarting. Active sessions will be invalidated.
- `CREDENTIAL_ENCRYPTION_KEY` — Rotation requires re-entering MCP server credentials (encrypted values become unreadable with a new key).
- Never commit secrets to version control. Use environment variables, secret managers, or orchestrator secrets.

### Headers

The server sets security headers on every response:

| Header | Value |
|--------|-------|
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; ...` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` |

---

## Health Checks

### Endpoints

| Endpoint | What It Checks | Use For |
|----------|---------------|---------|
| `GET /healthz` | Server process is running | Kubernetes liveness probe, container health |
| `GET /readyz` | Database connection is alive | Kubernetes readiness probe, load balancer |

### Container Healthcheck

The image includes a dedicated `/healthcheck` binary that calls `/healthz` and exits with code 0 (healthy) or 1 (unhealthy). This is configured in the Dockerfile:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/healthcheck"]
```

---

## First Boot

On first startup with an empty database:

1. Migrations run automatically (creating all tables)
2. Trust defaults are seeded (built-in trust classification patterns)
3. Global model configuration is seeded (sensible defaults)
4. Default admin account is created (`admin`/`admin`, must change password)
5. 16 product agents are seeded (6 with full tool definitions, 10 placeholders)

No manual setup required beyond providing the three required environment variables.
