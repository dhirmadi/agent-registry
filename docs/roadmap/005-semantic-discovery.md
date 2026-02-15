# 005 — Semantic & Capability-Based Discovery

> **Priority:** Medium
> **Phase:** 7.2
> **Effort:** Medium (2–3 weeks)
> **Status:** Planned

---

## Problem

Our current discovery is entirely ID-based and exact-match. To find an agent, you need to know its ID (`pmo`, `router`, `code-reviewer`). There is no way to ask:

- "Find me an agent that can manage calendar events"
- "Which agents have access to Git tools?"
- "Show agents that can process meeting transcripts"

The A2A Registry, AGNTCY Agent Directory, and the ACP Registry all support semantic search over agent capabilities using natural language queries and capability taxonomies. As the number of agents grows beyond a handful, semantic discovery becomes essential for both humans and other agents.

## Solution

Add a semantic search layer on top of our existing agent and tool data, enabling natural language queries over agent capabilities.

### Deliverables

1. **Capability Index**
   - Build a structured capability index from existing data:
     - Agent `description` and `name`
     - Tool names and descriptions across all agents
     - Prompt content (system prompts describe what the agent does)
     - Example prompts (describe what users can ask)
   - Store as a searchable index updated on every agent/prompt mutation

2. **Embedding-Based Search**
   - Generate embeddings for agent capabilities using a configurable embedding model
   - Store embeddings in PostgreSQL using `pgvector` extension
   - `GET /api/v1/agents/search?q=manage+calendar+events` — returns ranked agents by semantic similarity
   - Support both semantic and keyword fallback search

3. **Skill Taxonomy**
   - Define a skill taxonomy for categorizing agents:
     - Categories: `code`, `communication`, `project-management`, `data`, `security`, `devops`, etc.
     - Auto-classify agents based on their tools and description
     - Manual override via new `skills` field on Agent model
   - `GET /api/v1/agents?skill=communication` — filter by skill category

4. **Capability Matching API**
   - `POST /api/v1/agents/match` — given a set of required capabilities (tool names, skill categories, or natural language description), return the best matching agents
   - Useful for agent orchestrators (like a router agent) to dynamically select the right agent for a task

5. **Admin GUI: Search Enhancement**
   - Add a search bar to the Agents page with natural language query support
   - Show matched capabilities highlighted in search results
   - Add skill tags to agent cards in the UI

### Technical Considerations

- **pgvector** is the recommended PostgreSQL extension for vector similarity search — avoids adding a separate vector DB
- Embedding generation can use the model configured in `ModelConfig.embedding_model` (currently `nomic-embed-text:latest`)
- Embeddings are regenerated on agent/prompt update (async, via background job)
- For deployments without an embedding model available, fall back to PostgreSQL full-text search (`tsvector` + `ts_rank`)
- Index size: with <100 agents, even naive approaches are fast; design for 10,000+ agents

### Acceptance Criteria

- [ ] `GET /api/v1/agents/search?q=git+operations` returns agents with Git-related tools
- [ ] Semantic search ranks results by relevance, not just keyword match
- [ ] Skill taxonomy auto-classifies agents on create/update
- [ ] Capability matching API returns correct agent for a given set of requirements
- [ ] Search works with full-text fallback when no embedding model is configured
- [ ] Admin GUI search bar supports natural language queries

### Competitive Context

| Competitor | Semantic Discovery |
|---|---|
| A2A Registry | Skill-based search, Agent Cards |
| AGNTCY | IPFS-based semantic discovery |
| ACP Registry | Capability search with Ed25519 identity |
| Official MCP Registry | Keyword search only |
| **Us (current)** | **ID-based exact match only** |
| **Us (after this item)** | **Embedding-based semantic search + skill taxonomy** |

### Dependencies

- pgvector PostgreSQL extension (new migration)
- Optional: embedding model endpoint (falls back to full-text search)

### References

- [pgvector](https://github.com/pgvector/pgvector)
- [A2A Agent Discovery](https://a2a-protocol.org/latest/topics/agent-discovery/)
- [ACP Registry](https://registry.asabove.tech/)
