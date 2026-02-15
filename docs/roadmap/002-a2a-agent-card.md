# 002 — A2A Agent Card & Interoperability

> **Priority:** High
> **Phase:** 6.2
> **Effort:** Medium (2 weeks)
> **Status:** Planned

---

## Problem

Google's Agent-to-Agent (A2A) protocol, announced April 2025, has been adopted by 50+ technology partners (Atlassian, Salesforce, SAP, ServiceNow, Deloitte, McKinsey). It is becoming the standard for how agents discover and collaborate with each other across enterprise platforms. A2A v0.3.0 defines an Agent Card — a JSON metadata manifest describing an agent's identity, capabilities, skills, endpoints, and authentication requirements.

Our agents are invisible to the A2A ecosystem. No external agent can discover what agents we host, what they can do, or how to invoke them.

## Solution

Implement A2A Agent Card publishing so that our registered agents are discoverable by any A2A-aware system.

### Deliverables

1. **Well-Known Agent Card Endpoint**
   - Serve `GET /.well-known/agent.json` returning a composite Agent Card for the registry itself
   - The card describes the registry as a "meta-agent" that manages other agents
   - Include skills listing derived from our registered agents

2. **Per-Agent Agent Cards**
   - Serve `GET /api/v1/agents/{agentId}/agent-card` returning an A2A-compliant Agent Card for each registered agent
   - Map our agent model fields to A2A schema:
     - `name` → A2A `name`
     - `description` → A2A `description`
     - `tools` → A2A `skills[].capabilities`
     - `example_prompts` → A2A `skills[].examples`
     - `is_active` → A2A `provider.status`

3. **A2A Registry Publish (Optional Push)**
   - Add configuration option to automatically publish agent cards to an external A2A Registry instance
   - Support `POST` to a configurable A2A Registry URL on agent create/update
   - Include retry logic (reuse existing webhook dispatcher pattern)

4. **A2A Discovery Index**
   - Serve `GET /api/v1/agents/a2a-index` returning a list of all active agent cards
   - Support query parameters for filtering by capability or skill keyword

### A2A Agent Card Schema Mapping

```json
{
  "name": "pmo",
  "description": "Project Management Officer agent",
  "url": "https://registry.example.com/api/v1/agents/pmo",
  "provider": {
    "organization": "Agentic Registry",
    "url": "https://registry.example.com"
  },
  "version": "3",
  "capabilities": {
    "streaming": false,
    "pushNotifications": false
  },
  "skills": [
    {
      "id": "project-management",
      "name": "Project Management",
      "description": "Manages tasks, sprints, and project tracking",
      "examples": ["Create a new sprint", "Show my open tasks"]
    }
  ],
  "authentication": {
    "schemes": ["bearer"]
  }
}
```

### Technical Considerations

- Agent Cards are static JSON — cache aggressively with `ETag` / `If-None-Match`
- Invalidate cache on agent update (reuse our `updated_at` ETag pattern)
- The `/.well-known/agent.json` path must be served before the SPA catch-all
- Consider supporting A2A's planned protocols: JSON-RPC 2.0 and WebSocket (future roadmap item)

### Acceptance Criteria

- [ ] `GET /.well-known/agent.json` returns a valid A2A Agent Card
- [ ] `GET /api/v1/agents/{id}/agent-card` returns a per-agent card conforming to A2A v0.3.0 schema
- [ ] Agent cards update automatically when agents are modified
- [ ] External A2A Registry can discover our agents via well-known endpoint
- [ ] Authentication is documented in the card's `authentication` block

### Competitive Context

| Competitor | A2A Support |
|---|---|
| Google A2A Registry | Native (defines the standard) |
| Microsoft Entra Agent ID | Agent Cards + enterprise directory |
| AGNTCY | Decentralized agent directory |
| **Us (current)** | **None** |
| **Us (after this item)** | **Per-agent A2A cards + well-known discovery** |

### Dependencies

- None (can be built independently)

### References

- [A2A Protocol Specification](https://a2a-protocol.org/)
- [A2A Registry Documentation](https://a2a-registry.dev/documentation/)
- [Google A2A Announcement](https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability)
