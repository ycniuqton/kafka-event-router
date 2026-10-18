DROP TABLE IF EXISTS approval_comments;
DROP INDEX IF EXISTS deployments_one_pending_idx;
DROP TRIGGER IF EXISTS deployments_bump_row_version ON deployments;
DROP FUNCTION IF EXISTS deployments_bump_row_version();
ALTER TABLE deployments
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS rejected_at,
    DROP COLUMN IF EXISTS rejected_by,
    DROP COLUMN IF EXISTS decision_note;
