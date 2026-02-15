# 009 — Model Endpoints

> **Priority:** High
> **Phase:** 7.5 (Asset Maturity)
> **Effort:** Medium (2–3 weeks)
> **Status:** Planned

---

## Problem

The current `Model Config` resource is a single global settings blob (temperature, max tokens, default model, etc.). This creates three problems:

1. **Not a real asset.** It's configuration *about* a model, not a model resource itself. There's no URL, no endpoint address, no connection contract. Consumers can't pull "which model endpoint should I use" from the registry.

2. **Not versionable.** Changing the temperature from 0.7 to 0.3 overwrites the previous value. There's no history, no rollback, no ability to pin a consumer to a specific configuration snapshot. This contradicts the registry's core value proposition — versioned, auditable configuration.

3. **Not composable.** A single global config can't represent the reality that an organization uses multiple model endpoints (OpenAI, Azure OpenAI, Ollama, Anthropic) with different parameters per use case.

Meanwhile, model endpoints are a *real asset* that changes over time: providers deprecate models, URLs rotate, rate limits shift, and teams need to A/B test configurations. Consumers need a stable registry entry they can resolve at runtime.

## Solution

Replace the global `Model Config` with a first-class **Model Endpoints** resource — a versionable, addressable registry artifact that represents a model provider endpoint with its full connection and configuration contract.

### Data Model

```
model_endpoints
├── id                  UUID
├── slug                string (unique, human-readable: "openai-gpt4o", "azure-east-gpt4")
├── name                string ("GPT-4o Production", "Azure East US GPT-4")
├── provider            enum (openai, azure, anthropic, ollama, custom)
├── endpoint_url        string (base URL for the model API)
├── is_fixed_model      boolean
│   ├── true  → model_name is locked (e.g., "gpt-4o-2024-08-06")
│   └── false → consumers may select from allowed_models[]
├── model_name          string (default/fixed model)
├── allowed_models      string[] (when is_fixed_model=false)
├── is_active           boolean
├── workspace_id        string (nullable, for scoped endpoints)
├── created_at          timestamp
├── updated_at          timestamp
└── created_by          UUID

model_endpoint_versions
├── id                  UUID
├── endpoint_id         UUID → model_endpoints.id
├── version             integer (auto-increment per endpoint)
├── config              jsonb
│   ├── temperature     float
│   ├── max_tokens      integer
│   ├── max_output_tokens integer
│   ├── top_p           float
│   ├── frequency_penalty float
│   ├── presence_penalty  float
│   ├── context_window  integer
│   ├── history_token_budget integer
│   ├── max_history_messages integer
│   ├── max_tool_rounds integer
│   ├── headers         map (custom headers, encrypted at rest)
│   └── metadata        map (arbitrary key-value)
├── is_active           boolean (only one active version per endpoint)
├── change_note         string ("Lowered temperature for deterministic output")
├── created_at          timestamp
└── created_by          UUID
```

### API Design

```
# Endpoints CRUD
GET    /api/v1/model-endpoints                         # List all endpoints
POST   /api/v1/model-endpoints                         # Create endpoint
GET    /api/v1/model-endpoints/{slug}                  # Get endpoint + active version
PUT    /api/v1/model-endpoints/{slug}                  # Update endpoint metadata
DELETE /api/v1/model-endpoints/{slug}                  # Soft-delete endpoint

# Versioning
GET    /api/v1/model-endpoints/{slug}/versions         # List all versions
POST   /api/v1/model-endpoints/{slug}/versions         # Create new version
GET    /api/v1/model-endpoints/{slug}/versions/{v}     # Get specific version
POST   /api/v1/model-endpoints/{slug}/versions/{v}/activate  # Activate a version

# Workspace-scoped
GET    /api/v1/workspaces/{wid}/model-endpoints        # List workspace endpoints
POST   /api/v1/workspaces/{wid}/model-endpoints        # Create workspace-scoped endpoint
```

### Example Payloads

**Creating a fixed-model endpoint:**
```json
{
  "slug": "openai-gpt4o-prod",
  "name": "GPT-4o Production",
  "provider": "openai",
  "endpoint_url": "https://api.openai.com/v1",
  "is_fixed_model": true,
  "model_name": "gpt-4o-2024-08-06"
}
```

