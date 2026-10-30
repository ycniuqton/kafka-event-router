-- 3100_dlq_workflow: dead-letter workflow (dead-letters 00001-workflow §7). Bulk/edited replays can wait for a
-- second person (strict 4-eyes) and run rate-limited; a pending edited replay keeps its edit until it ends.
-- Alerts get the router-caused dead-letter kind and a per-route provisioning ledger (default rules are created
-- once per route; a deleted rule is never re-created).

ALTER TYPE dlq_job_status ADD VALUE IF NOT EXISTS 'pending_approval';
ALTER TYPE dlq_job_status ADD VALUE IF NOT EXISTS 'rejected';

ALTER TABLE dlq_jobs
    ADD COLUMN rate_per_second integer NOT NULL DEFAULT 50 CHECK (rate_per_second BETWEEN 1 AND 1000),
    ADD COLUMN edit            jsonb,
    ADD COLUMN note            text NOT NULL DEFAULT '',
    ADD COLUMN decided_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN decided_at      timestamptz,
    ADD COLUMN decision_note   text NOT NULL DEFAULT '';

ALTER TYPE alert_rule_kind ADD VALUE IF NOT EXISTS 'router_caused_dead_letters';

CREATE TABLE alert_rule_provisions (
    route_id       uuid            NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    kind           alert_rule_kind NOT NULL,
    provisioned_at timestamptz     NOT NULL DEFAULT now(),
    PRIMARY KEY (route_id, kind)
);
