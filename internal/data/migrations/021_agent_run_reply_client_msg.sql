-- +goose Up
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS reply_client_msg_id varchar(128) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_agent_runs_reply_client_msg ON agent_runs (reply_client_msg_id);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_runs_reply_client_msg;
ALTER TABLE agent_runs DROP COLUMN IF EXISTS reply_client_msg_id;
