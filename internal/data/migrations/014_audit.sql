-- +goose Up
-- Append-only audit trail for sensitive platform and agent operations.

CREATE TABLE IF NOT EXISTS audit_events (
    id bigserial PRIMARY KEY,
    actor_user_id bigint NOT NULL DEFAULT 0,
    action varchar(80) NOT NULL,
    resource_type varchar(40) NOT NULL,
    resource_id varchar(160) NOT NULL DEFAULT '',
    before jsonb,
    after jsonb,
    request_id varchar(128) NOT NULL DEFAULT '',
    trace_id varchar(64) NOT NULL DEFAULT '',
    ip varchar(64) NOT NULL DEFAULT '',
    user_agent varchar(512) NOT NULL DEFAULT '',
    outcome varchar(16) NOT NULL DEFAULT 'success',
    error_code varchar(64) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_events_created ON audit_events (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events (actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_resource ON audit_events (resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events (action, created_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_events_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'audit_events is append-only';
    END IF;
    IF current_setting('nonoka.audit_purge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'audit_events can only be deleted by the retention purge';
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_audit_events_append_only ON audit_events;
CREATE TRIGGER trg_audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();

-- +goose Down
DROP TRIGGER IF EXISTS trg_audit_events_append_only ON audit_events;
DROP FUNCTION IF EXISTS audit_events_append_only();
DROP TABLE IF EXISTS audit_events;
