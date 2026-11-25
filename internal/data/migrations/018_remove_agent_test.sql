-- +goose Up
-- The single-draft playground was removed. Test runs were never published
-- agents and must not survive as half-supported rows, so delete them and all
-- dependent execution state before dropping the now-dead columns.

DELETE FROM agent_tool_invocations
WHERE run_id IN (SELECT id FROM agent_runs WHERE kind = 'test');

DELETE FROM agent_run_steps
WHERE run_id IN (SELECT id FROM agent_runs WHERE kind = 'test');

DELETE FROM agent_approvals
WHERE run_id IN (SELECT id FROM agent_runs WHERE kind = 'test');

DELETE FROM agent_checkpoints
WHERE run_id IN (SELECT id FROM agent_runs WHERE kind = 'test');

DELETE FROM agent_usage_events
WHERE run_id IN (SELECT id FROM agent_runs WHERE kind = 'test');

DELETE FROM agent_turns
WHERE run_id IN (SELECT id FROM agent_runs WHERE kind = 'test');

DELETE FROM agent_context_snapshots
WHERE session_id IN (
    SELECT id FROM agent_sessions WHERE topic LIKE 'agent-test-%'
);

DELETE FROM agent_sessions WHERE topic LIKE 'agent-test-%';

DELETE FROM agent_quota_notices WHERE topic LIKE 'agent-test-%';

DELETE FROM agent_runs WHERE kind = 'test';

DROP INDEX IF EXISTS idx_agent_runs_kind_status;

ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS config_snapshot,
    DROP COLUMN IF EXISTS output_text,
    DROP COLUMN IF EXISTS input_text,
    DROP COLUMN IF EXISTS kind;

ALTER TABLE agent_usage_daily
    DROP COLUMN IF EXISTS test_runs;

ALTER TABLE agent_usage_events
    DROP COLUMN IF EXISTS kind;

-- +goose Down
ALTER TABLE agent_usage_events
    ADD COLUMN IF NOT EXISTS kind varchar(16) NOT NULL DEFAULT 'im';

ALTER TABLE agent_usage_daily
    ADD COLUMN IF NOT EXISTS test_runs bigint NOT NULL DEFAULT 0;

ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS kind varchar(16) NOT NULL DEFAULT 'im',
    ADD COLUMN IF NOT EXISTS input_text text,
    ADD COLUMN IF NOT EXISTS output_text text,
    ADD COLUMN IF NOT EXISTS config_snapshot text;

CREATE INDEX IF NOT EXISTS idx_agent_runs_kind_status ON agent_runs (kind, status);
