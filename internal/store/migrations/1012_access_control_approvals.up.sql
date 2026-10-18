-- 1012_access_control_approvals: an approval is a deployments row in pending_approval; this adds the
-- decision columns, its If-Match token, one pending request per route, and the review comments.

ALTER TABLE deployments
    ADD COLUMN decision_note text,
    ADD COLUMN rejected_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN rejected_at   timestamptz,
    ADD COLUMN cancel_reason text,
    ADD COLUMN row_version   bigint NOT NULL DEFAULT 1;

CREATE FUNCTION deployments_bump_row_version() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    NEW.row_version := OLD.row_version + 1;
    RETURN NEW;
END;
$$;
CREATE TRIGGER deployments_bump_row_version BEFORE UPDATE ON deployments
    FOR EACH ROW EXECUTE FUNCTION deployments_bump_row_version();

CREATE UNIQUE INDEX deployments_one_pending_idx ON deployments (route_id) WHERE status = 'pending_approval';

CREATE TABLE approval_comments (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid        NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    author_id     uuid        REFERENCES users (id) ON DELETE SET NULL,
    body          text        NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_comments_deployment_idx ON approval_comments (deployment_id, created_at);
