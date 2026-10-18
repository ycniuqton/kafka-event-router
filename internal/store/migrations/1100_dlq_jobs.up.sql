-- 1100_dlq_jobs: bulk dead-letter retry/discard jobs (dead-letters design §6). Runners in every API instance claim
-- queued jobs with FOR UPDATE SKIP LOCKED; cursor + counts make a job resumable after a crash.

CREATE TYPE dlq_job_status AS ENUM ('queued', 'running', 'cancel_requested', 'cancelled', 'succeeded', 'failed');
CREATE TYPE dlq_job_action AS ENUM ('retry', 'discard');

CREATE TABLE dlq_jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id     uuid NOT NULL REFERENCES routes (id) ON DELETE CASCADE,
    action       dlq_job_action NOT NULL,
    selection    jsonb NOT NULL,
    status       dlq_job_status NOT NULL DEFAULT 'queued',
    total        integer NOT NULL DEFAULT 0,
    processed    integer NOT NULL DEFAULT 0,
    counts       jsonb NOT NULL DEFAULT '{}'::jsonb,
    cursor       text,
    error        text,
    requested_by uuid REFERENCES users (id) ON DELETE SET NULL,
    heartbeat_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    started_at   timestamptz,
    finished_at  timestamptz
);

CREATE INDEX dlq_jobs_active_idx ON dlq_jobs (status) WHERE status IN ('queued', 'running', 'cancel_requested');
CREATE INDEX dlq_jobs_route_idx ON dlq_jobs (route_id, created_at DESC);
