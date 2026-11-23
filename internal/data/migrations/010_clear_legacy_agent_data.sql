-- +goose Up
-- Databases that applied the earlier experimental Phase 1/2 migrations kept
-- their old agent/MCP/Skill rows because goose skipped the rewritten 006/007.
-- The migration decision for this refactor is a clean Agent-platform rebuild,
-- so clear those tables once. IM users and messages are untouched.
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
    mcp_servers,
    mcp_tools,
    mcp_connections,
    skills,
    skill_versions
RESTART IDENTITY CASCADE;

-- +goose Down
-- Data cleanup only.
