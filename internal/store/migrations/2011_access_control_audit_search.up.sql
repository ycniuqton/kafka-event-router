-- 2011_access_control_audit_search: indexes behind the audit screen's free-text search (object id and action
-- trigram matches, and the object_id lookup of entries whose live object name matches).

CREATE INDEX audit_log_object_id_trgm_idx ON audit_log USING gin (object_id public.gin_trgm_ops);
CREATE INDEX audit_log_action_trgm_idx ON audit_log USING gin (action public.gin_trgm_ops);
CREATE INDEX audit_log_object_id_idx ON audit_log (object_id);
