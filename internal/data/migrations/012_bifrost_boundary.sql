-- +goose Up
-- Bifrost is the single LLM gateway. Collapse the platform ModelService layer
-- into a flat, locally curated model catalog and move virtual keys to the
-- agent/playground owners.
--
-- This is a deliberate breaking migration: agents reference one model_id
-- instead of (model_service_id, model_id). Existing model rows are preserved
-- and duplicate model keys are merged by keeping the lowest id.

-- Merge duplicate model keys across legacy model services before dropping the
-- service dimension. Agents and versions are remapped to the surviving row.
CREATE TEMP TABLE model_id_map ON COMMIT DROP AS
SELECT m.id AS old_id, k.keep_id AS new_id
FROM model_models m
JOIN (
    SELECT model_key, min(id) AS keep_id
    FROM model_models
    GROUP BY model_key
) k ON k.model_key = m.model_key;

UPDATE agents a
SET draft_model_id = map.new_id
FROM model_id_map map
WHERE a.draft_model_id = map.old_id;

UPDATE agent_versions v
SET model_id = map.new_id
FROM model_id_map map
WHERE v.model_id = map.old_id;

DELETE FROM model_models m
USING model_id_map map
WHERE m.id = map.old_id AND map.old_id <> map.new_id;

ALTER TABLE model_models DROP CONSTRAINT IF EXISTS model_models_service_id_fkey;
ALTER TABLE model_models DROP COLUMN IF EXISTS service_id;
ALTER TABLE model_models RENAME TO model_catalog;

ALTER TABLE model_catalog
    ADD COLUMN IF NOT EXISTS provider varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gateway_model_key varchar(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS icon varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS curated boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS recommended boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS sort_order integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS input_cache_read_price_per_mtok numeric(20, 8) NOT NULL DEFAULT 0;

UPDATE model_catalog
SET provider = CASE WHEN position('/' in model_key) > 0 THEN split_part(model_key, '/', 1) ELSE '' END,
    gateway_model_key = CASE
        WHEN position('/' in model_key) > 0 THEN substring(model_key from position('/' in model_key) + 1)
        ELSE model_key
    END,
    curated = true
WHERE gateway_model_key = '';

DROP INDEX IF EXISTS idx_model_models_service_key;
DROP INDEX IF EXISTS idx_model_models_service_id;
DROP INDEX IF EXISTS idx_model_models_enabled;
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_catalog_key ON model_catalog (model_key);
CREATE INDEX IF NOT EXISTS idx_model_catalog_enabled ON model_catalog (enabled, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_model_catalog_provider ON model_catalog (provider);

DROP TABLE IF EXISTS model_services CASCADE;

ALTER TABLE agents DROP COLUMN IF EXISTS draft_model_service_id;
ALTER TABLE agent_versions DROP COLUMN IF EXISTS model_service_id;

-- Per-agent and playground virtual keys. The sealed value is never exposed by
-- the API; only the status is returned to clients.
CREATE TABLE IF NOT EXISTS agent_gateway_keys (
    agent_id bigint PRIMARY KEY,
    virtual_key_id varchar(128) NOT NULL,
    value_enc text NOT NULL,
    status varchar(24) NOT NULL DEFAULT 'active',
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agent_gateway_keys_status ON agent_gateway_keys (status);

CREATE TABLE IF NOT EXISTS platform_gateway_keys (
    kind varchar(24) PRIMARY KEY,
    virtual_key_id varchar(128) NOT NULL,
    value_enc text NOT NULL,
    status varchar(24) NOT NULL DEFAULT 'active',
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Failed revocations are retried by the worker cleanup loop instead of
-- blocking agent deletion.
CREATE TABLE IF NOT EXISTS gateway_key_cleanup (
    virtual_key_id varchar(128) PRIMARY KEY,
    agent_id bigint NOT NULL DEFAULT 0,
    reason varchar(64) NOT NULL DEFAULT '',
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_gateway_key_cleanup_due ON gateway_key_cleanup (next_attempt_at);

-- +goose Down
DROP TABLE IF EXISTS gateway_key_cleanup;
DROP TABLE IF EXISTS platform_gateway_keys;
DROP TABLE IF EXISTS agent_gateway_keys;

ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS model_service_id bigint;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS draft_model_service_id bigint;

ALTER TABLE model_catalog RENAME TO model_models;
ALTER TABLE model_models
    DROP COLUMN IF EXISTS input_cache_read_price_per_mtok,
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS recommended,
    DROP COLUMN IF EXISTS curated,
    DROP COLUMN IF EXISTS icon,
    DROP COLUMN IF EXISTS gateway_model_key,
    DROP COLUMN IF EXISTS provider,
    ADD COLUMN IF NOT EXISTS service_id bigint;
DROP INDEX IF EXISTS idx_model_catalog_provider;
DROP INDEX IF EXISTS idx_model_catalog_enabled;
DROP INDEX IF EXISTS idx_model_catalog_key;
CREATE INDEX IF NOT EXISTS idx_model_models_service_id ON model_models (service_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_models_service_key ON model_models (service_id, model_key);

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
