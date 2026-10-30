# access-control — Plan (00001-password-auth)

Incremental version on top of `00000-init`. **Unchanged:** every AC-xx item in `00000-init/03-plan.md` keeps its
state there. Mark the step you start `[~]`, finished `[x]`; never delete items (skipped → inline note).

## BE (password auth, `ROUTER_AUTH_MODES=password`)
- [x] AC-PW-01 Config: `AuthModePassword`, bootstrap admin email/password(_FILE), login lockout knobs — `internal/platform/config/password.go`
- [x] AC-PW-02 Migration `3110_access_control_passwords` (`user_passwords`) + down — `internal/store/migrations`
- [x] AC-PW-03 Password domain: Hasher base + argon2id, hasher registry, policy (≥ 12), generator, credential store — `internal/auth/password`
- [x] AC-PW-04 Login flow `POST /auth/password/login`: constant-time verify, per-IP limiter + per-account lockout, login audit — `internal/auth/password`
- [x] AC-PW-05 Startup bootstrap admin (no password user yet → admin from env, workspace admin, must change) — `internal/auth/password/bootstrap.go`
- [x] AC-PW-06 Forced-change gate (403 `password_change_required`) + `POST /api/v1/auth/password` (own change, current required) — `internal/auth/password`, `cmd/api`
- [x] AC-PW-07 Admin: `POST /api/v1/users` (create, generated password shown once), `POST /api/v1/users/{id}/password-reset`; disable/enable via existing PATCH; sessions revoked on reset/disable; audited — `internal/rbac`, `internal/rbac/http`
- [x] AC-PW-08 Wiring: login flow/registry, contributor `must_change_password`, `/auth/config` password fields, PublicEndpoints, contract test modules — `cmd/api`
- [x] AC-PW-14 Unit tests (hasher, policy, flow, lockout, gate, admin service, no registration route)
- [x] AC-PW-15 Integration tests on testkit Postgres/Redis (bootstrap, forced change, fail/lockout, admin create/reset/disable, session revocation)

## API + FE
- [x] AC-PW-09 OpenAPI: `/auth/password/login`, `/auth/password`, `/users` POST, `/users/{id}/password-reset`, config/me fields; regenerate `web/src/api/generated/schema.ts`
- [x] AC-PW-10 Login page: email/password form when `password` mode; OIDC button kept when `oidc` mode
- [x] AC-PW-11 Forced change-password screen (blocks the app until changed)
- [x] AC-PW-12 Settings → Users: create user (generated password shown once), reset password, disable/enable
- [x] AC-PW-13 User menu → "Change password"
- [x] AC-PW-16 FE checks: typecheck, lint, vitest

## Verification
- [x] AC-PW-17 `go build ./... && go vet ./...`, relevant `go test` packages, FE checks — counts in the final report
