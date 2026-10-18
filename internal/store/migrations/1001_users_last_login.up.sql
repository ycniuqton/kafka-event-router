-- 1001_users_last_login: login tracking on users. Writing last_login_at alone must not bump
-- row_version (If-Match on PATCH /users stays valid), so the users trigger ignores that column.

ALTER TABLE users ADD COLUMN last_login_at timestamptz;

DROP TRIGGER IF EXISTS users_bump_row_version ON users;
CREATE TRIGGER users_bump_row_version BEFORE UPDATE ON users
    FOR EACH ROW
    WHEN ((to_jsonb(OLD) - 'last_login_at') IS DISTINCT FROM (to_jsonb(NEW) - 'last_login_at'))
    EXECUTE FUNCTION bump_row_version();
