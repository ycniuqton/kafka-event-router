-- 0001_init (down): drop the control-plane schema in reverse dependency order.

DROP TRIGGER IF EXISTS audit_log_append_only ON audit_log;
DROP FUNCTION IF EXISTS audit_log_reject_mutation();

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS alert_rules;
DROP TABLE IF EXISTS route_assignments;
DROP TABLE IF EXISTS workers;
DROP TABLE IF EXISTS test_cases;
DROP TABLE IF EXISTS lookup_table_versions;
DROP TABLE IF EXISTS lookup_tables;
DROP TABLE IF EXISTS transform_versions;
DROP TABLE IF EXISTS transforms;
DROP TABLE IF EXISTS route_runtime;
DROP TABLE IF EXISTS deployments;
ALTER TABLE IF EXISTS routes DROP CONSTRAINT IF EXISTS routes_active_version_fk;
DROP TABLE IF EXISTS route_versions;
DROP TABLE IF EXISTS routes;
DROP TABLE IF EXISTS connections;
DROP TABLE IF EXISTS role_bindings;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS teams;

DROP TYPE IF EXISTS deployment_status;
DROP TYPE IF EXISTS route_observed_state;
DROP TYPE IF EXISTS route_desired_state;
DROP TYPE IF EXISTS connection_health;
DROP TYPE IF EXISTS connection_security_mode;
DROP TYPE IF EXISTS team_role;
