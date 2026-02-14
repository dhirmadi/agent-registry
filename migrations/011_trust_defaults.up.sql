CREATE TABLE trust_defaults (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tier       VARCHAR(10) NOT NULL CHECK (tier IN ('auto', 'review', 'block')),
    patterns   JSONB NOT NULL DEFAULT '[]',
    priority   INT NOT NULL DEFAULT 100,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO trust_defaults (tier, patterns, priority) VALUES
    ('auto',   '["_read", "_list", "_search", "_get", "read_", "list_", "search_", "get_", "delegate_to_agent"]', 1),
    ('block',  '["_delete", "_send", "_force", "delete_", "send_", "force_"]', 2),
    ('review', '["_write", "_create", "_update", "_commit", "write_", "create_", "update_", "commit_"]', 3);
