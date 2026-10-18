-- 2010_access_control_workspace: workspace-wide settings (AI assist, defaults), SIEM export checkpoints
-- and a trigram index so the audit screen's free-text search on object labels stays fast.

CREATE TABLE workspace_settings (
    key         text        PRIMARY KEY,
    value       jsonb       NOT NULL,
    updated_by  uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    row_version bigint      NOT NULL DEFAULT 1
);
CREATE TRIGGER workspace_settings_bump_row_version BEFORE UPDATE ON workspace_settings
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();

CREATE TABLE audit_export_checkpoints (
    sink            text        PRIMARY KEY,
    last_id         bigint      NOT NULL DEFAULT 0,
    exported_total  bigint      NOT NULL DEFAULT 0,
    last_error      text,
    last_attempt_at timestamptz,
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE INDEX audit_log_object_label_trgm_idx ON audit_log USING gin (object_label public.gin_trgm_ops);
