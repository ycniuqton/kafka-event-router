-- 0002_row_version (down): drop the row_version triggers, columns and function.

DROP TRIGGER IF EXISTS alert_rules_bump_row_version ON alert_rules;
DROP TRIGGER IF EXISTS lookup_tables_bump_row_version ON lookup_tables;
DROP TRIGGER IF EXISTS transforms_bump_row_version ON transforms;
DROP TRIGGER IF EXISTS routes_bump_row_version ON routes;
DROP TRIGGER IF EXISTS connections_bump_row_version ON connections;
DROP TRIGGER IF EXISTS users_bump_row_version ON users;
DROP TRIGGER IF EXISTS teams_bump_row_version ON teams;

ALTER TABLE alert_rules   DROP COLUMN IF EXISTS row_version;
ALTER TABLE lookup_tables DROP COLUMN IF EXISTS row_version;
ALTER TABLE transforms    DROP COLUMN IF EXISTS row_version;
ALTER TABLE routes        DROP COLUMN IF EXISTS row_version;
ALTER TABLE connections   DROP COLUMN IF EXISTS row_version;
ALTER TABLE users         DROP COLUMN IF EXISTS row_version;
ALTER TABLE teams         DROP COLUMN IF EXISTS row_version;

DROP FUNCTION IF EXISTS bump_row_version();
