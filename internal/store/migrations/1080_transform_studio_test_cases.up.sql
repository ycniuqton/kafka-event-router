-- 1080_transform_studio_test_cases: saved test cases of a route branch (transform-studio P2).
-- A case locks an expected outcome; expected_output is only required when the outcome is "passed".

ALTER TABLE test_cases
    ADD COLUMN branch_id        text NOT NULL DEFAULT 'main',
    ADD COLUMN expected_outcome text NOT NULL DEFAULT 'passed',
    ADD COLUMN created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN updated_at       timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN row_version      bigint NOT NULL DEFAULT 1,
    ALTER COLUMN expected_output DROP NOT NULL,
    ADD CONSTRAINT test_cases_outcome_chk CHECK (expected_outcome IN ('passed', 'filtered', 'dropped', 'error')),
    ADD CONSTRAINT test_cases_output_chk CHECK (expected_outcome <> 'passed' OR expected_output IS NOT NULL);

CREATE INDEX test_cases_route_created_idx ON test_cases (route_id, created_at, id) WHERE route_id IS NOT NULL;

CREATE TRIGGER test_cases_bump_row_version BEFORE UPDATE ON test_cases
    FOR EACH ROW EXECUTE FUNCTION bump_row_version();
