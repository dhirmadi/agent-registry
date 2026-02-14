CREATE TABLE webhook_subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url        TEXT NOT NULL,
    secret     TEXT NOT NULL DEFAULT '',
    events     JSONB NOT NULL DEFAULT '[]',
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
