-- 3020_lookup_sources: managed lookup sources for lookup steps (connections 00001-lookup-sources, CN-LS-01).
-- Range 3020..3029 of the connections folder. Kind settings and limits are JSONB (never secrets);
-- secrets live in the secrets store behind secret_ref.

CREATE TABLE lookup_sources (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         uuid NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    kind            text NOT NULL,
    env_tag         text NOT NULL DEFAULT 'dev',
    sensitive       boolean NOT NULL DEFAULT false,
    config          jsonb NOT NULL DEFAULT '{}'::jsonb,
    limits          jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_ref      text,
    secret_fields   text[] NOT NULL DEFAULT '{}',
    health_status   text NOT NULL DEFAULT 'unknown',
    last_checked_at timestamptz,
    last_check      jsonb,
    row_version     bigint NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, name),
    CONSTRAINT lookup_sources_kind_chk CHECK (kind IN ('http', 'redis', 'postgres', 'static')),
    CONSTRAINT lookup_sources_env_tag_chk CHECK (env_tag IN ('dev', 'staging', 'prod')),
    CONSTRAINT lookup_sources_health_chk CHECK (health_status IN ('unknown', 'healthy', 'degraded', 'unreachable', 'auth_failed'))
);

CREATE INDEX lookup_sources_name_idx ON lookup_sources (name, id);
CREATE INDEX lookup_sources_kind_idx ON lookup_sources (kind);

-- Test results must not invalidate an editor's If-Match token (same convention as connections 0004).
CREATE TRIGGER lookup_sources_bump_row_version BEFORE UPDATE ON lookup_sources
    FOR EACH ROW WHEN (current_setting('router.health_only', true) IS DISTINCT FROM 'on')
    EXECUTE FUNCTION bump_row_version();
