-- +goose Up
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS trace_id varchar(64) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agent_runs DROP COLUMN IF EXISTS trace_id;
