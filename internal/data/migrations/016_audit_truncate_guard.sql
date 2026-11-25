-- +goose Up
-- TRUNCATE bypasses row-level DELETE triggers; guard it separately.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_events_no_truncate() RETURNS trigger AS $$
BEGIN
    IF current_setting('nonoka.audit_purge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'audit_events can only be truncated by the retention purge';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_audit_events_no_truncate ON audit_events;
CREATE TRIGGER trg_audit_events_no_truncate
    BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_no_truncate();

-- +goose Down
DROP TRIGGER IF EXISTS trg_audit_events_no_truncate ON audit_events;
DROP FUNCTION IF EXISTS audit_events_no_truncate();
