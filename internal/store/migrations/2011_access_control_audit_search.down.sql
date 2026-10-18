-- 2011_access_control_audit_search (down)

DROP INDEX IF EXISTS audit_log_object_id_idx;
DROP INDEX IF EXISTS audit_log_action_trgm_idx;
DROP INDEX IF EXISTS audit_log_object_id_trgm_idx;
