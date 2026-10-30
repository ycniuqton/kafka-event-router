-- 3110_access_control_passwords: password credentials for ROUTER_AUTH_MODES=password. One row per user that may
-- sign in with a password; must_change forces a change before any other API call. Numbered after 3100 so
-- databases already at 3100 still apply it. Users are never created here by self-registration.

CREATE TABLE user_passwords (
    user_id     uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    hash        text        NOT NULL,
    must_change boolean     NOT NULL DEFAULT true,
    changed_at  timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now()
);
