-- +goose Up
-- Baseline schema matching the pre-Phase-0 GORM AutoMigrate output.
-- IF NOT EXISTS keeps this migration safe for databases created before
-- versioned migrations were introduced.

CREATE TABLE IF NOT EXISTS users (
    id bigserial PRIMARY KEY,
    username varchar(50) NOT NULL,
    password varchar(255) NOT NULL,
    is_bot boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username);

CREATE TABLE IF NOT EXISTS groups (
    group_id varchar(26) PRIMARY KEY,
    name varchar(100) NOT NULL,
    owner_id bigint NOT NULL,
    topic varchar(64) NOT NULL,
    created_at bigint NOT NULL
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id varchar(64) NOT NULL,
    user_id bigint NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_members_group_user ON group_members (group_id, user_id);

CREATE TABLE IF NOT EXISTS conversations (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL,
    topic varchar(64) NOT NULL,
    type varchar(16) NOT NULL,
    peer_id bigint NOT NULL,
    peer_username varchar(50),
    last_msg_preview varchar(255),
    last_msg_at bigint,
    last_seq bigint DEFAULT 0,
    last_read_seq bigint DEFAULT 0,
    unread_count integer DEFAULT 0,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversations_user_topic ON conversations (user_id, topic);
CREATE INDEX IF NOT EXISTS idx_conversations_user_updated ON conversations (user_id);

CREATE TABLE IF NOT EXISTS agents (
    id bigserial PRIMARY KEY,
    bot_user_id bigint NOT NULL,
    owner_user_id bigint NOT NULL,
    name varchar(100) NOT NULL,
    description text,
    status varchar(24) NOT NULL DEFAULT 'draft',
    current_version bigint,
    draft_instruction text,
    draft_model varchar(100),
    draft_tools text,
    draft_base_url varchar(200),
    draft_api_key_enc text,
    draft_max_tokens bigint NOT NULL DEFAULT 4096,
    draft_mcp_bindings text,
    draft_skills text,
    draft_revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_bot_user_id ON agents (bot_user_id);
CREATE INDEX IF NOT EXISTS idx_agents_owner_user_id ON agents (owner_user_id);

CREATE TABLE IF NOT EXISTS agent_versions (
    id bigserial PRIMARY KEY,
    agent_id bigint NOT NULL,
    version bigint NOT NULL,
    instruction text NOT NULL,
    model varchar(100) NOT NULL,
    tools text NOT NULL,
    base_url varchar(200),
    api_key_enc text,
    max_tokens bigint NOT NULL DEFAULT 4096,
    mcp_bindings text,
    skills text,
    created_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_version ON agent_versions (agent_id, version);

CREATE TABLE IF NOT EXISTS agent_sessions (
    id bigserial PRIMARY KEY,
    agent_id bigint NOT NULL,
    actor_user_id bigint NOT NULL,
    topic varchar(128) NOT NULL,
    updated_at timestamptz,
    created_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_session ON agent_sessions (agent_id, actor_user_id, topic);

CREATE TABLE IF NOT EXISTS agent_runs (
    id bigserial PRIMARY KEY,
    event_id varchar(128) NOT NULL,
    agent_id bigint NOT NULL,
    agent_version bigint NOT NULL DEFAULT 0,
    session_id bigint,
    actor_user_id bigint NOT NULL,
    topic varchar(128) NOT NULL,
    msg_id bigint NOT NULL,
    topic_seq bigint NOT NULL DEFAULT 0,
    status varchar(24) NOT NULL,
    error_code varchar(64),
    attempts bigint NOT NULL DEFAULT 0,
    reply_msg_id bigint,
    checkpoint_id varchar(64),
    input_tokens bigint,
    output_tokens bigint,
    total_tokens bigint,
    created_at timestamptz,
    started_at timestamptz,
    finished_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_run_event ON agent_runs (event_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_agent_id ON agent_runs (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_session_id ON agent_runs (session_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_actor_user_id ON agent_runs (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_topic ON agent_runs (topic);
CREATE INDEX IF NOT EXISTS idx_agent_runs_msg_id ON agent_runs (msg_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_topic_seq ON agent_runs (topic_seq);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs (status);

CREATE TABLE IF NOT EXISTS agent_turns (
    id bigserial PRIMARY KEY,
    agent_id bigint NOT NULL,
    actor_user_id bigint NOT NULL,
    topic varchar(128) NOT NULL,
    session_id bigint,
    run_id bigint NOT NULL,
    trigger_msg_id bigint,
    reply_msg_id bigint,
    input text NOT NULL,
    output text,
    created_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_turns_run_id ON agent_turns (run_id);
CREATE INDEX IF NOT EXISTS idx_agent_turns_agent_id ON agent_turns (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_turns_actor_user_id ON agent_turns (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_agent_turns_topic ON agent_turns (topic);
CREATE INDEX IF NOT EXISTS idx_agent_turns_session_id ON agent_turns (session_id);

CREATE TABLE IF NOT EXISTS agent_context_snapshots (
    id bigserial PRIMARY KEY,
    session_id bigint NOT NULL,
    last_turn_id bigint NOT NULL,
    agent_version bigint NOT NULL,
    summary text NOT NULL,
    summary_tokens bigint,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_context_snapshots_session_id ON agent_context_snapshots (session_id);

CREATE TABLE IF NOT EXISTS agent_checkpoints (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    checkpoint_id varchar(64) NOT NULL,
    state bytea NOT NULL,
    created_at timestamptz,
    expires_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_checkpoints_checkpoint_id ON agent_checkpoints (checkpoint_id);
CREATE INDEX IF NOT EXISTS idx_agent_checkpoints_run_id ON agent_checkpoints (run_id);
CREATE INDEX IF NOT EXISTS idx_agent_checkpoints_expires_at ON agent_checkpoints (expires_at);

CREATE TABLE IF NOT EXISTS agent_approvals (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    tool_call_id varchar(128) NOT NULL,
    tool varchar(128) NOT NULL,
    args_summary text NOT NULL,
    args_hash varchar(64) NOT NULL,
    approver_id bigint NOT NULL,
    status varchar(24) NOT NULL,
    expires_at timestamptz,
    consumed_at timestamptz,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_agent_approval_run ON agent_approvals (run_id, tool_call_id);
CREATE INDEX IF NOT EXISTS idx_agent_approvals_approver_id ON agent_approvals (approver_id);
CREATE INDEX IF NOT EXISTS idx_agent_approvals_status ON agent_approvals (status);

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

-- Backfill sessions for turns created before sessions existed.
INSERT INTO agent_sessions (agent_id, actor_user_id, topic, created_at, updated_at)
SELECT DISTINCT agent_id, actor_user_id, topic, NOW(), NOW()
FROM agent_turns WHERE session_id = 0
ON CONFLICT (agent_id, actor_user_id, topic) DO NOTHING;
UPDATE agent_turns AS t SET session_id = s.id
FROM agent_sessions AS s
WHERE t.session_id = 0
  AND t.agent_id = s.agent_id AND t.actor_user_id = s.actor_user_id AND t.topic = s.topic;

-- +goose Down
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS mcp_servers;
DROP TABLE IF EXISTS agent_approvals;
DROP TABLE IF EXISTS agent_checkpoints;
DROP TABLE IF EXISTS agent_context_snapshots;
DROP TABLE IF EXISTS agent_turns;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS agent_versions;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS group_members;
DROP TABLE IF EXISTS groups;
DROP TABLE IF EXISTS users;
