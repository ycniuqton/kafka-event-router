-- 0007_routes_lifecycle (down): drop deployment, version and route lifecycle columns.

ALTER TABLE deployments
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS previous_version_id;

ALTER TABLE route_versions
    DROP COLUMN IF EXISTS based_on_version_id,
    DROP COLUMN IF EXISTS origin;

DROP TYPE IF EXISTS route_version_origin;

DROP INDEX IF EXISTS routes_updated_idx;
DROP INDEX IF EXISTS routes_team_updated_idx;
ALTER TABLE routes
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS description;
