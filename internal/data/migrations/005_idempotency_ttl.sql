-- +goose Up
-- Idempotency responses are retained for a bounded window and then pruned.
ALTER TABLE idempotency_keys ADD COLUMN IF NOT EXISTS expires_at timestamptz;
UPDATE idempotency_keys SET expires_at = created_at + interval '24 hours' WHERE expires_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_idempotency_expires_at ON idempotency_keys (expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_idempotency_expires_at;
ALTER TABLE idempotency_keys DROP COLUMN IF EXISTS expires_at;
