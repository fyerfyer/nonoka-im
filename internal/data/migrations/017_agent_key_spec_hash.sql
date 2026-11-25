-- +goose Up
ALTER TABLE agent_gateway_keys ADD COLUMN IF NOT EXISTS spec_hash varchar(64) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agent_gateway_keys DROP COLUMN IF EXISTS spec_hash;
