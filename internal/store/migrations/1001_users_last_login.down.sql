-- 1001_users_last_login (down): restore the unconditional users row_version trigger and drop the column.

DROP TRIGGER IF EXISTS users_bump_row_version ON users;
CREATE TRIGGER users_bump_row_version BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();

ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
