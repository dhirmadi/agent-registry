# 003 — MCP Gateway Mode

> **Priority:** High
> **Phase:** 6.3
> **Effort:** Large (4–6 weeks)
> **Status:** Planned

---

## Problem

Today we store MCP server configurations (endpoint URLs, auth credentials, circuit breaker settings) but never touch the actual traffic between agents and MCP servers. We are a *control plane* without a *data plane*.

Competitors like MCPJungle and Kong's MCP Registry act as runtime proxies — intercepting every tool call, enforcing policies, tracking costs, and providing unified observability. Enterprises need this gateway layer to prevent "shadow AI" tool usage and enforce governance at execution time.

## Solution

Add an optional gateway mode where the registry proxies MCP tool calls through itself, applying authentication, access control, rate limiting, auditing, and circuit breaking at the individual tool-call level.

### Deliverables

1. **MCP Proxy Endpoint**
   - `POST /mcp/v1/proxy/{serverLabel}/tools/{toolName}` — proxy a tool call to a registered MCP server
   - Registry resolves the target server from its config, injects stored credentials, forwards the call
   - Returns the MCP server's response to the caller

2. **Aggregated Tool Discovery**
   - `GET /mcp/v1/tools` — returns a unified tool list aggregated from all enabled MCP servers
   - Merges tools from multiple backends into a single namespace
   - Applies access control: only tools the caller is authorized to use are listed
   - Supports filtering by server label, tool name pattern, or trust tier

3. **Runtime Policy Enforcement**
   - **Trust tier enforcement** — before proxying a tool call, check the trust classification chain (agent overrides → workspace rules → system defaults) and block/require-review as configured
   - **Per-tool rate limiting** — configurable per MCP server and per tool
   - **Per-user access control** — restrict which users/API keys can call which tools
   - **Input validation** — optional JSON Schema validation of tool arguments before forwarding

4. **Circuit Breaker (Active)**
   - Implement the circuit breaker pattern using the `circuit_breaker_cfg` already stored on `MCPServer` records
   - States: closed → open (after N failures) → half-open (probe) → closed
   - Surface circuit state in health endpoints and admin GUI

5. **Call Auditing**
   - Log every proxied tool call to the `audit_log` table:
     - Who called it (user ID or API key)
     - Which tool and server
     - Request/response size
     - Latency
     - Success/failure
   - Expose in the Audit Log GUI page with tool-call-specific filters

6. **Credential Injection**
   - Decrypt stored `auth_credential` for the target MCP server at proxy time
   - Inject as Bearer token, Basic auth, or custom header depending on `auth_type`
   - Credentials never leave the registry server — callers never see them

### Architecture

```
Agent/BFF                 Agentic Registry                MCP Server
   │                           │                              │
   │  POST /mcp/v1/proxy/     │                              │
   │  mcp-git/tools/          │                              │
   │  git_read_file            │                              │
   │ ─────────────────────────>│                              │
   │                           │  1. Auth check               │
   │                           │  2. Trust tier check          │
   │                           │  3. Rate limit check          │
   │                           │  4. Circuit breaker check     │
   │                           │  5. Decrypt credentials       │
   │                           │  6. Forward to MCP server ───>│
   │                           │                              │
   │                           │  7. Log to audit_log   <─────│
   │                           │  8. Update circuit state      │
   │  <─────────────────────── │  9. Return response           │
   │                           │                              │
```

### Technical Considerations

- Gateway mode is **opt-in** via config: `GATEWAY_MODE=true` (default: false)
- When disabled, the proxy endpoints return 404 — zero overhead
- Use Go's `net/http` client with configurable timeouts per MCP server
- Connection pooling per MCP server for performance
- Consider supporting both JSON-RPC (MCP native) and REST forwarding
- Store circuit breaker state in-memory with periodic persistence to DB

### Acceptance Criteria

- [ ] Tool calls can be proxied through the registry to any configured MCP server
- [ ] Stored credentials are injected without exposure to the caller
- [ ] Trust tier rules block/allow tool calls based on classification
- [ ] Circuit breaker opens after configured failure threshold
- [ ] Every proxied call appears in the audit log
- [ ] Aggregated tool list merges tools from all enabled servers
- [ ] Gateway mode can be disabled with zero performance impact

### Competitive Context

| Competitor | Gateway Capabilities |
|---|---|
| MCPJungle | Full proxy, tool groups, no per-call audit |
| Kong MCP Registry | Full proxy, ACLs, token rate limits, commercial |
| MCP Gateway Registry | OAuth proxy, Keycloak integration |
| **Us (current)** | **Config storage only — no runtime proxy** |
| **Us (after this item)** | **Full proxy with trust enforcement, audit, circuit breaking** |

### Dependencies

- 001 (MCP Server Facade) — shares MCP transport code
- Existing MCP server config and trust rule systems

### References

- [MCP Registries and Gateways (Paperclipped)](https://www.paperclipped.de/en/blog/mcp-registry-gateway-enterprise-ai-agents/)
- [Kong MCP Governance](https://konghq.com/solutions/mcp-governance)
- [MCPJungle GitHub](https://github.com/mcpjungle/MCPJungle)
