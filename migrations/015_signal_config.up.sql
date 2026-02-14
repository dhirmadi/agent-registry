CREATE TABLE signal_config (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source        VARCHAR(50) NOT NULL UNIQUE,
    poll_interval VARCHAR(20) NOT NULL DEFAULT '15m',
    is_enabled    BOOLEAN NOT NULL DEFAULT true,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO signal_config (source, poll_interval) VALUES
    ('gmail',    '15m'),
    ('calendar', '1h'),
    ('drive',    '30m'),
    ('slack',    '30s');
