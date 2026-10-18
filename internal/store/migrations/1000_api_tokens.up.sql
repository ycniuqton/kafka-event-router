-- 1000_api_tokens: user-owned API tokens for automation (Authorization: Bearer ert_…).
-- Only the SHA-256 of a token is stored; the plaintext is shown once at creation.

CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text        NOT NULL,
    token_hash   bytea       NOT NULL,
    prefix       text        NOT NULL,
    scopes       text[]      NOT NULL DEFAULT '{}',
    expires_at   timestamptz,
    last_used_at timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT api_tokens_token_hash_key UNIQUE (token_hash),
    CONSTRAINT api_tokens_name_length CHECK (char_length(name) BETWEEN 1 AND 100)
);

CREATE UNIQUE INDEX api_tokens_user_name_active_key ON api_tokens (user_id, name) WHERE revoked_at IS NULL;
CREATE INDEX api_tokens_user_created_idx ON api_tokens (user_id, created_at DESC, id DESC);