**Creating a version with specific config:**
```json
{
  "config": {
    "temperature": 0.3,
    "max_tokens": 4096,
    "max_output_tokens": 8192,
    "context_window": 128000,
    "top_p": 0.95
  },
  "change_note": "Lowered temperature for deterministic summarization"
}
```

**A multi-model endpoint (consumer selects):**
```json
{
  "slug": "azure-east-flexible",
  "name": "Azure East US (Flexible)",
  "provider": "azure",
  "endpoint_url": "https://myorg-east.openai.azure.com",
  "is_fixed_model": false,
  "model_name": "gpt-4o",
  "allowed_models": ["gpt-4o", "gpt-4o-mini", "gpt-4-turbo"]
}
```

### Use Cases

1. **Pinned consumers.** A production agent resolves `openai-gpt4o-prod` at startup. The registry returns the active version's config. When the team creates version 6 with lower temperature, nothing changes for the agent until that version is activated.

2. **A/B testing.** Two versions of the same endpoint exist with different temperatures. Operators activate version A, measure quality, then switch to version B — all without touching application code.

3. **Provider migration.** The team moves from OpenAI to Azure. They create a new endpoint `azure-gpt4o-prod`, point consumers to the new slug, and decommission the old one. Full audit trail.

4. **Multi-model gateway.** An endpoint with `is_fixed_model: false` lets consumers select from an approved model list. The registry enforces which models are allowed.

### Migration Path

| Current | After |
|---|---|
| `GET /api/v1/model-config` (single global blob) | `GET /api/v1/model-endpoints` (list of real assets) |
| `PUT /api/v1/model-config` (overwrite in place) | `POST /api/v1/model-endpoints/{slug}/versions` (new version, old preserved) |
| No versioning | Full version history with rollback |
| No endpoint URL | Real provider endpoint URL per entry |
| Single config for all consumers | Multiple endpoints, scoped by workspace |

The existing `Model Config` endpoint can be preserved as a deprecated compatibility shim that reads from a designated "default" model endpoint, easing migration.

### Discovery Integration

The discovery endpoint (`GET /api/v1/discovery`) will include model endpoints in its response:

```json
{
  "model_endpoints": [
    {
      "slug": "openai-gpt4o-prod",
      "name": "GPT-4o Production",
      "provider": "openai",
      "endpoint_url": "https://api.openai.com/v1",
      "model_name": "gpt-4o-2024-08-06",
      "active_version": 5,
      "config": { "temperature": 0.3, "max_tokens": 4096 }
    }
  ]
}
```

### Acceptance Criteria

- [ ] CRUD operations for model endpoints with slug-based addressing
- [ ] Version management: create, list, get, activate, with only one active version per endpoint
- [ ] Fixed-model vs. flexible-model endpoint types
- [ ] Workspace-scoped endpoints with proper isolation
- [ ] Config stored as versioned JSONB with full audit trail
- [ ] Sensitive fields (custom headers, API keys in metadata) encrypted at rest
- [ ] Discovery endpoint includes model endpoints
- [ ] Admin GUI page for managing endpoints and versions (list, create, version history, activate/rollback)
- [ ] Migration shim: existing `model-config` endpoint continues to work during transition
- [ ] All mutations produce audit log entries and webhook notifications

### Dependencies

- None (independent of other roadmap items)
- Benefits from [004 Multi-Tenancy](004-multi-tenancy.md) for tenant-scoped endpoints

### Competitive Context

| Solution | Model Endpoint Registry |
|---|---|
| LiteLLM | Proxy with model aliasing, no versioning |
| Portkey | Gateway with config management, commercial |
| OpenRouter | Model directory, no self-hosted, no versioning |
| Kong AI Gateway | Route-level model config, commercial |
| **Us (current)** | **Single global config blob, not versioned** |
| **Us (after this item)** | **Full versioned model endpoint registry with rollback** |

No open-source solution currently offers a **versioned, self-hosted model endpoint registry** with activation/rollback semantics. This is a differentiated capability.
