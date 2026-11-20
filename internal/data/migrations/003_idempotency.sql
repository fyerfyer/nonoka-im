-- +goose Up
-- Idempotency-Key storage for mutating API requests. Keys are scoped to the
-- authenticated user and a canonical request fingerprint; a replay returns
-- the stored response instead of executing the operation twice.

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id bigserial PRIMARY KEY,
    scope_user_id bigint NOT NULL,
    key varchar(255) NOT NULL,
    fingerprint varchar(64) NOT NULL,
    status varchar(16) NOT NULL,
    response_code integer,
    response_body text,
    locked_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_user_key ON idempotency_keys (scope_user_id, key);
CREATE INDEX IF NOT EXISTS idx_idempotency_created_at ON idempotency_keys (created_at);

-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;
