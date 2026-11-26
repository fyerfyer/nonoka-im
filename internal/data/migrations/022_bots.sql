-- +goose Up
CREATE TABLE IF NOT EXISTS bots (
    id bigserial PRIMARY KEY,
    bot_user_id bigint NOT NULL REFERENCES users(id),
    owner_user_id bigint NOT NULL REFERENCES users(id),
    name varchar(128) NOT NULL,
    description text NOT NULL DEFAULT '',
    status varchar(16) NOT NULL DEFAULT 'active',
    event_subscriptions jsonb NOT NULL DEFAULT '["message.mention","message.direct"]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bots_status_check CHECK (status IN ('active', 'disabled'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bots_bot_user_id ON bots (bot_user_id);
CREATE INDEX IF NOT EXISTS idx_bots_owner_user_id ON bots (owner_user_id);

CREATE TABLE IF NOT EXISTS bot_tokens (
    id varchar(26) PRIMARY KEY,
    bot_id bigint NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    name varchar(100) NOT NULL,
    prefix varchar(24) NOT NULL,
    token_hash bytea NOT NULL,
    scopes jsonb NOT NULL DEFAULT '["bot:send","bot:read"]'::jsonb,
    expires_at timestamptz,
    revoked_at timestamptz,
    last_used_at timestamptz,
    created_by bigint NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_tokens_prefix ON bot_tokens (prefix);
CREATE INDEX IF NOT EXISTS idx_bot_tokens_bot_id ON bot_tokens (bot_id);

CREATE TABLE IF NOT EXISTS bot_webhooks (
    id bigserial PRIMARY KEY,
    bot_id bigint NOT NULL UNIQUE REFERENCES bots(id) ON DELETE CASCADE,
    url text NOT NULL,
    secret_ciphertext bytea NOT NULL,
    secret_prefix varchar(24) NOT NULL,
    events jsonb NOT NULL DEFAULT '["message.mention","message.direct"]'::jsonb,
    enabled boolean NOT NULL DEFAULT true,
    verified_at timestamptz,
    last_success_at timestamptz,
    consecutive_failures integer NOT NULL DEFAULT 0,
    disabled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bot_deliveries (
    id bigserial PRIMARY KEY,
    bot_id bigint NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    event_id varchar(128) NOT NULL,
    event_type varchar(64) NOT NULL,
    topic varchar(64) NOT NULL DEFAULT '',
    msg_id bigint NOT NULL DEFAULT 0,
    status varchar(16) NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    last_status_code integer,
    last_error text NOT NULL DEFAULT '',
    response_snippet text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    delivered_at timestamptz,
    CONSTRAINT bot_deliveries_status_check CHECK (status IN ('pending', 'delivering', 'succeeded', 'failed'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_deliveries_event ON bot_deliveries (bot_id, event_id);
CREATE INDEX IF NOT EXISTS idx_bot_deliveries_due ON bot_deliveries (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_bot_deliveries_bot_created ON bot_deliveries (bot_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS bot_deliveries;
DROP TABLE IF EXISTS bot_webhooks;
DROP TABLE IF EXISTS bot_tokens;
DROP TABLE IF EXISTS bots;
