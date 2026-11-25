-- +goose Up
CREATE TABLE IF NOT EXISTS agent_run_citations (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    turn_id bigint,
    ordinal integer NOT NULL DEFAULT 0,
    topic varchar(128) NOT NULL,
    msg_id bigint NOT NULL,
    topic_seq bigint NOT NULL,
    sender_id bigint NOT NULL,
    snippet text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, topic, msg_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_run_citations_run ON agent_run_citations (run_id, id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_reply_msg ON agent_runs (reply_msg_id);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_runs_reply_msg;
DROP TABLE IF EXISTS agent_run_citations;
