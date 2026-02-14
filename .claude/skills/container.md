# Skill: container

## Description
Builds, runs, and tests the Agentic Registry container image using Podman. Handles the full multi-stage build (Node + Go + Alpine), local PostgreSQL sidecar, and health verification.

## Execution Logic

When the user says "build container", "run container", "test container", or "/container":

### Build
```bash
podman build -t agentic-registry .
```
- Verify the multi-stage build completes (Node + Go + Alpine)
- Check final image size — should be under 30MB (Alpine + static Go binary)
- Report any build failures with the failing stage identified

### Run (with PostgreSQL sidecar)
```bash
# Create a pod for the registry + database
podman pod create --name areg-pod -p 8090:8090

# Start PostgreSQL
podman run -d --pod areg-pod --name areg-db \
  -e POSTGRES_USER=registry \
  -e POSTGRES_PASSWORD=localdev \
  -e POSTGRES_DB=agentic_registry \
  postgres:16-alpine

# Wait for PostgreSQL readiness
podman exec areg-db pg_isready -U registry --timeout=30

# Start the registry
podman run -d --pod areg-pod --name areg-server \
  -e DATABASE_URL="postgres://registry:localdev@localhost:5432/agentic_registry?sslmode=disable" \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -e CSRF_SECRET="$(openssl rand -hex 32)" \
  -e CREDENTIAL_ENCRYPTION_KEY="$(openssl rand -hex 16)" \
  -e ADMIN_PASSWORD="ChangeMe123!" \
  agentic-registry
```

### Health Check
```bash
# Wait for the healthcheck binary to report healthy
podman healthcheck run areg-server

# Verify API is responding
podman exec areg-server wget -qO- http://localhost:8090/api/v1/health/live
podman exec areg-server wget -qO- http://localhost:8090/api/v1/health/ready
```

### Cleanup
```bash
podman pod stop areg-pod
podman pod rm areg-pod
```

### Full Cycle
When asked to do a "full container test":
1. Build the image
2. Start pod with PostgreSQL
3. Wait for health checks to pass
4. Verify `/api/v1/health/live` and `/api/v1/health/ready` return 200
5. Verify migrations ran (ready endpoint reports database OK)
6. Clean up pod

## Constraints
- Use `podman` not `docker` — this system uses Podman 5.x
- Never persist secrets in image layers — all secrets via env vars at runtime
- Use `--pod` networking so containers share localhost
- Always clean up pods/containers after testing
- Port 8090 is the registry's default port
