CREATE TABLE prompts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      VARCHAR(50) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version       INT NOT NULL DEFAULT 1,
    system_prompt TEXT NOT NULL,
    template_vars JSONB NOT NULL DEFAULT '{}',
    mode          VARCHAR(30) NOT NULL DEFAULT 'toolcalling_safe'
                  CHECK (mode IN ('rag_readonly', 'toolcalling_safe', 'toolcalling_auto')),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_by    VARCHAR(200) NOT NULL DEFAULT 'system',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(agent_id, version)
);

CREATE INDEX idx_prompts_active ON prompts(agent_id) WHERE is_active = true;
