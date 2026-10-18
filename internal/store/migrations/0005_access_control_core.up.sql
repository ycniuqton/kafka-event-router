-- 0005_access_control_core: workspace-admin flag and disable on users, optimistic version on role bindings,
-- audit sensitivity + object label with read indexes, and NOTIFY rbac_changed for subject-cache invalidation.

ALTER TABLE users
    ADD COLUMN is_workspace_admin boolean NOT NULL DEFAULT false,
    ADD COLUMN disabled_at        timestamptz;

UPDATE users SET is_workspace_admin = true
WHERE id IN (SELECT user_id FROM role_bindings WHERE role = 'admin');

ALTER TABLE role_bindings
    ADD COLUMN row_version bigint      NOT NULL DEFAULT 1,
    ADD COLUMN updated_at  timestamptz NOT NULL DEFAULT now();
CREATE TRIGGER role_bindings_bump_row_version BEFORE UPDATE ON role_bindings
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();

ALTER TABLE audit_log
    ADD COLUMN sensitive    boolean NOT NULL DEFAULT false,
    ADD COLUMN object_label text;
CREATE INDEX audit_log_sensitive_idx ON audit_log (sensitive, at DESC) WHERE sensitive;
CREATE INDEX audit_log_action_idx ON audit_log (action, at DESC);

CREATE FUNCTION rbac_notify_binding_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    affected uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected := OLD.user_id;
    ELSE
        affected := NEW.user_id;
    END IF;
    PERFORM pg_notify('rbac_changed', json_build_object('user_id', affected)::text);
    RETURN NULL;
END;
$$;

CREATE FUNCTION rbac_notify_user_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.is_workspace_admin IS DISTINCT FROM OLD.is_workspace_admin
        OR NEW.disabled_at IS DISTINCT FROM OLD.disabled_at THEN
        PERFORM pg_notify('rbac_changed', json_build_object('user_id', NEW.id)::text);
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER role_bindings_rbac_changed
    AFTER INSERT OR UPDATE OR DELETE ON role_bindings
    FOR EACH ROW EXECUTE FUNCTION rbac_notify_binding_change();
CREATE TRIGGER users_rbac_changed
    AFTER UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION rbac_notify_user_change();
