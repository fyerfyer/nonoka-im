-- +goose Up
-- Lease/fencing state and failure classification for the Agent run state
-- machine. Existing running rows get a grace lease so a deploy does not
-- immediately reclaim work owned by the previous binary.

ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS attempt_id varchar(64);
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS lease_until timestamptz;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS error_class varchar(32);
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS last_error text;

UPDATE agent_runs
SET lease_until = now() + interval '5 minutes'
WHERE status = 'running' AND lease_until IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_runs_claimable ON agent_runs (status, next_attempt_at, topic_seq, id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_lease ON agent_runs (lease_until) WHERE status = 'running';

ALTER TABLE agent_approvals ADD COLUMN IF NOT EXISTS decided_at timestamptz;
ALTER TABLE agent_approvals ADD COLUMN IF NOT EXISTS decided_by bigint;
ALTER TABLE agent_approvals ADD COLUMN IF NOT EXISTS attempt_id varchar(64);

CREATE INDEX IF NOT EXISTS idx_agent_approvals_pending ON agent_approvals (status, expires_at) WHERE status = 'pending';

-- +goose Down
DROP INDEX IF EXISTS idx_agent_approvals_pending;
ALTER TABLE agent_approvals DROP COLUMN IF EXISTS attempt_id;
ALTER TABLE agent_approvals DROP COLUMN IF EXISTS decided_by;
ALTER TABLE agent_approvals DROP COLUMN IF EXISTS decided_at;
DROP INDEX IF EXISTS idx_agent_runs_lease;
DROP INDEX IF EXISTS idx_agent_runs_claimable;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS last_error;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS error_class;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS next_attempt_at;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS lease_until;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS attempt_id;
