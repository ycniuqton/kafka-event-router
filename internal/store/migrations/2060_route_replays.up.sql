-- route-runtime P3 (RR-202): replay coordination and audit. routes' POST /routes/{id}/replay inserts a pending
-- row through route-runtime's ReplayService; the leader's ReplayCoordinator moves it through
-- stopping -> resetting -> done | failed. At most one open replay per route.
CREATE TYPE replay_status AS ENUM ('pending', 'stopping', 'resetting', 'done', 'failed');

CREATE TABLE route_replays (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id       uuid NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    from_ts        timestamptz NOT NULL,
    status         replay_status NOT NULL DEFAULT 'pending',
    requested_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    requested_at   timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz,
    error          text,
    consumer_group text,
    partitions     integer NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX route_replays_one_open ON route_replays (route_id)
    WHERE status IN ('pending', 'stopping', 'resetting');
CREATE INDEX route_replays_route_requested ON route_replays (route_id, requested_at DESC);

CREATE TRIGGER route_replays_runtime_notify
    AFTER INSERT OR UPDATE OF status ON route_replays
    FOR EACH ROW EXECUTE FUNCTION router_runtime_notify('replay');
