# access-control — Design (00001-password-auth)

Incremental version on top of `00000-init`. **Unchanged:** RBAC engine, role matrix, bindings, teams, grants,
invitations, approvals, Bootstrapper rules, audit read side, session store and cookie/CSRF machinery
(platform-core). This version adds one login flow, one route stage, two admin routes and one table.

## 1. Components

```
internal/auth/password            (credentials domain; imports auth, store, ratelimit — never rbac)
  Hasher (base) ── Argon2idHasher            HasherSet: Manager, verify by PHC algorithm prefix
  Policy, Generator                          length window 12..256, one-time generated passwords
  CredentialStore (base) ── PostgresCredentialStore   table user_passwords
  Throttle (base) ── RedisThrottle           per-IP GCRA (platform limiter) + per-account failure counter
  Flow                                       auth.LoginFlow for mode "password"
  Credentials                                Manager: Assign (seed/admin), Change (self), Seeded
  AdminSeeder + PostgresSeedAccounts         first-start admin
  GateStage                                  api.RouteMiddleware: forced change
  httpapi/                                   POST /auth/password/login, POST /api/v1/auth/password,
                                             /auth/me + /auth/config contributors
internal/rbac
  PasswordUserService + PostgresPasswordAccountStore    create user, reset password
  http/password_users_handler.go             POST /api/v1/users, POST /api/v1/users/{id}/password-reset
cmd/api/wire_password.go                     PasswordAssembly (flow registered in AuthAssembly.flows),
                                             buildPassword step (after buildAccess), gate, modules
```

Selection is configuration only: `auth.Registry` maps mode `password` to the session-cookie authenticator;
`AuthAssembly.flows` maps it to `PasswordAssembly.Flow`; rbac admin routes mount when `Deps.Passwords != nil`.

## 2. Data

