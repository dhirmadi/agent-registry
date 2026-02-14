CREATE TABLE model_config (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope                     VARCHAR(20) NOT NULL CHECK (scope IN ('global', 'workspace', 'user')),
    scope_id                  VARCHAR(100) NOT NULL DEFAULT '',
    default_model             VARCHAR(100) NOT NULL DEFAULT 'qwen3:8b',
    temperature               NUMERIC(3,2) NOT NULL DEFAULT 0.70,
    max_tokens                INT NOT NULL DEFAULT 8192,
    max_tool_rounds           INT NOT NULL DEFAULT 10,
    default_context_window    INT NOT NULL DEFAULT 128000,
    default_max_output_tokens INT NOT NULL DEFAULT 8192,
    history_token_budget      INT NOT NULL DEFAULT 4000,
    max_history_messages      INT NOT NULL DEFAULT 20,
    embedding_model           VARCHAR(100) NOT NULL DEFAULT 'nomic-embed-text:latest',
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(scope, scope_id)
);
INSERT INTO model_config (scope, scope_id) VALUES ('global', '');
