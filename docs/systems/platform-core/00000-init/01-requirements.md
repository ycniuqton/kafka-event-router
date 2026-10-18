# platform-core — Requirements (00000-init)

Sources: SysDesign §2, §4, §6 (conventions), §11, §14 (deploy/, generated client); UI spec §11.2, §12, §13.
Code: `cmd/api` (base), `internal/platform`, `internal/store`, `internal/secrets`, `internal/auth`, `deploy/`, `api/openapi/`, `web/src/api/http`.

## Business intent
Give every feature one safe, consistent foundation: a stateless control-plane API with uniform
conventions, a durable Postgres model with migrations, authenticated users and services, credentials
that are never stored or returned in plain text, an append-only audit trail of every mutation, a
one-command local stack, an HA-ready Kubernetes deployment, and a typed web client so the UI can
swap its mock backend for the real `/api/v1` with no screen changes.

## Functional requirements
### Architecture & bootstrap
- FR-01 [MVP] Four deployables (web, api, worker, preview) share one bootstrap (`app.Runner`): config, slog JSON logs, `/healthz`, `/readyz`, `/metrics`, graceful shutdown.
- FR-02 [MVP] `app.Deps` carries shared infra (Postgres pool, Redis, secrets Manager, auth Manager, audit recorder); readiness fails until Postgres and Redis respond.
- FR-03 [MVP] API instances are stateless (sessions in Redis), so ≥ 2 replicas can sit behind a load balancer.
### Data model & migrations (SysDesign §4)
- FR-04 [MVP] All §4 tables exist via versioned, embedded SQL migrations (`0001_init` present); `api migrate up|down|status` subcommand and optional apply-on-start for local. MVP numbers are fixed (0002–0008, see design §3); P2/P3 migrations start at 0010 and take the next free number at implementation time.
- FR-05 [MVP] Migration `0002`: `row_version bigint` on every mutable table (teams, users, connections, routes, transforms, lookup_tables, alert_rules) for If-Match.
- FR-06 [MVP] Migration `0003`: `secrets` table (envelope ciphertext); `audit_log` gains `request_id`, `team_id`, `actor_kind`.
- FR-07 [P2] Migration `00NN_api_tokens` (P2 number ≥ 0010): `api_tokens` (hashed token, owner, scopes, expiry). Sessions are never stored in Postgres (Redis only).
- FR-08 [MVP] Store base: pgx pool, `TxManager`, generic `Repository`, keyset cursor query builder, `ErrNotFound`/`ErrConflict` mapping, Postgres LISTEN/NOTIFY publisher with domain channel names.
### API conventions (SysDesign §6)
- FR-09 [MVP] All endpoints under `/api/v1`, JSON only, max body size, unknown fields rejected with the offending `path`.
- FR-10 [MVP] Cursor pagination: `?limit=&cursor=`, default 50, max 200; response `{items, next_cursor}`; opaque, tamper-checked cursors.
- FR-11 [MVP] Optimistic concurrency: GET returns `ETag: "<row_version>"`; PATCH/DELETE require `If-Match` → 428 if missing, 412 if stale.
- FR-12 [MVP] Every error is RFC 7807 `application/problem+json` with `type`, `title`, `status`, `detail`, `instance`, `path`, `request_id`, optional `errors[]` (each with `path`, `code`, `message`); every `path` is a JSON Pointer (`/bootstrapServers/0`); messages follow "what happened, why, what to do" (UI §12).
- FR-13 [MVP] Request ID on every response (`X-Request-ID`), propagated to logs and audit rows.
- FR-14 [MVP] HTTP metrics `router_api_requests_total{route,method,status}` and `router_api_request_duration_seconds`.
- FR-15 [P2] Per-principal rate limiting backed by Redis → 429 problem with `Retry-After`.
- FR-16 [P2] OpenTelemetry traces for API calls (SysDesign §12).
### Authentication
- FR-17 [MVP] Generic OIDC (authorization code + PKCE) login for the UI; user upserted by `sso_subject`; the first user to log in becomes admin; roles are then assigned manually (access-control); session cookie (HttpOnly, Secure, SameSite=Lax) backed by Redis with idle + absolute TTL. [P3] OIDC group/claim → role mapping.
- FR-18 [MVP] Dev-mode login (only when `ROUTER_ENV=local` and enabled by config): seeds a default team and an admin user, issues a normal session.
- FR-19 [MVP] CSRF protection on cookie-authenticated mutations (double-submit token header).
- FR-20 [MVP] `GET /auth/me` returns the principal, teams and roles; `POST /auth/logout` revokes the session.
- FR-21 [MVP] Authenticator implementations are selected by config (`ROUTER_AUTH_MODES`), never by `if` on type; business code sees only `auth.Principal`.
- FR-22 [P2] API tokens for automation: create (shown once), list, revoke; stored hashed; `Authorization: Bearer`.
- FR-23 [MVP] Authorization hook: platform-core defines the `Authorizer` port and enforces it per route; the policy itself is owned by access-control.
### Secrets & transport (SysDesign §11)
- FR-24 [MVP] Envelope encryption: per-secret AES-256-GCM data key wrapped by a master-key provider; Postgres stores only ciphertext + wrapped key; features use only `secrets.Manager` (Put/Get/Delete/Replace).
- FR-25 [MVP] Providers: `local` (key file, dev/local env only) and `vault` (Transit, production default). [P2] AWS KMS and GCP KMS adapters.
- FR-26 [MVP] The public API never returns secrets; workers/preview fetch them via `GET /internal/v1/secrets/{ref}` on a separate listener authenticated by workload identity; each read is audited.
- FR-27 [MVP] Workload authenticators: `static_token` (local), `k8s_token` (TokenReview). [P2] `mtls` with client-CA pinning.
- FR-28 [P2] TLS on all listeners (cert/key from config); internal listener mTLS-only in non-local envs.
- FR-29 [P3] Master-key rotation: re-wrap all data keys without re-encrypting payloads; record `key_id`.
- FR-30 [MVP] Secrets and event payloads are never logged (slog redaction of tagged fields).
### Audit write
- FR-31 [MVP] Every successful mutating call writes one `audit_log` row (actor, action, object, diff, ip, request_id) in the same transaction as the change; a request that mutates without recording fails closed in tests.
### Deployment
- FR-32 [MVP] `deploy/docker-compose.yml` runs Postgres, Redis, Kafka, Prometheus plus api/worker/preview/web and a migrate job (`make up`); optional `oidc` profile with Dex.
- FR-33 [MVP] Dockerfiles: one multi-stage Go image (distroless, non-root) for all cmds; nginx image for `web/`.
- FR-34 [P2] Helm chart: api (≥ 2 replicas, PDB), worker, preview, web, migrate Job hook, ServiceAccounts for workload identity, NetworkPolicies, HPA.
### Generated API client
- FR-35 [MVP] OpenAPI 3.1 root at `api/openapi/openapi.yaml` with shared components (Problem, Page, ETag, auth); features add path files.
- FR-36 [MVP] TS types generated into `web/src/api/generated/`; `HttpClient` (base URL, credentials, CSRF, If-Match, problem → `ApiError`, cursor iteration) and `HttpServicesFactory` registered under `DataSource.Http`.
- FR-37 [MVP] FE session bootstrap: `/auth/me` on load, 401 → login screen, dev login button when the server advertises it.
- FR-38 [P2] Contract tests: every handler's responses validated against the OpenAPI spec in CI.

