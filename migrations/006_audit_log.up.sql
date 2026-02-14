CREATE TABLE audit_log (
    id            BIGSERIAL PRIMARY KEY,
    actor         VARCHAR(200) NOT NULL,
    actor_id      UUID,
    action        VARCHAR(50) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id   VARCHAR(200) NOT NULL DEFAULT '',
    details       JSONB NOT NULL DEFAULT '{}',
    ip_address    INET,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_time ON audit_log(created_at DESC);
CREATE INDEX idx_audit_log_actor ON audit_log(actor);
CREATE INDEX idx_audit_log_resource ON audit_log(resource_type, resource_id);
