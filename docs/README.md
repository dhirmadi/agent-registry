# Documentation

> Everything you need to understand, deploy, and contribute to the Agentic Registry.

---

## Guides

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | System design, layered architecture, technology decisions, and project layout |
| [API Reference](api-reference.md) | Complete REST API documentation with request/response examples |
| [Authentication](authentication.md) | Password, Google OAuth, API key auth — plus RBAC, CSRF, and session management |
| [Deployment](deployment.md) | Docker, Compose, configuration, security hardening, health checks |
| [Development](development.md) | Local setup, building, testing, code conventions, and contribution guide |
| [Security Checklist](security-checklist.md) | 67-point security verification against OWASP best practices |

## Roadmap

| Document | Description | Status |
|----------|-------------|--------|
| [Roadmap Overview](roadmap/README.md) | Phases 6–8: protocol interoperability, platform capabilities, ecosystem | |
| [001 — MCP Server Facade](roadmap/001-mcp-server-facade.md) | Expose the registry as a native MCP server | Planned |
| [002 — A2A Agent Card](roadmap/002-a2a-agent-card.md) | Publish Agent Cards for A2A ecosystem discovery | Planned |
| [003 — MCP Gateway Mode](roadmap/003-mcp-gateway-mode.md) | Runtime proxy with trust enforcement and audit | Planned |
| [004 — Multi-Tenancy](roadmap/004-multi-tenancy.md) | Tenant isolation and cross-instance federation | Planned |
| [005 — Semantic Discovery](roadmap/005-semantic-discovery.md) | Natural language search over agent capabilities | Planned |
| [006 — Real-Time Streaming](roadmap/006-realtime-streaming.md) | SSE for config changes and health status | Planned |
| [007 — Advanced Observability](roadmap/007-advanced-observability.md) | Usage analytics, cost attribution, and dashboards | Planned |
| [008 — Package Ecosystem](roadmap/008-package-ecosystem.md) | Portable `.agentpkg.json` format with CLI | Planned |
| [009 — Model Endpoints](roadmap/009-model-endpoints.md) | Versioned model provider endpoint registry with rollback | **Shipped** |

## Archive

| Document | Description |
|----------|-------------|
| [Original Implementation Spec (v1)](archive/agentic_registry_spec_v1.md) | The initial specification used to build the first release. Archived for reference. |
