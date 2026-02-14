CREATE TABLE trust_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    tool_pattern VARCHAR(100) NOT NULL,
    tier         VARCHAR(10) NOT NULL CHECK (tier IN ('auto', 'review', 'block')),
    created_by   VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(workspace_id, tool_pattern)
);
CREATE INDEX idx_trust_rules_ws ON trust_rules(workspace_id);
