-- 0006_team_hash_salt (down): restore the unconditional row_version trigger and drop the salt reference
-- (the referenced secrets stay until purged).

DROP TRIGGER IF EXISTS teams_bump_row_version ON teams;
CREATE TRIGGER teams_bump_row_version BEFORE UPDATE ON teams
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();

ALTER TABLE teams
    DROP COLUMN IF EXISTS hash_salt_ref;
