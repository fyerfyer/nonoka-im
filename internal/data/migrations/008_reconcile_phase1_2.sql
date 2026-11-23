-- +goose Up
-- Reconcile databases that already applied an earlier experimental Phase 1/2
-- migration (versions 006/007) whose table shape differs from the current
-- design. Fresh databases are unaffected: every operation is guarded.

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'public' AND table_name = 'model_services' AND column_name = 'kind')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_schema = 'public' AND table_name = 'model_services' AND column_name = 'gateway_type') THEN
        ALTER TABLE model_services RENAME COLUMN kind TO gateway_type;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE model_services ADD COLUMN IF NOT EXISTS display_name varchar(200) NOT NULL DEFAULT '';
ALTER TABLE model_services ADD COLUMN IF NOT EXISTS gateway_key_id varchar(128) NOT NULL DEFAULT '';
ALTER TABLE model_services ADD COLUMN IF NOT EXISTS api_key_enc text;
ALTER TABLE model_services ADD COLUMN IF NOT EXISTS enabled boolean NOT NULL DEFAULT true;
ALTER TABLE model_services ADD COLUMN IF NOT EXISTS created_by bigint NOT NULL DEFAULT 0;
ALTER TABLE model_services ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE model_services ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
UPDATE model_services SET display_name = name WHERE display_name = '';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'public' AND table_name = 'model_models' AND column_name = 'input_price_per_1m')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_schema = 'public' AND table_name = 'model_models' AND column_name = 'input_price_per_mtok') THEN
        ALTER TABLE model_models RENAME COLUMN input_price_per_1m TO input_price_per_mtok;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'public' AND table_name = 'model_models' AND column_name = 'output_price_per_1m')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_schema = 'public' AND table_name = 'model_models' AND column_name = 'output_price_per_mtok') THEN
        ALTER TABLE model_models RENAME COLUMN output_price_per_1m TO output_price_per_mtok;
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE model_models ADD COLUMN IF NOT EXISTS last_synced_at timestamptz;
ALTER TABLE model_models ADD COLUMN IF NOT EXISTS supports_tools boolean NOT NULL DEFAULT true;
ALTER TABLE model_models ADD COLUMN IF NOT EXISTS supports_vision boolean NOT NULL DEFAULT false;
ALTER TABLE model_models ADD COLUMN IF NOT EXISTS supports_reasoning boolean NOT NULL DEFAULT false;

ALTER TABLE agents ADD COLUMN IF NOT EXISTS draft_capability_presets text;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS draft_tool_policies text;
UPDATE agents SET draft_capability_presets = '[]' WHERE draft_capability_presets IS NULL;
UPDATE agents SET draft_tool_policies = '{}' WHERE draft_tool_policies IS NULL;
ALTER TABLE agents ALTER COLUMN draft_capability_presets SET DEFAULT '[]';
ALTER TABLE agents ALTER COLUMN draft_tool_policies SET DEFAULT '{}';

ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS capability_presets text;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS tool_policies text;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS model_key varchar(200) NOT NULL DEFAULT '';
UPDATE agent_versions SET capability_presets = '[]' WHERE capability_presets IS NULL;
UPDATE agent_versions SET tool_policies = '{}' WHERE tool_policies IS NULL;
ALTER TABLE agent_versions ALTER COLUMN capability_presets SET DEFAULT '[]';
ALTER TABLE agent_versions ALTER COLUMN tool_policies SET DEFAULT '{}';

-- Older experimental MCP registry had a single allowed_tools column.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'public' AND table_name = 'mcp_servers' AND column_name = 'allowed_tools') THEN
        DROP TABLE mcp_servers CASCADE;
    END IF;
END $$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS mcp_servers (
    id bigserial PRIMARY KEY,
    owner_user_id bigint NOT NULL,
    name varchar(100) NOT NULL,
    display_name varchar(200) NOT NULL DEFAULT '',
    description text,
    transport varchar(32) NOT NULL DEFAULT 'streamable_http',
    url varchar(500) NOT NULL,
    auth_type varchar(32) NOT NULL DEFAULT 'none',
    credential_enc text,
    visibility varchar(16) NOT NULL DEFAULT 'private',
    review_status varchar(16) NOT NULL DEFAULT 'approved',
    review_reason text,
    review_by bigint,
    review_at timestamptz,
    health_status varchar(24) NOT NULL DEFAULT 'unknown',
    last_checked_at timestamptz,
    last_error text,
    enabled boolean NOT NULL DEFAULT true,
    tools_revision bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_servers_owner_name ON mcp_servers (owner_user_id, name);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_owner ON mcp_servers (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_visibility ON mcp_servers (visibility, review_status, enabled);

CREATE TABLE IF NOT EXISTS mcp_tools (
    id bigserial PRIMARY KEY,
    server_id bigint NOT NULL REFERENCES mcp_servers (id) ON DELETE CASCADE,
    name varchar(200) NOT NULL,
    description text,
    input_schema jsonb,
    output_schema jsonb,
    annotations jsonb,
    risk varchar(24) NOT NULL DEFAULT 'write',
    schema_hash varchar(64) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    discovered_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_tools_server_name ON mcp_tools (server_id, name);
CREATE INDEX IF NOT EXISTS idx_mcp_tools_server ON mcp_tools (server_id);

CREATE TABLE IF NOT EXISTS mcp_connections (
    id bigserial PRIMARY KEY,
    server_id bigint NOT NULL REFERENCES mcp_servers (id) ON DELETE CASCADE,
    user_id bigint NOT NULL,
    auth_type varchar(32) NOT NULL,
    credential_enc text NOT NULL,
    status varchar(24) NOT NULL DEFAULT 'connected',
    expires_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_connections_server_user ON mcp_connections (server_id, user_id);
CREATE INDEX IF NOT EXISTS idx_mcp_connections_user ON mcp_connections (user_id);

-- Older experimental skills table stored one global row per name/version.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'public' AND table_name = 'skills' AND column_name = 'name') THEN
        DROP TABLE IF EXISTS skill_versions CASCADE;
        DROP TABLE skills CASCADE;
    END IF;
END $$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS skills (
    id bigserial PRIMARY KEY,
    owner_user_id bigint NOT NULL,
    slug varchar(100) NOT NULL,
    display_name varchar(200) NOT NULL DEFAULT '',
    description text,
    visibility varchar(16) NOT NULL DEFAULT 'private',
    review_status varchar(16) NOT NULL DEFAULT 'approved',
    latest_version bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_owner_slug ON skills (owner_user_id, slug);
CREATE INDEX IF NOT EXISTS idx_skills_owner ON skills (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_skills_visibility ON skills (visibility, review_status);

CREATE TABLE IF NOT EXISTS skill_versions (
    id bigserial PRIMARY KEY,
    skill_id bigint NOT NULL REFERENCES skills (id) ON DELETE CASCADE,
    version bigint NOT NULL,
    content text NOT NULL,
    front_matter jsonb,
    checksum varchar(64) NOT NULL,
    status varchar(24) NOT NULL DEFAULT 'published',
    review_status varchar(16) NOT NULL DEFAULT 'approved',
    review_reason text,
    review_by bigint,
    review_at timestamptz,
    created_by bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_versions_skill_version ON skill_versions (skill_id, version);
CREATE INDEX IF NOT EXISTS idx_skill_versions_skill ON skill_versions (skill_id, status);

-- +goose Down
-- The reconcile migration is intentionally not destructive on rollback; the
-- previous experimental shape is not a supported state.
