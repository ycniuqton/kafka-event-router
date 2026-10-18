-- 1011_access_control_approval_status: approval decisions (kept apart: enum values cannot be used
-- in the transaction that adds them).

ALTER TYPE deployment_status ADD VALUE IF NOT EXISTS 'rejected';
ALTER TYPE deployment_status ADD VALUE IF NOT EXISTS 'cancelled';
