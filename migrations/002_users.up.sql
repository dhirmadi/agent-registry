CREATE TABLE users (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username         VARCHAR(100) NOT NULL UNIQUE,
    email            VARCHAR(255) NOT NULL UNIQUE,
    display_name     VARCHAR(200) NOT NULL DEFAULT '',
    password_hash    TEXT NOT NULL DEFAULT '',
    role             VARCHAR(20) NOT NULL DEFAULT 'viewer'
                     CHECK (role IN ('admin', 'editor', 'viewer')),
    auth_method      VARCHAR(20) NOT NULL DEFAULT 'password'
                     CHECK (auth_method IN ('password', 'google', 'both')),
    is_active        BOOLEAN NOT NULL DEFAULT true,
    must_change_pass BOOLEAN NOT NULL DEFAULT false,
    failed_logins    INT NOT NULL DEFAULT 0,
    locked_until     TIMESTAMPTZ,
    last_login_at    TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_username ON users(username) WHERE is_active = true;
CREATE INDEX idx_users_email ON users(email) WHERE is_active = true;
