CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    name         VARCHAR(100) NOT NULL,
    key_prefix   VARCHAR(20) NOT NULL DEFAULT '',
    key_hash     VARCHAR(64) NOT NULL UNIQUE,
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    is_active    BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_hash ON api_keys(key_hash) WHERE is_active = true;
CREATE INDEX idx_api_keys_user ON api_keys(user_id);
