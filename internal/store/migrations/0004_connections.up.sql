-- 0004_connections: connection columns owned by the connections feature (CN-12).
-- The If-Match token is the existing row_version (0002); no separate version column is added.

ALTER TYPE connection_health ADD VALUE IF NOT EXISTS 'auth_failed';

ALTER TABLE connections
    ADD COLUMN description        text    NOT NULL DEFAULT '',
    ADD COLUMN sensitive          boolean NOT NULL DEFAULT false,
    ADD COLUMN allow_topic_create boolean NOT NULL DEFAULT false,
    ADD COLUMN probe_enabled      boolean NOT NULL DEFAULT true,
    ADD COLUMN kind               text    NOT NULL DEFAULT 'kafka',
    ADD COLUMN last_check         jsonb,
    ADD CONSTRAINT connections_kind_chk CHECK (kind IN ('kafka', 'demo')),
    ADD CONSTRAINT connections_env_tag_chk CHECK (env_tag IN ('dev', 'staging', 'prod'));

CREATE INDEX connections_probe_idx ON connections (last_checked_at NULLS FIRST)
    WHERE probe_enabled AND kind = 'kafka';
CREATE INDEX connections_health_idx ON connections (health_status);
CREATE INDEX connections_name_idx ON connections (name, id);

-- Health writes (test runs, the 60 s probe) must not invalidate an editor's If-Match token:
-- they run with `SET LOCAL router.health_only = 'on'`, which skips the row_version bump.
DROP TRIGGER connections_bump_row_version ON connections;
CREATE TRIGGER connections_bump_row_version BEFORE UPDATE ON connections
    FOR EACH ROW WHEN (current_setting('router.health_only', true) IS DISTINCT FROM 'on')
    EXECUTE FUNCTION bump_row_version();
