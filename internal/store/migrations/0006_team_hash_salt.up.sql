-- 0006_team_hash_salt: per-team hash salt for ctx.hash in JS steps; the salt itself lives in the secrets
-- table and is generated on first use, teams only keep its reference (js-sandbox design §3).
-- Provisioning the salt is not a team edit, so it does not bump the team's If-Match row_version.

ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS hash_salt_ref text;

DROP TRIGGER IF EXISTS teams_bump_row_version ON teams;
CREATE TRIGGER teams_bump_row_version BEFORE UPDATE ON teams
    FOR EACH ROW WHEN (OLD.hash_salt_ref IS NOT DISTINCT FROM NEW.hash_salt_ref)
    EXECUTE FUNCTION bump_row_version();
