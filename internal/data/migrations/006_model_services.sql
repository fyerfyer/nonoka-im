-- +goose Up
-- Phase 1: platform Model Service registry.
--
-- This is a deliberate breaking migration. Agent platform data is rebuilt
-- because agents now reference a platform-managed model service/model pair
-- instead of owning arbitrary base URLs and credentials. IM users, bot users
-- and IM messages are preserved.

CREATE TABLE IF NOT EXISTS model_services (
    id bigserial PRIMARY KEY,
    name varchar(100) NOT NULL,
    display_name varchar(200) NOT NULL DEFAULT '',
    gateway_type varchar(32) NOT NULL DEFAULT 'bifrost',
    base_url varchar(500) NOT NULL,
    gateway_key_id varchar(128) NOT NULL DEFAULT '',
    api_key_enc text,
    enabled boolean NOT NULL DEFAULT true,
    created_by bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_services_name ON model_services (name);
CREATE INDEX IF NOT EXISTS idx_model_services_enabled ON model_services (enabled);

CREATE TABLE IF NOT EXISTS model_models (
    id bigserial PRIMARY KEY,
    service_id bigint NOT NULL REFERENCES model_services (id) ON DELETE CASCADE,
    model_key varchar(200) NOT NULL,
    display_name varchar(200) NOT NULL DEFAULT '',
    context_window bigint NOT NULL DEFAULT 0,
    max_output_tokens bigint NOT NULL DEFAULT 4096,
    supports_tools boolean NOT NULL DEFAULT true,
    supports_vision boolean NOT NULL DEFAULT false,
    supports_reasoning boolean NOT NULL DEFAULT false,
    input_price_per_mtok numeric(20, 8) NOT NULL DEFAULT 0,
    output_price_per_mtok numeric(20, 8) NOT NULL DEFAULT 0,
    enabled boolean NOT NULL DEFAULT true,
    last_synced_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_models_service_key ON model_models (service_id, model_key);
CREATE INDEX IF NOT EXISTS idx_model_models_service_id ON model_models (service_id);
CREATE INDEX IF NOT EXISTS idx_model_models_enabled ON model_models (service_id, enabled);

-- Rebuild Agent platform state. Keep IM users (including bot accounts) and
-- messages; the application recreates agents explicitly after the upgrade.
TRUNCATE TABLE
    agent_approvals,
    agent_checkpoints,
    agent_context_snapshots,
    agent_turns,
    agent_sessions,
    agent_runs,
    agent_versions,
    agent_tool_invocations,
    agents,
    idempotency_keys
RESTART IDENTITY CASCADE;

ALTER TABLE agents
    DROP COLUMN IF EXISTS draft_base_url,
    DROP COLUMN IF EXISTS draft_api_key_enc,
    DROP COLUMN IF EXISTS draft_model,
    ADD COLUMN IF NOT EXISTS draft_model_service_id bigint,
    ADD COLUMN IF NOT EXISTS draft_model_id bigint,
    ADD COLUMN IF NOT EXISTS draft_capability_presets text,
    ADD COLUMN IF NOT EXISTS draft_tool_policies text;

ALTER TABLE agent_versions
    DROP COLUMN IF EXISTS base_url,
    DROP COLUMN IF EXISTS api_key_enc,
    DROP COLUMN IF EXISTS model,
    ADD COLUMN IF NOT EXISTS model_service_id bigint,
    ADD COLUMN IF NOT EXISTS model_id bigint,
    ADD COLUMN IF NOT EXISTS model_key varchar(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_presets text,
    ADD COLUMN IF NOT EXISTS tool_policies text;

-- +goose Down
ALTER TABLE agent_versions
    DROP COLUMN IF EXISTS tool_policies,
    DROP COLUMN IF EXISTS capability_presets,
    DROP COLUMN IF EXISTS model_key,
    DROP COLUMN IF EXISTS model_id,
    DROP COLUMN IF EXISTS model_service_id,
    ADD COLUMN IF NOT EXISTS model varchar(100) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS base_url varchar(200),
    ADD COLUMN IF NOT EXISTS api_key_enc text;

ALTER TABLE agents
    DROP COLUMN IF EXISTS draft_tool_policies,
    DROP COLUMN IF EXISTS draft_capability_presets,
    DROP COLUMN IF EXISTS draft_model_id,
    DROP COLUMN IF EXISTS draft_model_service_id,
    ADD COLUMN IF NOT EXISTS draft_model varchar(100),
    ADD COLUMN IF NOT EXISTS draft_base_url varchar(200),
    ADD COLUMN IF NOT EXISTS draft_api_key_enc text;

DROP TABLE IF EXISTS model_models;
DROP TABLE IF EXISTS model_services;
