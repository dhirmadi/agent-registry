# 007 — Advanced Observability & Cost Tracking

> **Priority:** Medium
> **Phase:** 7.4
> **Effort:** Medium (2–3 weeks)
> **Status:** Planned

---

## Problem

Our current observability stack includes OpenTelemetry traces and Prometheus metrics for the registry server itself, but we lack visibility into what matters most to operators:

- **Per-tool usage analytics** — which tools are called most, by whom, how often
- **Token consumption tracking** — how many tokens are agents consuming (via model config settings)
- **Cost attribution** — which teams/workspaces/agents are driving LLM costs
- **Health dashboards** — circuit breaker state, MCP server uptime, error rates over time
- **Performance trends** — API latency percentiles, slow queries, resource bottlenecks

Competitors like Kong provide token-based rate limiting and cost dashboards. MCPJungle ships OpenTelemetry integration with Prometheus metrics per tool. We have the foundation but not the domain-specific analytics.

## Solution

Build a domain-aware observability layer that tracks agent and tool usage, attributes costs, and provides actionable dashboards in the admin GUI.

### Deliverables

1. **Usage Analytics Table**
   - New `usage_stats` table tracking per-day aggregates:
     - `date`, `agent_id`, `tool_name`, `mcp_server_label`, `user_id`, `tenant_id`
     - `call_count`, `success_count`, `failure_count`
     - `total_latency_ms`, `avg_latency_ms`, `p95_latency_ms`
     - `estimated_input_tokens`, `estimated_output_tokens`
   - Populated from audit log entries (batch aggregation) or real-time if gateway mode is enabled

2. **Cost Attribution**
   - Configurable cost-per-token rates per model (stored in `model_config` or new `cost_config` table)
   - Calculate estimated costs from token usage × rate
   - Attribute costs to: agent, workspace/tenant, user, MCP server
   - `GET /api/v1/analytics/costs?period=30d&group_by=agent` — cost breakdown API

3. **Prometheus Metrics Enhancement**
   - Domain-specific metrics (in addition to standard Go/HTTP metrics):
     - `registry_agent_requests_total{agent_id, method}` — requests per agent
     - `registry_tool_calls_total{tool, server, status}` — tool calls through gateway
     - `registry_tool_call_duration_seconds{tool, server}` — tool call latency histogram
     - `registry_circuit_breaker_state{server}` — current circuit state gauge
     - `registry_active_sse_connections` — active streaming connections
     - `registry_webhook_deliveries_total{status, event_type}` — webhook delivery counters
     - `registry_estimated_token_usage{model, agent_id}` — token consumption counter

4. **Admin GUI: Analytics Dashboard**
   - New dashboard page (or enhanced existing Dashboard):
     - **Top agents by usage** — bar chart, last 7/30 days
     - **Tool call heatmap** — which tools are called when
     - **MCP server health timeline** — uptime/downtime over time with circuit breaker events
     - **Cost trend** — estimated LLM costs over time, grouped by agent or workspace
     - **Error rate trends** — tool failures, webhook delivery failures
   - Use lightweight charting library (PatternFly charts or recharts)

5. **Usage Alerts**
   - Configurable thresholds for cost and usage alerts:
     - "Alert when daily token usage exceeds X"
     - "Alert when tool error rate exceeds Y%"
     - "Alert when MCP server circuit breaker opens"
   - Deliver alerts via existing webhook system (dispatch to Slack/email/PagerDuty)

### Technical Considerations

- Usage stats are append-only aggregates — never update historical data
- Daily aggregation job runs at midnight UTC (or configurable)
- For real-time gateway metrics, use Prometheus counters/histograms directly
- Token estimation is approximate (based on model config's `max_tokens` and actual response sizes, not actual tokenizer counts)
- Keep analytics data retention configurable (default: 90 days, then rollup to monthly)
- Charts in the GUI should load quickly — pre-aggregate data server-side, don't push raw data to the browser

### Acceptance Criteria

- [ ] Per-agent, per-tool usage statistics are collected and queryable
- [ ] Cost estimates are computed and attributable to agents and workspaces
- [ ] Prometheus metrics include domain-specific counters and histograms
- [ ] Admin GUI shows analytics dashboard with charts
- [ ] Usage alerts fire through the webhook system when thresholds are exceeded
- [ ] Analytics data is retained for 90 days by default

### Competitive Context

| Competitor | Observability |
|---|---|
| Kong MCP Registry | Token rate limiting, cost dashboards, full APM (commercial) |
| MCPJungle | OpenTelemetry + Prometheus per-tool metrics |
| Microsoft Entra Agent ID | Azure Monitor integration |
| **Us (current)** | **OTel traces + Prometheus basics + audit log** |
| **Us (after this item)** | **Full usage analytics, cost attribution, health dashboards, alerts** |

### Dependencies

- Benefits greatly from 003 (MCP Gateway Mode) for per-tool-call data
- Can partially implement with audit log data alone (without gateway)

### References

- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [Prometheus Go Client](https://github.com/prometheus/client_golang)
- [Kong MCP Governance](https://konghq.com/solutions/mcp-governance)
