-- 0008_route_runtime (down): drop the router_runtime NOTIFY triggers and every column 0008 added.

DROP TRIGGER IF EXISTS route_runtime_runtime_notify_update ON route_runtime;
DROP TRIGGER IF EXISTS route_runtime_runtime_notify ON route_runtime;
DROP TRIGGER IF EXISTS deployments_runtime_notify ON deployments;
DROP TRIGGER IF EXISTS route_assignments_runtime_notify_update ON route_assignments;
DROP TRIGGER IF EXISTS route_assignments_runtime_notify ON route_assignments;
DROP TRIGGER IF EXISTS routes_runtime_notify ON routes;
DROP FUNCTION IF EXISTS router_runtime_notify();

DROP INDEX IF EXISTS deployments_rolling_idx;
ALTER TABLE deployments
    DROP COLUMN IF EXISTS activated_at;

DROP INDEX IF EXISTS route_runtime_halted_idx;
ALTER TABLE route_runtime
    DROP COLUMN IF EXISTS lag,
    DROP COLUMN IF EXISTS partitions,
    DROP COLUMN IF EXISTS error_category,
    DROP COLUMN IF EXISTS halted_version_id,
    DROP COLUMN IF EXISTS halt_reason,
    DROP COLUMN IF EXISTS halted_at,
    DROP COLUMN IF EXISTS running_version_id;

DROP INDEX IF EXISTS route_assignments_target_idx;
ALTER TABLE route_assignments
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS reported_at,
    DROP COLUMN IF EXISTS partitions,
    DROP COLUMN IF EXISTS lag,
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS error_category,
    DROP COLUMN IF EXISTS runner_state,
    DROP COLUMN IF EXISTS reported_version_id,
    DROP COLUMN IF EXISTS target_version_id;

ALTER TABLE workers
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS draining,
    DROP COLUMN IF EXISTS capacity_used,
    DROP COLUMN IF EXISTS zone;
