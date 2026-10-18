-- 2080_transform_studio_library: named, versioned library objects (transform-studio P3).
-- Metadata (description, key type, archive; row_version + triggers exist since 0002), version hash / change note,
-- lookup row counts, global name lookups and the GIN index used by usage queries.

ALTER TABLE transforms
    ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

ALTER TABLE transform_versions
    ADD COLUMN IF NOT EXISTS config_hash text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS change_note text NOT NULL DEFAULT '';

ALTER TABLE lookup_tables
    ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS key_type text NOT NULL DEFAULT 'string',
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

ALTER TABLE lookup_tables DROP CONSTRAINT IF EXISTS lookup_tables_key_type_chk;
ALTER TABLE lookup_tables
    ADD CONSTRAINT lookup_tables_key_type_chk CHECK (key_type IN ('string', 'number', 'boolean'));

ALTER TABLE lookup_table_versions
    ADD COLUMN IF NOT EXISTS config_hash text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS change_note text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS row_count integer GENERATED ALWAYS AS (jsonb_array_length(rows)) STORED;

CREATE INDEX IF NOT EXISTS transforms_name_idx ON transforms (name);
CREATE INDEX IF NOT EXISTS lookup_tables_name_idx ON lookup_tables (name);
CREATE INDEX IF NOT EXISTS route_versions_config_gin ON route_versions USING gin (config jsonb_path_ops);

