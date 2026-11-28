-- +goose Up
-- The queued-run replay anti-join filters by session_id, topic_seq, id and
-- status. The original single-column session index forces a large per-session
-- scan as long-lived agent sessions accumulate runs.
CREATE INDEX IF NOT EXISTS idx_agent_runs_session_order
    ON agent_runs (session_id, topic_seq, id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_runs_session_order;
