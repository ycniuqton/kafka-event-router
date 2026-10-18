-- 0008_route_runtime: worker placement/drain columns, per-replica lease status and rollout target,
-- route halt state, deployments.activated_at, and the router_runtime NOTIFY trigger for fast reconciliation.

ALTER TABLE workers
    ADD COLUMN zone          text NOT NULL DEFAULT 'default',
    ADD COLUMN capacity_used double precision NOT NULL DEFAULT 0,
    ADD COLUMN draining      boolean NOT NULL DEFAULT false,
    ADD COLUMN started_at    timestamptz NOT NULL DEFAULT now();

ALTER TABLE route_assignments
    ADD COLUMN target_version_id   uuid REFERENCES route_versions (id) ON DELETE SET NULL,
    ADD COLUMN reported_version_id uuid REFERENCES route_versions (id) ON DELETE SET NULL,
    ADD COLUMN runner_state        text,
    ADD COLUMN error_category      text,
    ADD COLUMN error_message       text,
    ADD COLUMN lag                 bigint NOT NULL DEFAULT 0,
    ADD COLUMN partitions          integer NOT NULL DEFAULT 0,
    ADD COLUMN reported_at         timestamptz,
    ADD COLUMN created_at          timestamptz NOT NULL DEFAULT now();
CREATE INDEX route_assignments_target_idx ON route_assignments (route_id, target_version_id);

ALTER TABLE route_runtime
    ADD COLUMN running_version_id uuid REFERENCES route_versions (id) ON DELETE SET NULL,
    ADD COLUMN halted_at          timestamptz,
    ADD COLUMN halt_reason        text,
    ADD COLUMN halted_version_id  uuid REFERENCES route_versions (id) ON DELETE SET NULL,
    ADD COLUMN error_category     text,
    ADD COLUMN partitions         integer NOT NULL DEFAULT 0,
    ADD COLUMN lag                bigint NOT NULL DEFAULT 0;
CREATE INDEX route_runtime_halted_idx ON route_runtime (halted_at) WHERE halted_at IS NOT NULL;

ALTER TABLE deployments
    ADD COLUMN activated_at timestamptz;
CREATE INDEX deployments_rolling_idx ON deployments (route_id) WHERE status = 'rolling_out';

CREATE FUNCTION router_runtime_notify() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    rec   record;
    route uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        rec := OLD;
    ELSE
        rec := NEW;
    END IF;
    IF TG_TABLE_NAME = 'routes' THEN
        route := rec.id;
    ELSE
        route := rec.route_id;
    END IF;
    PERFORM pg_notify('router_runtime', json_build_object('route_id', route, 'kind', TG_ARGV[0])::text);
    RETURN NULL;
END;
$$;

CREATE TRIGGER routes_runtime_notify
    AFTER INSERT OR DELETE OR UPDATE OF desired_state, active_version_id, replicas ON routes
    FOR EACH ROW EXECUTE FUNCTION router_runtime_notify('route');

CREATE TRIGGER route_assignments_runtime_notify
    AFTER INSERT OR DELETE ON route_assignments
    FOR EACH ROW EXECUTE FUNCTION router_runtime_notify('assignment');

CREATE TRIGGER route_assignments_runtime_notify_update
    AFTER UPDATE ON route_assignments
    FOR EACH ROW
    WHEN (OLD.target_version_id IS DISTINCT FROM NEW.target_version_id
       OR OLD.reported_version_id IS DISTINCT FROM NEW.reported_version_id
       OR OLD.runner_state IS DISTINCT FROM NEW.runner_state)
    EXECUTE FUNCTION router_runtime_notify('assignment');

CREATE TRIGGER deployments_runtime_notify
    AFTER INSERT OR UPDATE OF status ON deployments
    FOR EACH ROW EXECUTE FUNCTION router_runtime_notify('deployment');

CREATE TRIGGER route_runtime_runtime_notify
    AFTER INSERT ON route_runtime
    FOR EACH ROW EXECUTE FUNCTION router_runtime_notify('runtime');

CREATE TRIGGER route_runtime_runtime_notify_update
    AFTER UPDATE ON route_runtime
    FOR EACH ROW
    WHEN (OLD.observed_state IS DISTINCT FROM NEW.observed_state
       OR OLD.halted_at IS DISTINCT FROM NEW.halted_at
       OR OLD.running_version_id IS DISTINCT FROM NEW.running_version_id)
    EXECUTE FUNCTION router_runtime_notify('runtime');
