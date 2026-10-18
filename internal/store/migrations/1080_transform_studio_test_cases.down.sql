-- 1080_transform_studio_test_cases (down): back to the 0001_init test_cases columns.

DROP TRIGGER IF EXISTS test_cases_bump_row_version ON test_cases;
DROP INDEX IF EXISTS test_cases_route_created_idx;

UPDATE test_cases SET expected_output = 'null'::jsonb WHERE expected_output IS NULL;

ALTER TABLE test_cases
    DROP CONSTRAINT IF EXISTS test_cases_output_chk,
    DROP CONSTRAINT IF EXISTS test_cases_outcome_chk,
    ALTER COLUMN expected_output SET NOT NULL,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS expected_outcome,
    DROP COLUMN IF EXISTS branch_id;
