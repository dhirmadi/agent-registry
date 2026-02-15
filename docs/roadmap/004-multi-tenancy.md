# 004 — Multi-Tenancy & Federation

> **Priority:** Medium
> **Phase:** 7.1
> **Effort:** Large (4–5 weeks)
> **Status:** Planned

---

## Problem

Our registry is single-tenant. All agents, prompts, MCP servers, and configuration live in one flat namespace. This works for a single team but becomes a liability as adoption grows:

- Multiple teams sharing one registry cannot isolate their agents or configurations
- There is no way to run a "staging" and "production" registry with config promotion between them
- Enterprises running multiple business units need per-tenant data isolation
- The market (Microsoft Entra Agent ID, Kong, Azure API Center) is converging on multi-tenant models

Our workspace-scoped resources (trust rules, triggers, model config, context config) already hint at multi-tenancy but don't deliver true tenant isolation at the data layer.

## Solution

Introduce first-class tenant (organization) support with data isolation, and add federation capabilities for syncing configurations between registry instances.

### Deliverables

1. **Tenant Model**
   - New `tenants` table: `id`, `name`, `slug`, `is_active`, `created_at`
   - Every resource gains a `tenant_id` foreign key (nullable for backward compatibility; default tenant auto-created on migration)
   - Tenant resolution via subdomain (`acme.registry.example.com`) or header (`X-Tenant-ID`)
   - System-wide resources (trust defaults, global model config) are tenant-scoped

2. **Tenant Isolation**
   - All store queries include `WHERE tenant_id = $N` filter
   - Row-level security (RLS) in PostgreSQL as defense-in-depth
   - Cross-tenant data access is impossible at the DB layer
   - API keys are scoped to a tenant
   - Users can belong to multiple tenants with per-tenant roles

3. **Tenant Administration**
   - `POST /api/v1/tenants` — create tenant (super-admin only)
   - `GET /api/v1/tenants` — list tenants
   - `PUT /api/v1/tenants/{tenantId}` — update tenant settings
   - `DELETE /api/v1/tenants/{tenantId}` — soft-delete tenant
   - Admin GUI: tenant switcher in the top navigation bar

4. **Config Promotion (Federation Lite)**
   - Export a tenant's full configuration as a portable JSON bundle
   - `GET /api/v1/tenants/{tenantId}/export` — export all agents, prompts, MCP servers, rules
   - `POST /api/v1/tenants/{tenantId}/import` — import a bundle, with conflict resolution (skip/overwrite/rename)
   - Enables staging → production promotion workflow

5. **Cross-Instance Federation (Future)**
   - Registry-to-registry sync via webhook or pull-based replication
   - Designate a "source of truth" registry and downstream "read replicas"
   - Conflict detection via version numbers and `updated_at` timestamps

### Migration Strategy

- Add `tenant_id` column to all resource tables with `DEFAULT` pointing to an auto-created `default` tenant
- Existing single-tenant deployments continue working with zero configuration change
- Multi-tenancy activates only when `MULTI_TENANT=true` env var is set
- Backward-compatible: single-tenant mode skips all tenant resolution logic

### Technical Considerations

- Tenant resolution middleware runs before auth middleware
- PostgreSQL RLS policies are applied per-tenant; application code acts as secondary enforcement
- Connection pooling is shared across tenants (not per-tenant pools — too expensive at small scale)
- Audit log entries include `tenant_id` for tenant-scoped audit views
- Discovery endpoint returns data for the resolved tenant only

### Acceptance Criteria

- [ ] Two tenants can exist with identically-named agents without conflict
- [ ] Users see only their tenant's data in the admin GUI
- [ ] API keys are scoped to a single tenant
- [ ] Config export/import round-trips without data loss
- [ ] Existing single-tenant deployments upgrade seamlessly with no config change
- [ ] Audit log shows tenant context for all entries

### Competitive Context

| Competitor | Multi-Tenancy |
|---|---|
| Microsoft Entra Agent ID | Full (per-organization directories) |
| Kong MCP Registry | Full (per-org within Konnect platform) |
| Azure API Center | Full (per-subscription) |
| MCPJungle | None (single instance) |
| **Us (current)** | **None (workspace hints only)** |
| **Us (after this item)** | **Full tenant isolation + config promotion** |

### Dependencies

- None (can be built independently, but benefits from being done before 005–008)

### References

- [Microsoft Entra Agent Registry](https://learn.microsoft.com/en-us/entra/agent-id/identity-platform/what-is-agent-registry)
- [Azure API Center MCP Integration](https://learn.microsoft.com/en-us/azure/api-center/register-discover-mcp-server)
