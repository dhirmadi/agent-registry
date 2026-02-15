---
name: container
description: Build, run, and test the Agentic Registry container image with Podman. Handles multi-stage build, PostgreSQL sidecar, and health verification. Use when the user says "build container", "run container", "test container", or "docker build".
---

# Container Workflows

Uses **Podman** (not Docker). The Dockerfile is a multi-stage build: Node (frontend) → Go (backend) → Alpine (runtime).

## Build

```bash
podman build -t agentic-registry .
```

Target image size: under 30MB.

## Run with PostgreSQL Sidecar

```bash
podman pod create --name areg-pod -p 8090:8090

podman run -d --pod areg-pod --name areg-db \
  -e POSTGRES_USER=registry \
  -e POSTGRES_PASSWORD=localdev \
  -e POSTGRES_DB=agentic_registry \
  postgres:16-alpine

podman exec areg-db pg_isready -U registry --timeout=30

podman run -d --pod areg-pod --name areg-server \
  -e DATABASE_URL="postgres://registry:localdev@localhost:5432/agentic_registry?sslmode=disable" \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -e CSRF_SECRET="$(openssl rand -hex 32)" \
  -e CREDENTIAL_ENCRYPTION_KEY="$(openssl rand -hex 16)" \
  -e ADMIN_PASSWORD="ChangeMe123!" \
  agentic-registry
```

## Health Check

```bash
podman exec areg-server wget -qO- http://localhost:8090/api/v1/health/live
podman exec areg-server wget -qO- http://localhost:8090/api/v1/health/ready
```

## Cleanup

```bash
podman pod stop areg-pod && podman pod rm areg-pod
```

## Full Cycle

1. Build image
2. Start pod + PostgreSQL
3. Wait for health checks
4. Verify `/api/v1/health/live` and `/api/v1/health/ready` return 200
5. Clean up

## Constraints

- Use `podman` not `docker`
- Never persist secrets in image layers
- Use `--pod` networking (containers share localhost)
- Always clean up after testing
- Default port: 8090
