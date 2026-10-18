-- 1070_preview_test_topics: temporary router.test.<route>.<ts> topics written by preview test runs (live-preview LP-108);
-- the preview TestTopicJanitor deletes them on the cluster after expires_at and stamps deleted_at.

CREATE TABLE IF NOT EXISTS preview_test_topics (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id      uuid REFERENCES routes (id) ON DELETE SET NULL,
    connection_id uuid NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
    topic         text NOT NULL,
    produced      integer NOT NULL DEFAULT 0,
    created_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    deleted_at    timestamptz,
    UNIQUE (connection_id, topic)
);

CREATE INDEX IF NOT EXISTS preview_test_topics_expiry ON preview_test_topics (expires_at) WHERE deleted_at IS NULL;
