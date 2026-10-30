# access-control — Requirements (00001-password-auth)

Incremental version on top of `00000-init`. **Unchanged:** every FR/NFR of `00000-init/01-requirements.md`
(roles, bindings, teams, grants, approvals, masking, audit read) still applies. This version adds a
**password login mode** for installs without an identity provider (the "lite" profile, see
`docs/systems/platform-core/00002-lite-profile`). Code: `internal/auth/password`, `internal/rbac/password_users.go`,
`internal/rbac/http/password_users_handler.go`, `cmd/api/wire_password.go`, `web/src` (login, gate, Settings → Users).

## Business intent
Small teams want to run the router with only Postgres + Redis. They have no OIDC provider, yet still need
named accounts with roles, audited actions and safe credentials. Password login must be as safe as the
OIDC path: no self-registration, no default passwords left in place, brute force throttled, every
administrative action audited.

## Functional requirements
- PW-01 `ROUTER_AUTH_MODES=password` enables email + password login; combinable with `api_token` and `oidc`.
  Allowed in every environment, including production. With `oidc` absent no OIDC setting is required.
- PW-02 `/auth/config` lists `password` in `modes`, adds `password_login_url` and `password_policy.min_length`;
  the login page shows the email/password form when the mode is on and keeps the OIDC button when `oidc` is on.
- PW-03 `POST /auth/password/login {email, password}` → 204 + the same session / CSRF cookies as OIDC.
  Failure is always the generic 401 `invalid_credentials` (unknown email and wrong password look the same,
  including timing). Disabled users get the existing 403.
- PW-04 Throttle: per client IP (`ROUTER_LOGIN_IP_PER_MINUTE`, default 20/min) and per account
  (`ROUTER_LOGIN_MAX_FAILURES`, default 5 failures within `ROUTER_LOGIN_LOCKOUT`, default 15 min, locks the
  account for the rest of that window) → 429 `login_locked` + `Retry-After`. Shared by API replicas (Redis);
  Redis down → login fails closed. Every success / failure is audited (`auth.login`, `auth.login_failed`).
- PW-05 No self-registration, ever: no unauthenticated route creates users. Credentials are created only by
  the first-start seed or by a workspace admin.
- PW-06 First start: when no password credential exists, the API creates the admin from
  `ROUTER_BOOTSTRAP_ADMIN_EMAIL` + `ROUTER_BOOTSTRAP_ADMIN_PASSWORD` (or `_FILE`, which wins) as workspace
  admin + admin of the default team, flagged must-change. Runs once (advisory lock; safe with replicas);
  email set without password, or a password below the policy, refuses to start. No email → warning only.
- PW-07 Forced change: while must-change is set, every route except `GET /api/v1/auth/me`,
  `POST /api/v1/auth/password`, `POST /auth/logout`, `GET /auth/config` returns 403
  `password_change_required`; `/auth/me` reports `must_change_password`. The UI shows only the change screen.
- PW-08 Self-service `POST /api/v1/auth/password {current_password, new_password}` (current required; codes
  `invalid_current_password`, `weak_password`, `password_reused`). Success revokes all sessions of the user
  and issues a fresh session to the calling browser. Audited `auth.password_changed`.
- PW-09 Workspace admins (`user.manage`): `POST /api/v1/users` creates a user (email, display name, optional
  workspace admin, optional team + role via the existing role bindings, initial password or a generated one
  returned once as `generatedPassword`); always must-change. `POST /api/v1/users/{id}/password-reset`
  sets a given or generated password, forces a change and revokes the user's sessions.
  Disable / enable stays `PATCH /api/v1/users/{id} {disabled}` (revokes sessions, last-admin guard).
  Audited `user.created`, `user.password_reset`, `user.updated`. These routes exist only in password mode.
- PW-10 Users list / detail show `passwordLogin` and `mustChangePassword`.
- PW-11 Password policy: 12 ≤ length ≤ 256 characters (counted in characters, any character allowed).

## Non-functional requirements
- NFR-PW-1 Hashes: argon2id (x/crypto), PHC string with its own parameters (19 MiB, t=2, p=1 baseline), random
  16-byte salt; verification in constant time; the hasher set is keyed by algorithm so the scheme can change
  without invalidating stored hashes.
- NFR-PW-2 Passwords never appear in logs, audit diffs or API responses (except the one-time generated value).
- NFR-PW-3 Generated passwords: 20 characters from a 58-symbol unambiguous alphabet (~117 bits), crypto/rand.

## Out of scope
Password recovery by email, MFA, password expiry/history beyond "not the same as current", admin unlock of a
locked account (the lock expires on its own; an admin reset does not clear it).
