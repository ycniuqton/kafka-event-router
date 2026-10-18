-- 0003_secrets_audit (down): drop audit context columns, the secret_ref FK and the secrets table.

DROP INDEX IF EXISTS audit_log_team_idx;
ALTER TABLE audit_log
    DROP COLUMN IF EXISTS actor_kind,
    DROP COLUMN IF EXISTS team_id,
    DROP COLUMN IF EXISTS request_id;

ALTER TABLE connections DROP CONSTRAINT IF EXISTS connections_secret_ref_fk;

DROP INDEX IF EXISTS secrets_key_id_idx;
DROP TABLE IF EXISTS secrets;
