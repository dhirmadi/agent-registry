# 006 — Real-Time Streaming

> **Priority:** Medium
> **Phase:** 7.3
> **Effort:** Medium (2–3 weeks)
> **Status:** Planned

---

## Problem

Our registry is strictly request-response. Consumers (BFF, admin GUI) must poll for changes or rely on webhook delivery. This creates several limitations:

- **Config change latency** — BFF doesn't learn about a config change until the next webhook delivery (which may fail/retry)
- **Admin GUI staleness** — the GUI shows stale data until manual refresh
- **MCP server health** — no live health status updates; the GUI shows point-in-time snapshots
- **Large discovery payloads** — the composite discovery endpoint returns everything at once with no streaming option

All MCP-compatible competitors support Server-Sent Events (SSE) or WebSocket streaming. The MCP protocol itself is built on SSE for server-to-client communication.

## Solution

Add real-time streaming capabilities via Server-Sent Events (SSE) for config change notifications, live health status, and progressive data loading.

### Deliverables

1. **SSE Config Change Stream**
   - `GET /api/v1/stream/changes` — SSE endpoint that emits events when any configuration changes
   - Event types match our existing webhook event types:
     - `agent.created`, `agent.updated`, `agent.deleted`
     - `prompt.activated`, `prompt.created`
     - `mcp_server.updated`, `trust_rule.created`, etc.
   - Payload is a lightweight delta (resource type, ID, action, timestamp) — not the full resource
   - Clients can filter by event type via query parameter: `?events=agent.updated,prompt.activated`

2. **SSE Health Stream**
   - `GET /api/v1/stream/health` — SSE endpoint for live MCP server health status
   - Emits health check results as they occur (if gateway mode is enabled)
   - Includes circuit breaker state changes (closed → open → half-open)
   - Admin GUI connects to show real-time health indicators

3. **Progressive Discovery**
   - `GET /api/v1/discovery?stream=true` — SSE variant of the discovery endpoint
   - Streams configuration sections progressively: agents first, then prompts, then MCP servers, etc.
   - Allows BFF to start processing agents while prompts are still loading
   - Includes a final `complete` event signaling all data has been sent

4. **Admin GUI Live Updates**
   - Connect the React admin GUI to the SSE change stream
   - Auto-refresh data tables when relevant resources change
   - Show toast notifications for changes made by other users
   - Display live MCP server health badges (green/yellow/red)

5. **WebSocket Support (Future-Proofing)**
   - Add WebSocket endpoint at `/ws/v1` as an alternative to SSE
   - Bidirectional: clients can subscribe/unsubscribe to specific event channels
   - Required for future A2A protocol support (A2A plans WebSocket transport)

### Technical Considerations

- SSE is simpler than WebSocket for server-to-client push — start with SSE, add WebSocket later
- Use Go's `http.Flusher` interface for SSE (well-supported in chi/net/http)
- Fan-out via in-memory pub/sub (Go channels); no external message broker needed at our scale
- SSE connections must respect authentication (session cookie or API key)
- Set appropriate `Cache-Control: no-cache` and `Connection: keep-alive` headers
- Implement heartbeat (`:keep-alive` comment every 30s) to detect stale connections
- SSE connections count against rate limits differently (1 connection, not 1-per-event)

### Architecture

```
Admin GUI / BFF                    Agentic Registry
     │                                    │
     │  GET /api/v1/stream/changes        │
     │  Accept: text/event-stream         │
     │ ──────────────────────────────────> │
     │                                    │
     │  event: agent.updated              │
     │  data: {"id":"pmo","version":4}    │
     │ <────────────────────────────────── │
     │                                    │
     │  event: prompt.activated           │
     │  data: {"agent_id":"pmo","v":7}    │
     │ <────────────────────────────────── │
     │                                    │
     │  :keep-alive                       │
     │ <────────────────────────────────── │  (every 30s)
     │                                    │
```

### Acceptance Criteria

- [ ] SSE change stream emits events within 1 second of a mutation
- [ ] Admin GUI auto-updates when config changes (no manual refresh)
- [ ] MCP server health status is reflected in real-time in the GUI
- [ ] Progressive discovery streams sections and signals completion
- [ ] SSE connections authenticate and respect role-based access
- [ ] Heartbeats keep connections alive through proxies/load balancers
- [ ] Graceful degradation: GUI falls back to polling if SSE disconnects

### Competitive Context

| Competitor | Real-Time Support |
|---|---|
| MCPJungle | SSE (MCP transport) |
| Kong MCP Registry | SSE + WebSocket (commercial) |
| A2A Registry | Planned WebSocket transport |
| Official MCP Registry | None (REST only) |
| **Us (current)** | **None (REST + webhooks only)** |
| **Us (after this item)** | **SSE for changes, health, and progressive discovery** |

### Dependencies

- Benefits from 003 (MCP Gateway Mode) for health stream data
- Independent for config change streaming

### References

- [MDN: Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)
- [MCP Transports Specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports)
