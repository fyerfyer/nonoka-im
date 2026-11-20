-- +goose Up
-- Approval idempotency: one pending/decided row per (run, tool call). The old
-- non-unique index allowed concurrent FirstOrCreate calls to insert duplicates.
ALTER TABLE agent_approvals ALTER COLUMN tool_call_id TYPE varchar(512);

DELETE FROM agent_approvals a
USING agent_approvals b
WHERE a.id < b.id
  AND a.run_id = b.run_id
  AND a.tool_call_id = b.tool_call_id;

DROP INDEX IF EXISTS idx_agent_approval_run;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_approval_run_tool ON agent_approvals (run_id, tool_call_id);

-- A run that already produced an external side effect must not be retried
-- automatically after a transient failure.
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS side_effect_at timestamptz;

-- Durable ledger for side-effecting tool calls. The unique key is the tool call
-- identity inside one run, so a resume/retry can never execute the same call
-- twice.
CREATE TABLE IF NOT EXISTS agent_tool_invocations (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    tool_call_id varchar(128) NOT NULL,
    tool varchar(128) NOT NULL,
    args_hash varchar(64) NOT NULL,
    status varchar(16) NOT NULL,
    result_summary text,
    error_code varchar(64),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_tool_invocation ON agent_tool_invocations (run_id, tool_call_id);
CREATE INDEX IF NOT EXISTS idx_agent_tool_invocations_run ON agent_tool_invocations (run_id);

-- +goose Down
DROP TABLE IF EXISTS agent_tool_invocations;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS side_effect_at;
DROP INDEX IF EXISTS idx_agent_approval_run_tool;
CREATE INDEX IF NOT EXISTS idx_agent_approval_run ON agent_approvals (run_id, tool_call_id);
