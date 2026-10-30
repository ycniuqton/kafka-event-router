-- Postgres cannot drop enum values: jobs in the new statuses end as cancelled, provisioned router-caused
-- alert rules are removed; the enum values themselves stay.
DROP TABLE IF EXISTS alert_rule_provisions;
DELETE FROM alert_rules WHERE kind::text = 'router_caused_dead_letters';

UPDATE dlq_jobs SET status = 'cancelled', finished_at = COALESCE(finished_at, now())
 WHERE status::text IN ('pending_approval', 'rejected');
ALTER TABLE dlq_jobs
    DROP COLUMN IF EXISTS decision_note,
    DROP COLUMN IF EXISTS decided_at,
    DROP COLUMN IF EXISTS decided_by,
    DROP COLUMN IF EXISTS note,
    DROP COLUMN IF EXISTS edit,
    DROP COLUMN IF EXISTS rate_per_second;
