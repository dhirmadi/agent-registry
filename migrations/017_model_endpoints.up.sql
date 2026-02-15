CREATE TYPE model_provider AS ENUM ('openai', 'azure', 'anthropic', 'ollama', 'custom');

CREATE TABLE model_endpoints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            VARCHAR(100) NOT NULL UNIQUE,
    name            VARCHAR(200) NOT NULL,
    provider        model_provider NOT NULL,
    endpoint_url    TEXT NOT NULL,
    is_fixed_model  BOOLEAN NOT NULL DEFAULT true,
    model_name      VARCHAR(100) NOT NULL DEFAULT '',
    allowed_models  JSONB NOT NULL DEFAULT '[]',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    workspace_id    VARCHAR(100),
    created_by      VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE model_endpoint_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id     UUID NOT NULL REFERENCES model_endpoints(id) ON DELETE CASCADE,
    version         INT NOT NULL,
    config          JSONB NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT false,
    change_note     TEXT NOT NULL DEFAULT '',
    created_by      VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(endpoint_id, version)
);

CREATE INDEX idx_model_endpoint_versions_endpoint ON model_endpoint_versions(endpoint_id, version DESC);
CREATE INDEX idx_model_endpoint_versions_active ON model_endpoint_versions(endpoint_id) WHERE is_active = true;
CREATE INDEX idx_model_endpoints_workspace ON model_endpoints(workspace_id) WHERE workspace_id IS NOT NULL;
