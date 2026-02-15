# Roadmap

> Where we're headed next — protocol interoperability, platform capabilities, and ecosystem.

Last updated: February 2026

---

## Current State

The Agentic Registry is the **most feature-complete self-hosted agent configuration management server available today**. No single alternative covers the same breadth of artifacts with the same depth of versioning, security, and built-in admin GUI.

### What's Shipped

| Capability | Status |
|---|---|
| Full agent lifecycle CRUD with versioning and rollback | Shipped |
| Versioned prompts with activation, rollback, and diff view | Shipped |
| MCP server config with AES-256-GCM encrypted credentials | Shipped |
| Trust classification chain (agent overrides, workspace rules, system defaults) | Shipped |
| Global and workspace-scoped model configuration with inheritance | Shipped (legacy — superseded by [009](009-model-endpoints.md)) |
| Versioned model endpoint registry with activation and rollback ([009](009-model-endpoints.md)) | **Shipped** |
| Three auth methods (password, Google OAuth 2.0 PKCE, API keys) | Shipped |
| OWASP-grade security (CSRF, bcrypt, rate limiting, security headers) | Shipped |
| Comprehensive audit logging for every mutation | Shipped |
| Webhook push notifications with HMAC-SHA256 signing | Shipped |
| Composite discovery endpoint (single-call cache hydration) | Shipped |
| Full React + PatternFly 5 admin GUI embedded in binary | Shipped |

### Where the Market Is Going

The AI agent ecosystem is converging on two interoperability protocols — **MCP** (Model Context Protocol) and **A2A** (Agent-to-Agent). The enterprise segment demands multi-tenancy, runtime governance, and semantic discovery. This roadmap addresses those gaps.

---

## Phase 6 — Protocol Interoperability (Critical)

| # | Item | Priority | Effort | Status |
|---|---|---|---|---|
| [001](001-mcp-server-facade.md) | **MCP Server Facade** — Expose the registry as a native MCP server for Claude, Cursor, and VS Code discovery | Critical | 3–4 weeks | Planned |
| [002](002-a2a-agent-card.md) | **A2A Agent Card** — Publish `.well-known/agent.json` for A2A ecosystem interoperability | High | 2 weeks | Planned |
| [003](003-mcp-gateway-mode.md) | **MCP Gateway Mode** — Runtime proxy for tool calls with trust enforcement, audit, and circuit breaking | High | 4–6 weeks | Planned |

**Why these first:** Without MCP and A2A support, we are invisible to the two protocols the AI agent ecosystem is standardizing on. These items transform us from a configuration store into a platform.

## Phase 7 — Platform Capabilities

| # | Item | Priority | Effort | Status |
|---|---|---|---|---|
| [004](004-multi-tenancy.md) | **Multi-Tenancy & Federation** — Tenant isolation, config export/import, cross-instance sync | Medium | 4–5 weeks | Planned |
| [005](005-semantic-discovery.md) | **Semantic Discovery** — Natural language search over agent capabilities using pgvector embeddings | Medium | 2–3 weeks | Planned |
| [006](006-realtime-streaming.md) | **Real-Time Streaming** — SSE for config changes, health status, and progressive discovery | Medium | 2–3 weeks | Planned |
| [007](007-advanced-observability.md) | **Advanced Observability** — Per-tool usage analytics, cost attribution, health dashboards, alerts | Medium | 2–3 weeks | Planned |
| [009](009-model-endpoints.md) | **Model Endpoints** — Replace global model config with versioned, addressable model endpoint assets (fixed or multi-model, with rollback) | High | 2–3 weeks | **Shipped** |

## Phase 8 — Ecosystem

| # | Item | Priority | Effort | Status |
|---|---|---|---|---|
| [008](008-package-ecosystem.md) | **Package Ecosystem** — Portable `.agentpkg.json` format with import/export API and CLI | Low | 3 weeks | Planned |

---

## Dependency Graph

```
                    ┌─────────────────┐
                    │ 001 MCP Facade  │
                    └────────┬────────┘
                             │ shares transport code
                             ▼
┌──────────────────┐  ┌─────────────────┐  ┌───────────────────┐
│ 002 A2A Cards    │  │ 003 MCP Gateway │  │ 006 Streaming     │
│ (independent)    │  │                 │  │ (independent)     │
└──────────────────┘  └────────┬────────┘  └───────────────────┘
                               │ provides per-tool data
                               ▼
                    ┌─────────────────────┐
                    │ 007 Observability   │
                    │ (benefits from 003) │
                    └─────────────────────┘

┌──────────────────┐           ┌───────────────────┐
│ 004 Multi-Tenancy│           │ 005 Semantic      │
│ (independent)    │           │ Discovery         │
└────────┬─────────┘           │ (independent)     │
         │ benefits from       └───────────────────┘
         ▼
┌──────────────────┐           ┌───────────────────┐
│ 008 Packages     │           │ 009 Model         │
│ (benefits from   │           │ Endpoints         │
│  004 for cross-  │           │ (independent;     │
│  tenant import)  │           │  replaces global  │
└──────────────────┘           │  model config)    │
                               └───────────────────┘
```

Most items are independent and can be parallelized. Recommended critical path: **001 → 003 → 007**, with **002** and **006** built concurrently.

---

## Effort Summary

| Phase | Items | Effort |
|---|---|---|
| Phase 6 — Protocol Interoperability | 001, 002, 003 | 9–12 weeks |
| Phase 7 — Platform Capabilities | 004, 005, 006, 007, 009 | 12–17 weeks |
| Phase 8 — Ecosystem | 008 | 3 weeks |
| **Total** | **9 items** | **24–32 weeks** |

With parallelization, the full roadmap can be delivered in approximately **17–22 weeks** (4–5 months).

---

## Competitive Positioning After Roadmap

| Dimension | Current | After Roadmap | Best Competitor |
|---|---|---|---|
| Config breadth | Best-in-class | Best-in-class | None match |
| Versioning & rollback | Best-in-class | Best-in-class | Agenta (prompts only) |
| Security hardening | Best-in-class | Best-in-class | Kong (delegated) |
| Built-in admin GUI | Best OSS | Best OSS | Kong Konnect (commercial) |
| MCP protocol support | — | Full (facade + gateway) | MCPJungle |
| A2A interoperability | — | Per-agent cards + well-known | Google A2A Registry |
| Multi-tenancy | — | Full isolation + federation | Microsoft Entra |
| Runtime governance | — | Full proxy with trust enforcement | Kong (commercial) |
| Semantic discovery | — | Embedding-based + skill taxonomy | A2A Registry |
| Real-time streaming | — | SSE for changes + health + discovery | MCPJungle |
| Observability & cost | — | Full analytics + dashboards + alerts | Kong (commercial) |
| Package ecosystem | — | Portable format + CLI | Official MCP Registry |

After completing this roadmap, the Agentic Registry will be the **only solution — open source or commercial — that combines full agent configuration management, MCP gateway capabilities, A2A interoperability, multi-tenancy, semantic discovery, real-time streaming, and cost-aware observability in a single self-hosted binary**.
