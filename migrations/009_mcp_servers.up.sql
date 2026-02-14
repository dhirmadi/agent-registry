CREATE TABLE mcp_servers (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label              VARCHAR(100) NOT NULL UNIQUE,
    endpoint           TEXT NOT NULL,
    auth_type          VARCHAR(20) NOT NULL DEFAULT 'none'
                       CHECK (auth_type IN ('none', 'bearer', 'basic')),
    auth_credential    TEXT NOT NULL DEFAULT '',
    health_endpoint    TEXT NOT NULL DEFAULT '',
    circuit_breaker    JSONB NOT NULL DEFAULT '{"fail_threshold": 5, "open_duration_s": 30}',
    discovery_interval VARCHAR(20) NOT NULL DEFAULT '5m',
    is_enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
