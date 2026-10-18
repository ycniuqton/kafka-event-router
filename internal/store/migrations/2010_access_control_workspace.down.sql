-- 2010_access_control_workspace (down): the pg_trgm extension stays (other schemas may use it).

DROP INDEX IF EXISTS audit_log_object_label_trgm_idx;
DROP TABLE IF EXISTS audit_export_checkpoints;
DROP TABLE IF EXISTS workspace_settings;
