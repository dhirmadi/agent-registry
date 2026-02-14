CREATE TABLE agents (
    id              VARCHAR(50) PRIMARY KEY,
    name            VARCHAR(200) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    system_prompt   TEXT NOT NULL DEFAULT '',
    tools           JSONB NOT NULL DEFAULT '[]',
    trust_overrides JSONB NOT NULL DEFAULT '{}',
    example_prompts JSONB NOT NULL DEFAULT '[]',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    version         INT NOT NULL DEFAULT 1,
    created_by      VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agent_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        VARCHAR(50) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version         INT NOT NULL,
    name            VARCHAR(200) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    system_prompt   TEXT NOT NULL DEFAULT '',
    tools           JSONB NOT NULL DEFAULT '[]',
    trust_overrides JSONB NOT NULL DEFAULT '{}',
    example_prompts JSONB NOT NULL DEFAULT '[]',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_by      VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(agent_id, version)
);

CREATE INDEX idx_agent_versions_agent ON agent_versions(agent_id, version DESC);
