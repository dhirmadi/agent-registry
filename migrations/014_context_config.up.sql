CREATE TABLE context_config (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope            VARCHAR(20) NOT NULL CHECK (scope IN ('global', 'workspace')),
    scope_id         VARCHAR(100) NOT NULL DEFAULT '',
    max_total_tokens INT NOT NULL DEFAULT 18000,
    layer_budgets    JSONB NOT NULL DEFAULT '{"workspace_structure": 500, "ui_context": 200, "semantic_retrieval": 4000, "file_content": 8000, "conversation_memory": 4000, "domain_state": 1000}',
    enabled_layers   JSONB NOT NULL DEFAULT '["workspace_structure", "ui_context", "semantic_retrieval", "file_content", "conversation_memory", "domain_state"]',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(scope, scope_id)
);

INSERT INTO context_config (scope, scope_id) VALUES ('global', '');
