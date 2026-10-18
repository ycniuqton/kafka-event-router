DROP INDEX IF EXISTS users_email_lower_idx;
ALTER TABLE users
    DROP COLUMN IF EXISTS invited_by,
    DROP COLUMN IF EXISTS invited_at;
DROP TABLE IF EXISTS team_connection_grants;
ALTER TABLE teams DROP COLUMN IF EXISTS require_prod_approval;
