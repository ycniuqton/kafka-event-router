-- 0004_connections (down): drop the connections feature columns.
-- Postgres cannot drop an enum value: 'auth_failed' stays in connection_health; rows using it are reset.

DROP TRIGGER IF EXISTS connections_bump_row_version ON connections;
CREATE TRIGGER connections_bump_row_version BEFORE UPDATE ON connections
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();

DROP INDEX IF EXISTS connections_name_idx;
DROP INDEX IF EXISTS connections_health_idx;
DROP INDEX IF EXISTS connections_probe_idx;

UPDATE connections SET health_status = 'unknown' WHERE health_status::text = 'auth_failed';

ALTER TABLE connections
    DROP CONSTRAINT IF EXISTS connections_env_tag_chk,
    DROP CONSTRAINT IF EXISTS connections_kind_chk,
    DROP COLUMN IF EXISTS last_check,
    DROP COLUMN IF EXISTS kind,
    DROP COLUMN IF EXISTS probe_enabled,
    DROP COLUMN IF EXISTS allow_topic_create,
    DROP COLUMN IF EXISTS sensitive,
    DROP COLUMN IF EXISTS description;