## Non-functional / constraints
- Go 1.27, chi, slog, prometheus, pgx v5, go-redis v9; React 19 + TS on the FE. Devlens coding rules (base interface + Manager per extensible domain, config-selected implementations, enums, class-based handlers).
- API p95 < 150 ms for CRUD at 50 rps per instance; no in-process state that breaks horizontal scaling.
- Security: secrets encrypted at rest, TLS in transit, cookies HttpOnly/Secure, audit log append-only (DB trigger).
- Tests: unit (table-driven), integration with Testcontainers (Postgres, Redis, Vault dev), contract (OpenAPI).

## Out of scope
- RBAC policy rules, users/teams/role-bindings CRUD API, approvals, audit log read/export screen → access-control.
- Feature endpoints and their tables' repositories → owning feature folders. Leader election/scheduler → route-runtime.
- SCIM/user provisioning, multi-workspace tenancy (single workspace per deployment, decided), real AI providers.

## Dependencies
- access-control: implements `auth.Authorizer`, reads `audit_log`, manages role bindings seeded here.
- connections: first consumer of `secrets.Manager`; route-runtime/live-preview: consumers of the internal secrets endpoint.
- pipeline-engine: generated TS types for route config live beside the API client (`web/src/api/generated/config`).
- All features: use conventions, Problem types, pagination, If-Match, audit recorder, OpenAPI components from here.
