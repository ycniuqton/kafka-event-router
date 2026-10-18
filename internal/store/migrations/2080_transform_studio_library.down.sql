DROP INDEX IF EXISTS route_versions_config_gin;
DROP INDEX IF EXISTS lookup_tables_name_idx;
DROP INDEX IF EXISTS transforms_name_idx;

ALTER TABLE lookup_table_versions
    DROP COLUMN IF EXISTS row_count,
    DROP COLUMN IF EXISTS change_note,
    DROP COLUMN IF EXISTS config_hash;

ALTER TABLE lookup_tables
    DROP CONSTRAINT IF EXISTS lookup_tables_key_type_chk,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS key_type,
    DROP COLUMN IF EXISTS description;

ALTER TABLE transform_versions
    DROP COLUMN IF EXISTS change_note,
    DROP COLUMN IF EXISTS config_hash;

ALTER TABLE transforms
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS description;
