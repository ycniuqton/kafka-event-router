-- 0001_init: control-plane schema (design doc section 4).
-- Route/transform/lookup-table versions are immutable; DLQ events live in Kafka, not here.

CREATE TYPE team_role AS ENUM ('viewer', 'editor', 'approver', 'admin');
CREATE TYPE connection_security_mode AS ENUM (
    'plaintext', 'ssl', 'mtls', 'sasl_plain', 'sasl_scram_256', 'sasl_scram_512', 'sasl_oauthbearer'
);
CREATE TYPE connection_health AS ENUM ('unknown', 'healthy', 'degraded', 'unreachable');
CREATE TYPE route_desired_state AS ENUM ('running', 'paused', 'archived');
CREATE TYPE route_observed_state AS ENUM ('running', 'paused', 'error');
CREATE TYPE deployment_status AS ENUM ('pending_approval', 'rolling_out', 'active', 'failed', 'superseded');

CREATE TABLE teams (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email        text NOT NULL UNIQUE,
    display_name text NOT NULL,
    sso_subject  text UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_bindings (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    role       team_role NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, team_id)
);
CREATE INDEX role_bindings_team_idx ON role_bindings (team_id);

CREATE TABLE connections (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id           uuid NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name              text NOT NULL,
    env_tag           text NOT NULL DEFAULT 'dev',
    bootstrap_servers text[] NOT NULL CHECK (cardinality(bootstrap_servers) > 0),
    security_mode     connection_security_mode NOT NULL,
    config            jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_ref        text,
    schema_registry   jsonb,
    health_status     connection_health NOT NULL DEFAULT 'unknown',
    last_checked_at   timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, name)
);
CREATE INDEX connections_env_tag_idx ON connections (env_tag);

CREATE TABLE routes (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id           uuid NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name              text NOT NULL,
    tags              text[] NOT NULL DEFAULT '{}',
    desired_state     route_desired_state NOT NULL DEFAULT 'paused',
    active_version_id uuid,
    replicas          integer NOT NULL DEFAULT 2 CHECK (replicas >= 1),
    draft_config      jsonb,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, name)
);
CREATE INDEX routes_desired_state_idx ON routes (desired_state);
CREATE INDEX routes_tags_idx ON routes USING gin (tags);

CREATE TABLE route_versions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id    uuid NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    version_no  integer NOT NULL CHECK (version_no >= 1),
    config      jsonb NOT NULL,
    config_hash text NOT NULL,
    created_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    change_note text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (route_id, version_no)
);

ALTER TABLE routes
    ADD CONSTRAINT routes_active_version_fk
    FOREIGN KEY (active_version_id) REFERENCES route_versions (id) ON DELETE SET NULL;

CREATE TABLE deployments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id     uuid NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    version_id   uuid NOT NULL REFERENCES route_versions (id) ON DELETE CASCADE,
    status       deployment_status NOT NULL,
    requested_by uuid REFERENCES users (id) ON DELETE SET NULL,
    approved_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    approved_at  timestamptz,
    started_at   timestamptz,
    finished_at  timestamptz
);
CREATE INDEX deployments_route_idx ON deployments (route_id, requested_at DESC);
CREATE INDEX deployments_pending_idx ON deployments (status) WHERE status = 'pending_approval';

CREATE TABLE route_runtime (
    route_id         uuid PRIMARY KEY REFERENCES routes (id) ON DELETE CASCADE,
    observed_state   route_observed_state NOT NULL,
    error_message    text,
    assigned_workers text[] NOT NULL DEFAULT '{}',
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE transforms (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, name)
);

CREATE TABLE transform_versions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transform_id uuid NOT NULL REFERENCES transforms (id) ON DELETE CASCADE,
    version_no   integer NOT NULL CHECK (version_no >= 1),
    steps        jsonb NOT NULL,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (transform_id, version_no)
);

CREATE TABLE lookup_tables (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, name)
);

CREATE TABLE lookup_table_versions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lookup_table_id uuid NOT NULL REFERENCES lookup_tables (id) ON DELETE CASCADE,
    version_no      integer NOT NULL CHECK (version_no >= 1),
    rows            jsonb NOT NULL,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (lookup_table_id, version_no)
);

CREATE TABLE test_cases (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id        uuid REFERENCES routes (id) ON DELETE CASCADE,
    transform_id    uuid REFERENCES transforms (id) ON DELETE CASCADE,
    name            text,
    input_event     jsonb NOT NULL,
    expected_output jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT test_cases_owner_chk CHECK (num_nonnulls(route_id, transform_id) = 1)
);
CREATE INDEX test_cases_route_idx ON test_cases (route_id) WHERE route_id IS NOT NULL;
CREATE INDEX test_cases_transform_idx ON test_cases (transform_id) WHERE transform_id IS NOT NULL;

CREATE TABLE workers (
    id                text PRIMARY KEY,
    host              text NOT NULL,
    version           text NOT NULL,
    capacity          integer NOT NULL CHECK (capacity >= 0),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workers_heartbeat_idx ON workers (last_heartbeat_at);

CREATE TABLE route_assignments (
    route_id         uuid NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    worker_id        text NOT NULL REFERENCES workers (id) ON DELETE CASCADE,
    lease_expires_at timestamptz NOT NULL,
    PRIMARY KEY (route_id, worker_id)
);
CREATE INDEX route_assignments_worker_idx ON route_assignments (worker_id);
CREATE INDEX route_assignments_lease_idx ON route_assignments (lease_expires_at);

CREATE TABLE alert_rules (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id   uuid NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    kind       text NOT NULL,
    threshold  double precision NOT NULL,
    "window"   interval NOT NULL,
    channels   jsonb NOT NULL DEFAULT '[]'::jsonb,
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alert_rules_route_idx ON alert_rules (route_id);

CREATE TABLE audit_log (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id    uuid REFERENCES users (id) ON DELETE SET NULL,
    action      text NOT NULL,
    object_type text NOT NULL,
    object_id   text,
    diff        jsonb,
    ip          inet,
    at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_object_idx ON audit_log (object_type, object_id, at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id, at DESC);
CREATE INDEX audit_log_at_idx ON audit_log (at DESC);

CREATE FUNCTION audit_log_reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

CREATE TRIGGER audit_log_append_only
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_reject_mutation();
