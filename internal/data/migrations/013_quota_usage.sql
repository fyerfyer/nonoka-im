-- +goose Up
-- Product-level quota and usage accounting. Bifrost owns per-agent hard LLM
-- budgets; Nonoka owns actor/agent fairness, run concurrency and explainable
-- per-run cost.

CREATE TABLE IF NOT EXISTS agent_quotas (
    id bigserial PRIMARY KEY,
    scope varchar(16) NOT NULL CHECK (scope IN ('user', 'agent')),
    scope_key varchar(160) NOT NULL,
    max_concurrent_runs integer,
    daily_runs integer,
    daily_tokens bigint,
    daily_cost_usd numeric(18, 8),
    max_tool_turns integer,
    updated_by bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope, scope_key)
);
CREATE INDEX IF NOT EXISTS idx_agent_quotas_scope ON agent_quotas (scope, scope_key);

CREATE TABLE IF NOT EXISTS agent_usage_daily (
    day date NOT NULL,
    scope varchar(16) NOT NULL CHECK (scope IN ('user', 'agent')),
    scope_key varchar(160) NOT NULL,
    runs bigint NOT NULL DEFAULT 0,
    test_runs bigint NOT NULL DEFAULT 0,
    input_tokens bigint NOT NULL DEFAULT 0,
    output_tokens bigint NOT NULL DEFAULT 0,
    cost_usd numeric(18, 8) NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (day, scope, scope_key)
);
CREATE INDEX IF NOT EXISTS idx_agent_usage_daily_scope ON agent_usage_daily (day, scope, scope_key);

CREATE TABLE IF NOT EXISTS agent_usage_events (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    attempt_id varchar(64) NOT NULL,
    step integer NOT NULL,
    kind varchar(16) NOT NULL DEFAULT 'im',
    agent_id bigint NOT NULL,
    actor_user_id bigint NOT NULL,
    topic varchar(128) NOT NULL DEFAULT '',
    model_id bigint NOT NULL DEFAULT 0,
    model_key varchar(200) NOT NULL DEFAULT '',
    provider varchar(64) NOT NULL DEFAULT '',
    input_tokens integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    cost_usd numeric(18, 8) NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, attempt_id, step)
);
CREATE INDEX IF NOT EXISTS idx_agent_usage_events_run ON agent_usage_events (run_id);
CREATE INDEX IF NOT EXISTS idx_agent_usage_events_agent_time ON agent_usage_events (agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_usage_events_user_time ON agent_usage_events (actor_user_id, created_at DESC);

-- Durable, cross-worker dedupe for "agent is over quota" chat notices.
CREATE TABLE IF NOT EXISTS agent_quota_notices (
    agent_id bigint NOT NULL,
    topic varchar(128) NOT NULL,
    error_code varchar(64) NOT NULL,
    last_sent_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, topic, error_code)
);

ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS model_key varchar(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS estimated_cost_usd numeric(18, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS retry_after_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_agent_runs_user_concurrency ON agent_runs (actor_user_id, status, lease_until);
CREATE INDEX IF NOT EXISTS idx_agent_runs_agent_concurrency ON agent_runs (agent_id, status, lease_until);
CREATE INDEX IF NOT EXISTS idx_agent_runs_usage_day ON agent_runs (created_at, agent_id, actor_user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_runs_usage_day;
DROP INDEX IF EXISTS idx_agent_runs_agent_concurrency;
DROP INDEX IF EXISTS idx_agent_runs_user_concurrency;
ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS retry_after_at,
    DROP COLUMN IF EXISTS estimated_cost_usd,
    DROP COLUMN IF EXISTS model_key;
DROP TABLE IF EXISTS agent_quota_notices;
DROP TABLE IF EXISTS agent_usage_events;
DROP TABLE IF EXISTS agent_usage_daily;
DROP TABLE IF EXISTS agent_quotas;
