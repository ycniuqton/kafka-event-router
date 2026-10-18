DROP TABLE IF EXISTS alert_event_acks;
DROP TABLE IF EXISTS alert_events;
DROP TYPE IF EXISTS alert_state;
ALTER TABLE alert_rules
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS name,
    ALTER COLUMN kind TYPE text USING kind::text;
DROP TYPE IF EXISTS alert_rule_kind;
