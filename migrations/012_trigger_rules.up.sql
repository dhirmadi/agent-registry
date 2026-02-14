CREATE TABLE trigger_rules (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       UUID NOT NULL,
    name               TEXT NOT NULL,
    event_type         TEXT NOT NULL,
    condition          JSONB NOT NULL DEFAULT '{}',
    agent_id           VARCHAR(50) NOT NULL REFERENCES agents(id),
    prompt_template    TEXT NOT NULL DEFAULT '',
    enabled            BOOLEAN NOT NULL DEFAULT true,
    rate_limit_per_hour INT NOT NULL DEFAULT 10,
    schedule           VARCHAR(100) NOT NULL DEFAULT '',
    run_as_user_id     UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trigger_rules_ws_event ON trigger_rules(workspace_id, event_type) WHERE enabled = true;
