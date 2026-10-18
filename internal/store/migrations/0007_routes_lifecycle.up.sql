-- 0007_routes_lifecycle: route authorship and description, version provenance (origin, based_on),
-- deployment rollback source and failure reason, and the list cursor index.

ALTER TABLE routes
    ADD COLUMN description text NOT NULL DEFAULT '',
    ADD COLUMN created_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN updated_by  uuid REFERENCES users (id) ON DELETE SET NULL;
CREATE INDEX routes_team_updated_idx ON routes (team_id, updated_at DESC, id);
CREATE INDEX routes_updated_idx ON routes (updated_at DESC, id);

CREATE TYPE route_version_origin AS ENUM ('deploy', 'rollback', 'auto_rollback', 'import');

ALTER TABLE route_versions
    ADD COLUMN origin              route_version_origin NOT NULL DEFAULT 'deploy',
    ADD COLUMN based_on_version_id uuid REFERENCES route_versions (id) ON DELETE SET NULL;

ALTER TABLE deployments
    ADD COLUMN previous_version_id uuid REFERENCES route_versions (id) ON DELETE SET NULL,
    ADD COLUMN failure_reason      text;
