-- 0005_access_control_core (down): drop rbac_changed triggers, audit sensitivity columns and indexes,
-- role-binding versioning and the users workspace-admin / disable columns.

DROP TRIGGER IF EXISTS users_rbac_changed ON users;
DROP TRIGGER IF EXISTS role_bindings_rbac_changed ON role_bindings;
DROP FUNCTION IF EXISTS rbac_notify_user_change();
DROP FUNCTION IF EXISTS rbac_notify_binding_change();

DROP INDEX IF EXISTS audit_log_action_idx;
DROP INDEX IF EXISTS audit_log_sensitive_idx;
ALTER TABLE audit_log
    DROP COLUMN IF EXISTS object_label,
    DROP COLUMN IF EXISTS sensitive;

DROP TRIGGER IF EXISTS role_bindings_bump_row_version ON role_bindings;
ALTER TABLE role_bindings
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS row_version;

ALTER TABLE users
    DROP COLUMN IF EXISTS disabled_at,
    DROP COLUMN IF EXISTS is_workspace_admin;