Migration `3110_access_control_passwords` (numbered after `3100` because databases already at 3100 would
skip a lower number; access-control's block-of-ten rule would give 3010):

```
user_passwords(user_id uuid PK → users ON DELETE CASCADE, hash text, must_change bool default true,
               changed_at, created_at)
```

Password users get `users.sso_subject = 'password:<user id>'` so the existing `UserProvisioner.Upsert`
(by subject) and the Bootstrapper work unchanged. Resetting the password of an invited user (subject NULL)
gives it that subject. `identity.UserRepository.LoadUser` reads `must_change`; `auth.User` and
`auth.Principal` carry `MustChangePassword`; `rbac.User` adds `passwordLogin`, `mustChangePassword`.

## 3. Flows

```
POST /auth/password/login {email, password}                     (AuthNone; global per-IP rate limit first)
└─ LoginManager.Complete(password)
   └─ Flow.Complete
      ├─ body invalid → 422 (/email, /password)                  → auth.login_failed
      └─ RedisThrottle.Admit
         ├─ client over ROUTER_LOGIN_IP_PER_MINUTE → 429 login_locked + Retry-After → auth.login_failed
         ├─ account failures ≥ MAX_FAILURES (key login_fail:<sha256(email)>) → 429 + Retry-After = key TTL
         ├─ Redis error → 503 (fail closed)
         └─ CredentialStore.FindByEmail (case-insensitive)
            ├─ not found → verify against dummy hash (same cost) → 401 invalid_credentials, Failed++
            ├─ hash mismatch → 401 invalid_credentials, Failed++ (first failure starts the window)
            └─ match → Succeeded (counter deleted) → Identity{password, subject, email}
   └─ provision (tx): Upsert by subject → disabled? → 403 user disabled → MarkLogin → Bootstrapper.OnLogin
   └─ session created → er_session + er_csrf cookies → auth.login → 204
---
Any routed request with a session cookie
└─ AuthenticateStage → CookieAuthenticator → LoadUser (must_change from user_passwords)
   └─ GateStage
      ├─ route AuthNone or in ChangeAllowed → pass
      ├─ principal.MustChangePassword → 403 password_change_required
      └─ else → RateLimit → CSRF → Authorize → Scope → Audit → handler
---
POST /api/v1/auth/password {current_password, new_password}    (session + CSRF; allowed during the gate)
└─ ChangeHandler (user principals only, API tokens → 403)
   └─ Credentials.Change
      ├─ no credential / wrong current → 422 invalid_current_password
      ├─ policy fails → 422 weak_password;  next == current → 422 password_reused
      └─ tx: user_passwords.hash = argon2id(next), must_change = false → auth.password_changed
   └─ LoginManager.Reissue: RevokeAllForUser → new session → new cookies → 204 (UI re-reads /auth/me)
---
API start (password in ROUTER_AUTH_MODES), step buildPassword after buildAccess
└─ AdminSeeder.Run (tx, pg_advisory_xact_lock 7002 shared with identity bootstrap)
   ├─ a credential exists → nothing
   ├─ ROUTER_BOOTSTRAP_ADMIN_EMAIL empty → warn log, nothing
   ├─ password missing / below policy → start fails
   └─ find-or-create user by email → Assign(must_change) → is_workspace_admin → admin on default team
      → user.bootstrap_admin
---
POST /api/v1/users {email, displayName, password?, workspaceAdmin?, teamId?, role?}   (user.manage)
└─ PasswordUserService.Create
   ├─ invalid email / name / role not assignable / weak password → 422
   └─ tx: team exists? (422 unknown_team) → email exists? (409 user_exists)
      → insert user (subject password:<id>) → Assign(given | generated, must_change)
      → role binding → user.created (no password in the diff) → 201 {user, generatedPassword?}
---
POST /api/v1/users/{id}/password-reset {password?}                                    (user.manage)
└─ PasswordUserService.Reset
   └─ tx: user exists (404) → Assign(given | generated, must_change) → user.password_reset
   └─ RevokeAllForUser → 200 {generatedPassword?}
---
PATCH /api/v1/users/{id} {disabled}  (unchanged UserService) → disabled: RevokeAllForUser; login → 403
```

## 4. Decisions
- **Login flow, not a new authenticator:** password login only proves the identity; sessions, cookies, CSRF,
  disabled checks, login audit and the Bootstrapper are reused as-is.
- **Throttle in the password package:** `ratelimit` imports `auth`, so `auth` cannot use it; the password
  package can. The per-account counter is a fixed window (`INCR` + `PEXPIRE` on first failure); lockout is a
  deliberate DoS trade-off (documented, no admin unlock in this version).
- **Gate as a route stage with an allow list** (`password.ChangeAllowed`) instead of a new RouteSpec flag:
  no change to every feature handler; public routes pass untouched.
- **Session renewal after change** (`LoginManager.Reissue`): signs other devices out without logging the
  current browser out; the UI re-reads `/auth/me` for the new CSRF token.
- **Admin routes in rbac** next to the users API (same JSON casing, `user.manage`, AdminGuard semantics for
  disable); credential logic stays in `internal/auth/password` (rbac depends on it, not the reverse).
- **Bootstrap reuses users with the same email** (e.g. an OIDC user) and attaches a credential to it.
- OIDC and password together: the same email cannot be both a password user and a different OIDC subject
  (unique email) — the OIDC callback reports `conflict` as before.

## 5. Tests
Unit (`internal/auth/password`): hasher, malformed hashes, stored-parameter verify, policy, generator, flow
(success, generic failure for unknown/wrong, lock skips verify, validation, no Begin), Redis throttle on
miniredis (lockout + expiry, success clears, per-IP rate, Redis down fails closed), credentials (assign,
change rules, audit), gate, seeder (once, skip, refuse, file wins). `internal/rbac/http`: routes only in
password mode, `user.manage`. `internal/platform/config`: production + password without OIDC, bad lockout.
Integration (`cmd/api/password_integration_test.go`, testkit Postgres sandbox + Redis, production wiring
steps, responses checked against OpenAPI): bootstrap + forced change + session revocation + reseed,
failures + lockout + audit, admin create/duplicate/weak/reset/disable/enable + revocation, no registration.
