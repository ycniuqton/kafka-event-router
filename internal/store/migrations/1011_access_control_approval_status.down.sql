-- Postgres cannot drop enum values; rows using them are moved back to superseded instead.
UPDATE deployments SET status = 'superseded' WHERE status::text IN ('rejected', 'cancelled');
