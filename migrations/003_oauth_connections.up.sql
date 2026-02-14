CREATE TABLE oauth_connections (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      VARCHAR(20) NOT NULL DEFAULT 'google',
    provider_uid  VARCHAR(200) NOT NULL,
    email         VARCHAR(255) NOT NULL,
    display_name  VARCHAR(200) NOT NULL DEFAULT '',
    access_token  TEXT NOT NULL DEFAULT '',
    refresh_token TEXT NOT NULL DEFAULT '',
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_uid)
);

CREATE INDEX idx_oauth_user ON oauth_connections(user_id);
CREATE INDEX idx_oauth_provider ON oauth_connections(provider, provider_uid);
