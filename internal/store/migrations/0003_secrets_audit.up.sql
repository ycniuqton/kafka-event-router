-- 0003_secrets_audit: envelope-encrypted secrets (ciphertext + wrapped data key only)
-- and audit context columns (request id, team, actor kind).

CREATE TABLE secrets (
    ref         text PRIMARY KEY,
    provider    text NOT NULL,
    key_id      text NOT NULL,
    wrapped_key bytea NOT NULL,
    nonce       bytea NOT NULL,
    ciphertext  bytea NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    rotated_at  timestamptz
);
CREATE INDEX secrets_key_id_idx ON secrets (key_id, ref);

ALTER TABLE connections
    ADD CONSTRAINT connections_secret_ref_fk
    FOREIGN KEY (secret_ref) REFERENCES secrets (ref) ON DELETE SET NULL;

ALTER TABLE audit_log
    ADD COLUMN request_id text,
    ADD COLUMN team_id    uuid,
    ADD COLUMN actor_kind text NOT NULL DEFAULT 'user';
CREATE INDEX audit_log_team_idx ON audit_log (team_id, at DESC);
