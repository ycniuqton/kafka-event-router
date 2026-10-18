-- 1010_access_control_teams: per-team production approval policy, cross-team connection grants
-- and pre-invited users (pending until first login matches them by email).

ALTER TABLE teams
    ADD COLUMN require_prod_approval boolean NOT NULL DEFAULT true;

CREATE TABLE team_connection_grants (
    team_id       uuid        NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    connection_id uuid        NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
    granted_by    uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, connection_id)
);
CREATE INDEX team_connection_grants_connection_idx ON team_connection_grants (connection_id);

ALTER TABLE users
    ADD COLUMN invited_at timestamptz,
    ADD COLUMN invited_by uuid REFERENCES users (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(email));
