# Agentic Registry — Roadmap to Best-in-Class

> Last updated: February 2026

## Where We Stand

The Agentic Registry is the **most feature-complete self-hosted agent configuration management server available today**. No single competitor covers the same breadth of artifacts (agents, versioned prompts, MCP server configs, trust rules, triggers, model parameters, context budgets, signal configs) with the same depth of versioning, security hardening, and built-in admin GUI.

### Current Strengths

| Capability | Status |
|---|---|
| Full agent lifecycle CRUD with versioning & rollback | Shipped |
| Versioned prompts with activation, rollback, and diff view | Shipped |
| MCP server config with AES-256-GCM encrypted credentials | Shipped |
| Trust classification chain (agent → workspace → system defaults) | Shipped |
| Trigger rules with cron scheduling and rate limits | Shipped |
| Model, context, and signal configuration with scope inheritance | Shipped |
| Three auth methods (password, Google OAuth PKCE, API keys) | Shipped |
| OWASP-grade security (CSRF, bcrypt, rate limiting, security headers) | Shipped |
| Comprehensive audit logging for every mutation | Shipped |
| Webhook push notifications with HMAC-SHA256 signing | Shipped |
| Composite discovery endpoint (single-call cache hydration) | Shipped |
| 15-page React + PatternFly 5 admin GUI embedded in binary | Shipped |
| OpenTelemetry traces + Prometheus metrics | Shipped |

### Current Gaps

The market is converging on two interoperability protocols — **MCP** (Model Context Protocol) and **A2A** (Agent-to-Agent) — and the enterprise segment demands multi-tenancy, runtime governance, and semantic discovery. These are the gaps this roadmap addresses.

---

## Roadmap Overview

### Phase 6 — Protocol Interoperability (Critical)

| # | Item | Priority | Effort | Status |
|---|---|---|---|---|
| [001](001-mcp-server-facade.md) | **MCP Server Facade** — Expose the registry as a native MCP server for Claude/Cursor/VS Code discovery | Critical | 3–4 weeks | Planned |
| [002](002-a2a-agent-card.md) | **A2A Agent Card** — Publish `.well-known/agent.json` for A2A ecosystem interoperability | High | 2 weeks | Planned |
| [003](003-mcp-gateway-mode.md) | **MCP Gateway Mode** — Runtime proxy for tool calls with trust enforcement, audit, and circuit breaking | High | 4–6 weeks | Planned |

**Why these first:** Without MCP and A2A support, we are invisible to the two protocols that the AI agent ecosystem is standardizing on. These items transform us from a configuration store into a platform.

### Phase 7 — Platform Capabilities

| # | Item | Priority | Effort | Status |
|---|---|---|---|---|
| [004](004-multi-tenancy.md) | **Multi-Tenancy & Federation** — Tenant isolation, config export/import, cross-instance sync | Medium | 4–5 weeks | Planned |
| [005](005-semantic-discovery.md) | **Semantic Discovery** — Natural language search over agent capabilities using pgvector embeddings | Medium | 2–3 weeks | Planned |
| [006](006-realtime-streaming.md) | **Real-Time Streaming** — SSE for config changes, health status, and progressive discovery | Medium | 2–3 weeks | Planned |
| [007](007-advanced-observability.md) | **Advanced Observability** — Per-tool usage analytics, cost attribution, health dashboards, alerts | Medium | 2–3 weeks | Planned |

### Phase 8 — Ecosystem

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
┌──────────────────┐
│ 008 Packages     │
│ (benefits from   │
│  004 for cross-  │
│  tenant import)  │
└──────────────────┘
```

Most items are independent and can be parallelized. The recommended critical path is: **001 → 003 → 007**, with **002** and **006** built concurrently.

---

## Effort Summary

| Phase | Items | Total Effort |
|---|---|---|
| Phase 6 — Protocol Interoperability | 001, 002, 003 | 9–12 weeks |
| Phase 7 — Platform Capabilities | 004, 005, 006, 007 | 10–14 weeks |
| Phase 8 — Ecosystem | 008 | 3 weeks |
| **Total** | **8 items** | **22–29 weeks** |

With parallelization on the critical path, the full roadmap can be delivered in approximately **16–20 weeks** (4–5 months).

---

## Competitive Positioning After Roadmap

| Dimension | Current | After Roadmap | Best Competitor |
|---|---|---|---|
| Config breadth | Best-in-class | Best-in-class | None match |
| Versioning & rollback | Best-in-class | Best-in-class | Agenta (prompts only) |
| Security hardening | Best-in-class | Best-in-class | Kong (delegated) |
| Built-in admin GUI | Best OSS | Best OSS | Kong Konnect (commercial) |
| MCP protocol support | Missing | Full (facade + gateway) | MCPJungle |
| A2A interoperability | Missing | Per-agent cards + well-known | Google A2A Registry |
| Multi-tenancy | Partial | Full isolation + federation | Microsoft Entra |
| Runtime governance | Missing | Full proxy with trust enforcement | Kong (commercial) |
| Semantic discovery | Missing | Embedding-based + skill taxonomy | A2A Registry |
| Real-time streaming | Missing | SSE for changes + health + discovery | MCPJungle |
| Observability & cost | Basic | Full analytics + dashboards + alerts | Kong (commercial) |
| Package ecosystem | Missing | Portable format + CLI | Official MCP Registry |

After completing this roadmap, the Agentic Registry will be the **only solution — open source or commercial — that combines full agent configuration management, MCP gateway capabilities, A2A interoperability, multi-tenancy, semantic discovery, real-time streaming, and cost-aware observability in a single self-hosted binary**.
