# 008 — Package & Distribution Ecosystem

> **Priority:** Low
> **Phase:** 8.1
> **Effort:** Medium (3 weeks)
> **Status:** Planned

---

## Problem

Our agent configurations are locked inside the registry with no portable format. There is no way to:

- Share an agent definition between two registry instances
- Version-control agent configurations outside the registry (e.g., in Git)
- Publish a reusable "agent template" for the community
- Import community-built agent configurations
- Back up and restore specific agents without full database dumps

The official MCP Registry supports distributing servers as npm, PyPI, Docker, and NuGet packages with ownership verification. The A2A ecosystem uses Agent Cards as portable descriptors. We have no equivalent.

## Solution

Define a portable agent package format and build import/export tooling around it.

### Deliverables

1. **Agent Package Format (`.agentpkg.json`)**
   - A self-contained JSON file containing everything needed to recreate an agent:
     ```json
     {
       "format_version": "1.0",
       "agent": { /* agent definition */ },
       "prompts": [ /* all prompt versions */ ],
       "trust_overrides": { /* agent-level overrides */ },
       "tools": [ /* tool definitions with MCP server references */ ],
       "mcp_servers": [ /* referenced MCP server configs (without credentials) */ ],
       "metadata": {
         "exported_from": "registry.example.com",
         "exported_at": "2026-02-14T12:00:00Z",
         "exported_by": "admin",
         "checksum": "sha256:..."
       }
     }
     ```
   - Credentials are **never** included — MCP server entries include endpoint and auth_type but not auth_credential

2. **Export API**
   - `GET /api/v1/agents/{agentId}/export` — export a single agent as `.agentpkg.json`
   - `GET /api/v1/export` — export all agents as a combined package
   - Include options: `?include_prompts=true&include_mcp_servers=true`

3. **Import API**
   - `POST /api/v1/agents/import` — import an `.agentpkg.json` file
   - Conflict resolution strategies:
     - `skip` — skip agents that already exist
     - `overwrite` — replace existing agent with imported version
     - `rename` — append suffix to imported agent ID
   - Validate package integrity (checksum verification)
   - Log import as audit event with full details

4. **Admin GUI: Import/Export**
   - Export button on Agent detail page and Agents list page
   - Import dialog with file upload, conflict resolution picker, and preview
   - Show import results: created, skipped, overwritten, errors

5. **CLI Tool (Optional)**
   - Standalone CLI binary for headless import/export:
     ```bash
     registry-cli export --agent pmo --output pmo.agentpkg.json
     registry-cli import --file pmo.agentpkg.json --conflict-strategy overwrite
     ```
   - Authenticates via API key

6. **Git-Based Config Management (Future)**
   - Watch a Git repository for `.agentpkg.json` files
   - Auto-import on push (GitOps pattern)
   - Enables version-controlled agent configuration outside the registry

### Technical Considerations

- Package format is JSON for human readability and Git-friendliness
- Checksum covers the entire payload (minus the checksum field itself) for integrity
- MCP server references are by `label` — import resolves to existing servers or creates stubs
- Prompt versions are ordered by version number; import preserves order
- Large packages (many agents) should support streaming JSON parsing
- Consider adding a `dependencies` field for declaring required MCP servers

### Acceptance Criteria

- [ ] Exporting an agent produces a valid `.agentpkg.json` file
- [ ] Importing a package into a fresh registry recreates the agent exactly
- [ ] Round-trip export → import preserves all data (except credentials)
- [ ] Conflict resolution works correctly for all three strategies
- [ ] Import validates checksum and rejects tampered packages
- [ ] Admin GUI supports file upload import with preview

### Competitive Context

| Competitor | Package/Distribution |
|---|---|
| Official MCP Registry | npm, PyPI, Docker, NuGet packages |
| A2A Registry | Agent Card JSON manifests |
| Agenta | Prompt export/import (limited) |
| MCPJungle | None |
| **Us (current)** | **None** |
| **Us (after this item)** | **Portable agent packages with import/export/CLI** |

### Dependencies

- Benefits from 004 (Multi-Tenancy) for cross-tenant import
- Independent for single-tenant use

### References

- [Official MCP Registry Package Types](https://modelcontextprotocol.io/registry/package-types)
- [A2A Agent Cards](https://a2a-protocol.org/latest/topics/agent-discovery/)
