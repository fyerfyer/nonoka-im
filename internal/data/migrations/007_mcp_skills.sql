-- +goose Up
-- Phase 2: user-owned MCP servers/connections and versioned Skills.
--
-- Deliberate breaking migration: the previous admin-only registries are
-- rebuilt. Agent platform data was cleared in 006, so no bindings need to be
-- preserved here.

DROP TABLE IF EXISTS mcp_servers CASCADE;
DROP TABLE IF EXISTS skills CASCADE;

CREATE TABLE mcp_servers (
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
CREATE UNIQUE INDEX idx_mcp_servers_owner_name ON mcp_servers (owner_user_id, name);
CREATE INDEX idx_mcp_servers_owner ON mcp_servers (owner_user_id);
CREATE INDEX idx_mcp_servers_visibility ON mcp_servers (visibility, review_status, enabled);

CREATE TABLE mcp_tools (
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
CREATE UNIQUE INDEX idx_mcp_tools_server_name ON mcp_tools (server_id, name);
CREATE INDEX idx_mcp_tools_server ON mcp_tools (server_id);

CREATE TABLE mcp_connections (
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
CREATE UNIQUE INDEX idx_mcp_connections_server_user ON mcp_connections (server_id, user_id);
CREATE INDEX idx_mcp_connections_user ON mcp_connections (user_id);

CREATE TABLE skills (
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
CREATE UNIQUE INDEX idx_skills_owner_slug ON skills (owner_user_id, slug);
CREATE INDEX idx_skills_owner ON skills (owner_user_id);
CREATE INDEX idx_skills_visibility ON skills (visibility, review_status);

CREATE TABLE skill_versions (
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
CREATE UNIQUE INDEX idx_skill_versions_skill_version ON skill_versions (skill_id, version);
CREATE INDEX idx_skill_versions_skill ON skill_versions (skill_id, status);

-- +goose Down
DROP TABLE IF EXISTS skill_versions;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS mcp_connections;
DROP TABLE IF EXISTS mcp_tools;
DROP TABLE IF EXISTS mcp_servers;

CREATE TABLE IF NOT EXISTS mcp_servers (
    id bigserial PRIMARY KEY,
    name varchar(100) NOT NULL,
    transport varchar(16) NOT NULL DEFAULT 'http',
    url varchar(500) NOT NULL,
    allowed_tools text NOT NULL,
    risk_class varchar(24) NOT NULL DEFAULT 'read_only',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_servers_name ON mcp_servers (name);

CREATE TABLE IF NOT EXISTS skills (
    id bigserial PRIMARY KEY,
    name varchar(100) NOT NULL,
    version bigint NOT NULL,
    content text NOT NULL,
    published boolean NOT NULL DEFAULT false,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_name_version ON skills (name, version);
