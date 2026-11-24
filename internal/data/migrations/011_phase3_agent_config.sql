-- +goose Up
-- Phase 3: agent configuration history, draft playground runs and durable
-- run steps. Existing IM data and published agent versions are preserved.

ALTER TABLE agent_versions
    ADD COLUMN IF NOT EXISTS publish_note text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS published_by bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS published_at timestamptz,
    ADD COLUMN IF NOT EXISTS source_revision bigint NOT NULL DEFAULT 0;

-- Existing versions were published before notes/revisions were tracked. Treat
-- them as published revisions so the UI can distinguish "published" from a
-- draft that has never been published.
UPDATE agent_versions
SET published_at = COALESCE(published_at, created_at, now()),
    source_revision = CASE WHEN source_revision = 0 THEN 1 ELSE source_revision END
WHERE published_at IS NULL OR source_revision = 0;

ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS kind varchar(16) NOT NULL DEFAULT 'im',
    ADD COLUMN IF NOT EXISTS input_text text,
    ADD COLUMN IF NOT EXISTS output_text text,
    ADD COLUMN IF NOT EXISTS config_snapshot text;

CREATE INDEX IF NOT EXISTS idx_agent_runs_kind_status ON agent_runs (kind, status);

-- Durable timeline for the Runs page. Delta events stay transient in Redis;
-- only state/tool/approval events are written here.
CREATE TABLE IF NOT EXISTS agent_run_steps (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    seq bigint NOT NULL,
    type varchar(32) NOT NULL,
    tool varchar(200) NOT NULL DEFAULT '',
    tool_call_id varchar(512) NOT NULL DEFAULT '',
    summary text NOT NULL DEFAULT '',
    status varchar(32) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_run_steps_run_seq ON agent_run_steps (run_id, seq);
CREATE INDEX IF NOT EXISTS idx_agent_run_steps_run_id ON agent_run_steps (run_id);

-- +goose Down
DROP TABLE IF EXISTS agent_run_steps;

DROP INDEX IF EXISTS idx_agent_runs_kind_status;
ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS config_snapshot,
    DROP COLUMN IF EXISTS output_text,
    DROP COLUMN IF EXISTS input_text,
    DROP COLUMN IF EXISTS kind;

ALTER TABLE agent_versions
    DROP COLUMN IF EXISTS source_revision,
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS published_by,
    DROP COLUMN IF EXISTS publish_note;
