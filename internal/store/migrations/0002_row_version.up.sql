-- 0002_row_version: optimistic-concurrency token (If-Match / ETag) on every mutable table.
-- bump_row_version() increments row_version and refreshes updated_at on each UPDATE.

CREATE FUNCTION bump_row_version() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    NEW.row_version := OLD.row_version + 1;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

ALTER TABLE teams         ADD COLUMN row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE users         ADD COLUMN row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE connections   ADD COLUMN row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE routes        ADD COLUMN row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE transforms    ADD COLUMN row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE lookup_tables ADD COLUMN row_version bigint NOT NULL DEFAULT 1;
ALTER TABLE alert_rules   ADD COLUMN row_version bigint NOT NULL DEFAULT 1;

CREATE TRIGGER teams_bump_row_version BEFORE UPDATE ON teams
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
CREATE TRIGGER users_bump_row_version BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
CREATE TRIGGER connections_bump_row_version BEFORE UPDATE ON connections
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
CREATE TRIGGER routes_bump_row_version BEFORE UPDATE ON routes
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
CREATE TRIGGER transforms_bump_row_version BEFORE UPDATE ON transforms
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
CREATE TRIGGER lookup_tables_bump_row_version BEFORE UPDATE ON lookup_tables
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
CREATE TRIGGER alert_rules_bump_row_version BEFORE UPDATE ON alert_rules
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
