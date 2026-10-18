-- 1090_monitoring_alerts: typed alert rule kinds, rule name/creator, alert history (firing → resolved),
-- pending notification bookkeeping and per-user acknowledgements for the bell.

CREATE TYPE alert_rule_kind AS ENUM ('lag_above', 'error_rate_above', 'throughput_zero', 'match_rate_change');

DELETE FROM alert_rules
 WHERE kind NOT IN ('lag_above', 'error_rate_above', 'throughput_zero', 'match_rate_change');

ALTER TABLE alert_rules
    ALTER COLUMN kind TYPE alert_rule_kind USING kind::alert_rule_kind,
    ADD COLUMN name text NOT NULL DEFAULT '',
    ADD COLUMN created_by uuid REFERENCES users (id) ON DELETE SET NULL;

CREATE TYPE alert_state AS ENUM ('firing', 'resolved');

CREATE TABLE alert_events (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_id         uuid NOT NULL REFERENCES alert_rules (id) ON DELETE CASCADE,
    route_id        uuid NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    state           alert_state NOT NULL,
    value           double precision,
    fired_at        timestamptz NOT NULL,
    resolved_at     timestamptz,
    notify_pending  alert_state,
    notify_attempts integer NOT NULL DEFAULT 0,
    notified_at     timestamptz,
    notify_error    text,
    acked_by        uuid REFERENCES users (id) ON DELETE SET NULL,
    acked_at        timestamptz
);
CREATE UNIQUE INDEX alert_events_one_firing ON alert_events (rule_id) WHERE state = 'firing';
CREATE INDEX alert_events_recent ON alert_events (fired_at DESC, id DESC);
CREATE INDEX alert_events_pending ON alert_events (id) WHERE notify_pending IS NOT NULL;

CREATE TABLE alert_event_acks (
    event_id bigint NOT NULL REFERENCES alert_events (id) ON DELETE CASCADE,
    user_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    acked_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id)
);
