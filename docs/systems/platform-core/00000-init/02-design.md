# platform-core — Design (00000-init)

Scope: architecture (SysDesign §2), data model + migrations (§4), API conventions (§6), authentication,
secrets + transport (§11), audit-write middleware, `deploy/`, generated API client for `web/`.
Everything here is a building block; feature folders own their endpoints and business rules.

## 1. Architecture

```
            browser (web/, React)                       automation (API tokens, P2)
                   │ HTTPS, session cookie + CSRF               │ Bearer
                   ▼                                            ▼
        ┌────────────────────── cmd/api  (≥2, stateless) ──────────────────────┐
        │ public listener :8080  /api/v1/*  /auth/*  WS                       │
        │ internal listener :8443 /internal/v1/*  (workload identity, mTLS P2) │
        └───────┬───────────────┬──────────────────┬───────────────────────────┘
                │ pgx            │ go-redis          │ secrets.Provider
                ▼                ▼                   ▼
           PostgreSQL         Redis           Vault Transit / KMS / local key
           (§4 model,       (sessions,
           LISTEN/NOTIFY)    rate limits,
                             samples, tail)
                ▲                ▲
        cmd/worker, cmd/preview ─┘── fetch config/leases/secrets via api internal listener;
                                     event traffic goes worker/preview ↔ user Kafka only
```

Process layout (unchanged from skeleton): every binary is `app.NewRunner(service).Run()`.
platform-core extends `app.Deps` and adds a second, optional internal listener.

| Package | Responsibility | Key types |
| --- | --- | --- |
| `internal/platform/app` | bootstrap, dependency wiring, listeners | `Runner`, `Service`, `Deps`, `InternalService` (opt. second mount) |
| `internal/platform/config` | env config incl. new Auth/TLS/Secrets/Internal sections | `Config`, `EnvKey` enum, `AuthConfig`, `TLSConfig` |
| `internal/platform/httpserver` | servers, health, TLS | `Server`, `Health`, `TLSFactory` |
| `internal/platform/problem` | RFC 7807 documents and catalog | `Details`, `TypeURI`, `FieldError`, `Writer`, `Catalog` |
| `internal/platform/api` (new) | conventions shared by all handlers | `Decoder`, `Responder`, `PageRequest`, `PageResponse[T]`, `ETag`, `Precondition`, `RequestIDMiddleware`, `MetricsMiddleware`, `Handler` base |
| `internal/platform/logging` | slog JSON + redaction | `Factory`, `Attr` enum, `Redactor` |
| `internal/platform/metrics` | Prometheus registry + HTTP metrics | `Registry`, `HTTPCollector` |
| `internal/platform/ratelimit` (P2) | Redis token bucket | `Limiter` interface, `RedisLimiter`, `Middleware` |
| `internal/platform/tracing` (P2) | OTel setup | `Provider` |
| `internal/store` | pgx pool, tx, repos base, cursor, migrations, notify | `DB`, `TxManager`, `Repository[T,ID]`, `Cursor`, `KeysetQuery`, `Migrator`, `Notifier`, `Channel` enum |
| `internal/auth` (new) | authn: sessions, OIDC, dev login, tokens, workload | `Principal`, `Authenticator`, `AuthManager`, `SessionStore`, `Authorizer` (port), handlers |
| `internal/audit` write side | recorder + middleware (read side is access-control) | `Entry`, `Action` enum, `ObjectType` enum, `Recorder`, `Scope`, `Middleware` |
| `internal/secrets` | envelope encryption | `Provider`, `Manager`, `Registry`, `EnvelopeManager`, `Envelope`, `SecretRepository` |
| `deploy/` | compose, Dockerfiles, Helm | — |
| `api/openapi/` | OpenAPI 3.1 root + shared components | `openapi.yaml`, `components/*.yaml`, `paths/*.yaml` (features) |
| `web/src/api/http`, `web/src/api/generated` | real REST client | `HttpClient`, `ApiError`, `CursorIterator`, `HttpServicesFactory`, `SessionService` |

Note: `internal/audit` is shared with access-control. platform-core owns `recorder.go`, `scope.go`,
`middleware.go`, `entry.go`; access-control adds `query.go`, `export.go` and the `/audit-log` handlers.

## 2. Configuration additions (`internal/platform/config`)

New `EnvKey` constants (all with defaults in `ConnectionDefaults`/new domain enums):

| Key | Section | Example / default |
| --- | --- | --- |
| `ROUTER_INTERNAL_ADDR` | Internal | `:8443` (api only; empty = disabled) |
| `ROUTER_TLS_CERT_FILE`, `ROUTER_TLS_KEY_FILE`, `ROUTER_TLS_CLIENT_CA_FILE` | TLS | empty = plain HTTP (local only) |
| `ROUTER_AUTH_MODES` | Auth | `dev,oidc` (comma list of `AuthMode` enum: `oidc`, `dev`, `api_token`) |
| `ROUTER_OIDC_ISSUER`, `ROUTER_OIDC_CLIENT_ID`, `ROUTER_OIDC_CLIENT_SECRET`, `ROUTER_OIDC_REDIRECT_URL`, `ROUTER_OIDC_SCOPES` | Auth | `openid,email,profile` |
| `ROUTER_SESSION_IDLE_TTL`, `ROUTER_SESSION_ABSOLUTE_TTL` | Auth | `8h`, `7d` |
| `ROUTER_DEV_ADMIN_EMAIL`, `ROUTER_DEV_TEAM_NAME` | Auth | `admin@local.dev`, `default` |
| `ROUTER_WORKLOAD_AUTH` | Internal | `static_token` / `k8s_token` / `mtls` |
| `ROUTER_WORKLOAD_TOKEN` | Internal | dev shared token (local only) |
| `ROUTER_SECRETS_LOCAL_KEY_FILE` | Secrets | `./.local/master.key` |
| `ROUTER_VAULT_ADDR`, `ROUTER_VAULT_TRANSIT_KEY`, `ROUTER_VAULT_AUTH` | Secrets | `transit/keys/event-router` |
| `ROUTER_MIGRATE_ON_START` | Database | `true` only for local |
| `ROUTER_PUBLIC_URL` | HTTP | used for OIDC redirect + cookie domain |

Loader validation: `dev` auth mode or `local` secrets provider or `static_token` workload auth with
`ROUTER_ENV != local` → startup error `config: <mode> is only allowed in local environment`.

## 3. Data model touchpoints and migrations

Existing `0001_init` stays untouched. New migrations (each with `.down.sql`):

| File | Change | Why |
| --- | --- | --- |
| `0002_row_version.up.sql` | `ALTER TABLE {teams,users,connections,routes,transforms,lookup_tables,alert_rules} ADD COLUMN row_version bigint NOT NULL DEFAULT 1`; trigger `bump_row_version()` BEFORE UPDATE sets `row_version = OLD.row_version + 1, updated_at = now()` | If-Match/ETag (FR-11) |
| `0003_secrets_audit.up.sql` | `CREATE TABLE secrets (ref text PK, provider text NOT NULL, key_id text NOT NULL, wrapped_key bytea NOT NULL, nonce bytea NOT NULL, ciphertext bytea NOT NULL, created_at, rotated_at)`; `connections.secret_ref` FK → `secrets(ref)` ON DELETE SET NULL; `audit_log ADD request_id text, team_id uuid, actor_kind text NOT NULL DEFAULT 'user'` + index `(team_id, at DESC)` | Envelope storage (FR-24), audit context (FR-31) |
| `1000_api_tokens.up.sql` (P2) | `api_tokens (id uuid PK, user_id FK, name, token_hash bytea UNIQUE, prefix text, scopes text[], expires_at, last_used_at, revoked_at, created_at)`, unique active name per user | FR-22 |
| `1001_users_last_login.up.sql` (P2) | `users ADD last_login_at timestamptz`; `users_bump_row_version` gets `WHEN (to_jsonb(OLD) - 'last_login_at' IS DISTINCT FROM …)` so logins keep If-Match valid (`users.disabled_at` is owned by access-control `0005`) | login tracking |

Fixed MVP migration order (decision; each folder owns its file, platform-core owns `Migrator` and the
convention test that every `up` has a `down`):

| # | File | Owner |
| --- | --- | --- |
| 0002 | `0002_row_version` | platform-core |
| 0003 | `0003_secrets_audit` (secrets table + audit_log request_id/team_id/actor_kind) | platform-core |
| 0004 | `0004_connections` (auth_failed, sensitive, allow_topic_create, probe_enabled, …) | connections |
| 0005 | `0005_access_control_core` (users.is_workspace_admin/disabled_at, audit_log.sensitive/object_label) | access-control |
| 0006 | `0006_team_hash_salt` | js-sandbox |
| 0007 | `0007_routes_lifecycle` | routes |
| 0008 | `0008_route_runtime` (incl. `router_runtime_notify` trigger) | route-runtime |

0009 is unassigned. P2/P3 migrations start at 0010 and take the next free number at implementation
time. Exactly one folder owns each column: `connections.*` columns belong to connections,
`users.disabled_at` to access-control, `teams.hash_salt_ref` to js-sandbox.

`Channel` enum for LISTEN/NOTIFY (payload = JSON `{id, row_version}`):
`route_changed`, `connection_changed`, `deployment_changed`, `alert_rule_changed`. Publishers call
`Notifier.Notify(ctx, Channel, payload)` inside the tx (delivered on commit). Consumers: route-runtime.

## 4. API conventions (`internal/platform/api`)

### 4.1 Handler base
Class-based handlers implement `api.Handler`:

```go
type Handler interface {
    Route() RouteSpec               // method, pattern, AuditAction, Permission, Mutating bool
    Serve(w http.ResponseWriter, r *http.Request) error   // returned error → problem via Catalog
}
type Module interface { Handlers() []Handler }            // one per feature (connections, routes, …)
type Mounter struct { /* wires middleware chain per RouteSpec */ }
```

`Mounter.Mount(router, modules...)` attaches the chain:
`RequestID → Recover → Metrics → Logging → RateLimit(P2) → Authenticate → CSRF → Authorize(Permission) → Audit(if Mutating) → handler`.
Feature code never writes HTTP status codes directly for errors; it returns typed domain errors.

### 4.2 Errors → RFC 7807
`problem.Catalog` maps domain error kinds to `(TypeURI, status)`; it is the only place that maps.

| Error kind (enum `problem.Kind`) | Status | TypeURI suffix |
| --- | --- | --- |
| `Validation` | 422 | `/validation` (with `errors[]`) |
| `MalformedBody` | 400 | `/malformed-body` (with `path`) |
| `Unauthenticated` | 401 | `/unauthenticated` |
| `Forbidden` | 403 | `/forbidden` |
| `NotFound` | 404 | `/not-found` |
| `Conflict` (unique, state) | 409 | `/conflict` |
| `PreconditionFailed` (stale If-Match) | 412 | `/precondition-failed` (+ `current_etag`) |
| `PayloadTooLarge` | 413 | `/payload-too-large` |
| `PreconditionRequired` | 428 | `/precondition-required` |
| `RateLimited` (P2) | 429 | `/rate-limited` |
| `Unavailable` (dependency down) | 503 | `/unavailable` |
| anything else | 500 | `/internal` (detail generic, request_id only) |

```json
{ "type": "https://eventrouter.dev/problems/validation", "title": "Unprocessable Entity", "status": 422,
  "detail": "2 fields need attention.", "instance": "/api/v1/connections", "request_id": "01J…",
  "path": "/bootstrapServers/0",
  "errors": [ { "path": "/bootstrapServers/0", "code": "host_port", "message": "Use host:port, e.g. kafka-1:9092." },
              { "path": "/name", "code": "duplicate", "message": "A connection named orders-prod already exists in this team." } ] }
```
`path` = first error's path, always a JSON Pointer (RFC 6901, decision shared with pipeline-engine).
`domain.ValidationError` carries `[]FieldError`.

### 4.3 Request decoding
`api.Decoder[T]`: `http.MaxBytesReader` (default 1 MiB, `BodyLimit` enum per RouteSpec), `DisallowUnknownFields`,
JSON syntax/type errors mapped to `MalformedBody` with a JSON Pointer path (e.g. `/config/retries`).

### 4.4 Cursor pagination
Request `GET /x?limit=50&cursor=<opaque>`; response:
```json
{ "items": [ … ], "next_cursor": "eyJrIjpb…" }     // next_cursor null on last page
```
`store.Cursor{SortKey []any, ID string}` encoded as base64url(JSON) + HMAC-SHA256 (key from config) to
reject tampering → 400 `/malformed-body` path `/cursor`. `KeysetQuery` builds
`WHERE (sort_col, id) < ($1, $2) ORDER BY sort_col DESC, id DESC LIMIT limit+1`. `PageLimit` enum:
default 50, max 200; limit > max is clamped, ≤ 0 → 422.

### 4.5 Optimistic concurrency
GET single resource sets `ETag: "<row_version>"` (strong). PATCH/PUT/DELETE on versioned resources
(`RouteSpec.Versioned=true`) require `If-Match`. Repository `Update(ctx, entity, expectedVersion)` runs
`UPDATE … WHERE id=$1 AND row_version=$2 RETURNING *`; zero rows + row exists → `PreconditionFailed`.

### 4.6 Other headers
`X-Request-ID` (accepted if valid ULID, else generated), `X-CSRF-Token`, `Retry-After` (429/503),
`Cache-Control: no-store` on all `/api/v1` and `/auth` responses.

### Flows

```
HTTP GET /api/v1/connections?limit=50&cursor=abc
└─ RequestID → Metrics → Logging → Authenticate(cookie session)
   └─ no/expired session → 401 /unauthenticated
   └─ principal ok → Authorize(PermConnectionsRead)
      └─ denied → 403 /forbidden
      └─ allowed → handler: api.PageRequest.Parse(query)
         └─ cursor HMAC invalid → 400 /malformed-body path=/cursor
         └─ valid → repo.List(KeysetQuery limit+1)
            └─ got limit+1 rows → 200 {items[:limit], next_cursor=encode(last)}
            └─ ≤ limit rows → 200 {items, next_cursor:null}
```
---
```
HTTP PATCH /api/v1/routes/{id}  (If-Match: "7")
└─ middleware chain (… Authenticate → CSRF → Authorize(PermRoutesEdit))
   └─ CSRF header ≠ cookie token → 403 /forbidden detail "CSRF token missing or invalid"
   └─ ok → Audit middleware opens audit.Scope in ctx
      └─ handler: Precondition.Require(If-Match)
         └─ header missing → 428 /precondition-required
         └─ present → Decoder[RoutePatch].Decode
            └─ bad JSON → 400 /malformed-body path=<json path>
            └─ ok → TxManager.WithinTx
               └─ repo.Update(entity, expected=7) → 0 rows → 412 /precondition-failed current_etag="8"
               └─ updated row_version=8 → scope.Record(update, route, id, diff) → audit row inserted in same tx
                  └─ Notifier.Notify(route_changed) → delivered on commit [event]
                  └─ commit → 200 body, ETag "8"
```

## 5. Authentication (`internal/auth`)

### 5.1 Types
```go
type PrincipalKind string // enum: user, api_token, workload, anonymous
type Principal struct {
    Kind PrincipalKind; UserID uuid.UUID; Email, DisplayName string
    Teams []TeamRole  // {TeamID, Role team_role}
    SessionID string; TokenID uuid.UUID; WorkloadName string
}
type Authenticator interface {                // base for every request authenticator
    Mode() AuthMode
    Authenticate(r *http.Request) (Principal, Outcome, error) // Outcome enum: matched, not_applicable, rejected
}
type AuthManager struct { chain []Authenticator }   // built from ROUTER_AUTH_MODES via Registry
type LoginFlow interface {                    // base for interactive logins
    Mode() AuthMode
    Begin(w http.ResponseWriter, r *http.Request) error
    Complete(w http.ResponseWriter, r *http.Request) (Identity, error)
}
type LoginManager struct { flows map[AuthMode]LoginFlow; users UserProvisioner; sessions SessionStore }
type SessionStore interface { Create(ctx, Session) (id string, err error); Get; Touch; Revoke; RevokeAllForUser }
type UserProvisioner interface { Upsert(ctx, Identity) (User, error) } // users repo; bootstrap of role bindings
type Authorizer interface {                   // PORT — implemented by access-control (internal/rbac)
    Authorize(ctx context.Context, p Principal, perm Permission, obj ObjectRef) error
}
type WorkloadAuthenticator interface { Kind() WorkloadAuthKind; Authenticate(r *http.Request) (Principal, error) }
```
Implementations (registered by name in `auth.Registry`, chosen by config):
`SessionCookieAuthenticator` (mode oidc|dev), `APITokenAuthenticator` (P2), `OIDCLoginFlow`, `DevLoginFlow`;
workload: `StaticTokenWorkloadAuthenticator`, `K8sTokenReviewAuthenticator`, `MTLSWorkloadAuthenticator` (P2).
MVP default `Authorizer` until access-control lands: `TeamRoleAuthorizer` stub allowing admin/editor on
their team (single team) — lives in `internal/auth/authorizer_bootstrap.go` and is replaced by config
`ROUTER_AUTHZ=rbac`.

Session (Redis hash `sess:<id>`, id = 32 random bytes base64url): `user_id, created_at, last_seen_at,
csrf_token, ip, user_agent`. Idle TTL renewed on `Touch` (at most once/min). `user_sessions:<user_id>` set
for revoke-all. Cookies: `er_session` (HttpOnly, Secure unless local, SameSite=Lax, Path=/),
`er_csrf` (readable by JS, same attrs minus HttpOnly).

### 5.2 Endpoints (owned)

| Method & path | Phase | Request | Response |
| --- | --- | --- | --- |
| `GET /auth/config` | MVP | — | `{modes:["oidc","dev"], oidc_login_url:"/auth/oidc/login"}` (public) |
| `GET /auth/oidc/login?return_to=/routes` | MVP | — | 302 to IdP; sets `er_oidc_state` cookie (state, nonce, PKCE verifier, return_to; encrypted, 10 min) |
| `GET /auth/oidc/callback?code&state` | MVP | — | 302 to `return_to`; sets session cookies |
| `POST /auth/dev/login` | MVP (local) | `{email?: string}` | 204; sets session cookies |
| `POST /auth/logout` | MVP | — | 204; clears cookies |
| `GET /api/v1/auth/me` | MVP | — | `{id,email,display_name,initials,teams:[{id,name,role}],workspace,csrf_token}` |
| `GET /api/v1/auth/tokens` | P2 | — | `{items:[{id,name,prefix,scopes,expires_at,last_used_at}]}` |
| `POST /api/v1/auth/tokens` | P2 | `{name, scopes[], expires_in_days}` | 201 `{id, token:"ert_…"}` (shown once) |
| `DELETE /api/v1/auth/tokens/{id}` | P2 | — | 204 |

Internal (internal listener):

| `GET /internal/v1/secrets/{ref}` | MVP | — | `{ref, value_b64}`; `Cache-Control: no-store`; audited `secret.read` actor_kind=workload |

### Flows

```
User action: click "Sign in with SSO" (GET /auth/oidc/login?return_to=/routes)
└─ OIDCLoginFlow.Begin
   └─ return_to not a relative path → 400 /malformed-body path=/return_to
   └─ ok → generate state, nonce, PKCE verifier → set er_oidc_state cookie (encrypted, 10 min)
      └─ 302 → IdP authorize endpoint (discovery cached at startup, refreshed hourly [cron])
---
HTTP GET /auth/oidc/callback?code&state  (from IdP)
└─ LoginManager.Complete(OIDC)
   └─ state cookie missing/mismatch → 302 /login?error=state → audit auth.login_failed
   └─ ok → exchange code (+verifier) at token endpoint
      └─ IdP unreachable → 302 /login?error=idp_unavailable
      └─ id_token signature/nonce/aud/exp invalid → 302 /login?error=invalid_token → audit auth.login_failed
      └─ valid → UserProvisioner.Upsert(sso_subject, email, name) → users row created/updated
         └─ LoginHook.OnLogin (default impl here; access-control Bootstrapper replaces it)
            └─ first user ever in workspace → role_bindings(admin, default team) → audit user.bootstrap_admin
            └─ any later user → no binding (roles assigned manually by an admin)
         └─ SessionStore.Create → Redis sess:<id> (TTL idle) → set er_session + er_csrf cookies
            └─ audit auth.login → 302 return_to
---
HTTP POST /auth/dev/login {email?}
└─ DevLoginFlow registered? (only if ROUTER_ENV=local and "dev" in ROUTER_AUTH_MODES)
   └─ not registered → 404 /not-found
   └─ registered → DevSeeder.Ensure(team ROUTER_DEV_TEAM_NAME, user ROUTER_DEV_ADMIN_EMAIL, role admin)
      └─ rows exist → no-op
      └─ missing → insert team, user, role_binding → audit dev.seed
      └─ SessionStore.Create → set cookies → audit auth.login (actor_kind=user, detail dev) → 204
---
HTTP request with cookie er_session (any /api/v1 route)
└─ AuthManager.Authenticate: iterate configured Authenticators
   └─ SessionCookieAuthenticator: no cookie → not_applicable → next authenticator
   └─ Redis down → 503 /unavailable (fail closed)
   └─ session missing/expired → 401 /unauthenticated
   └─ ok → load roles (cached per request) → Principal in ctx → SessionStore.Touch (≤1/min)
      └─ users.disabled_at set (column from access-control 0005) → revoke session → 401
---
HTTP request with Authorization: Bearer ert_… (P2)
└─ APITokenAuthenticator: sha256(token) lookup in api_tokens
   └─ not found / revoked / expired → 401 /unauthenticated
   └─ ok → Principal{kind api_token, scopes} → CSRF skipped (no cookie) → last_used_at updated [async, batched every 60 s]
---
HTTP POST /auth/logout
└─ SessionStore.Revoke(id) → Redis key deleted → cookies cleared → audit auth.logout → 204
```

### 5.3 Error cases
- OIDC discovery fails at startup → API still starts; `/readyz` reports not ready only if `oidc` is the sole mode.
- Email claim missing → login rejected `invalid_token` (users.email NOT NULL).
- Same email arriving with a different `sso_subject` → 409 conflict page; admin must unlink (access-control).
- Clock skew on id_token → 60 s leeway.

## 6. Secrets (`internal/secrets`)

### 6.1 Types (extends existing skeleton)
```go
type Provider interface { Kind() ProviderKind; KeyID() string; WrapKey(ctx, dek []byte) ([]byte, error); UnwrapKey(ctx, wrapped []byte, keyID string) ([]byte, error) }
type Manager interface { Put(ctx, plaintext []byte) (Ref, error); Get(ctx, ref Ref) ([]byte, error); Replace(ctx, ref Ref, plaintext []byte) error; Delete(ctx, ref Ref) error }
type EnvelopeManager struct { provider Provider; repo SecretRepository; cipher DataCipher; audit audit.Recorder }   // the only Manager impl
type DataCipher interface { Seal(dek, plaintext []byte) (nonce, ct []byte, err error); Open(dek, nonce, ct []byte) ([]byte, error) } // AESGCMCipher
type SecretRepository interface { Insert(ctx, Envelope) error; Get(ctx, Ref) (Envelope, error); Update(ctx, Envelope) error; Delete(ctx, Ref) error; ListByKeyID(ctx, keyID string, page store.Page) ([]Envelope, string, error) }
type Rotator struct { /* P3: re-wraps envelopes whose key_id ≠ provider.KeyID() */ }
```
Providers: `LocalProvider` (AES-256-GCM master key from file, `local` env only), `VaultTransitProvider`
(`/transit/encrypt|decrypt/<key>`, production default in MVP), `AWSKMSProvider` and `GCPKMSProvider` (P2 adapters behind the same `Provider` interface).
`Ref` format: `sec_<ulid>`. Plaintext buffers are zeroed after use. The `connections` feature decides
what goes inside the plaintext (a JSON blob of its credential fields).

### Flows
```
connections feature calls secrets.Manager.Put(ctx, credentialsJSON)  (inside its tx)
└─ EnvelopeManager.Put
   └─ generate 32-byte DEK (crypto/rand) → AESGCMCipher.Seal → nonce + ciphertext
   └─ Provider.WrapKey(DEK)
      └─ Vault/KMS error → return Unavailable → API 503 /unavailable "Secret store unreachable. Try again; nothing was saved." → tx rolled back
      └─ ok → SecretRepository.Insert(ref, provider, key_id, wrapped, nonce, ct) → zero DEK → return Ref
---
HTTP GET /internal/v1/secrets/{ref}  (worker or preview at runner start)
└─ internal listener: TLS/mTLS handshake (P2 mTLS) → WorkloadAuthenticator (configured kind)
   └─ invalid token / unknown cert → 401 → log warn (no secret data)
   └─ ok → Principal{kind workload, name worker-…}
      └─ EnvelopeManager.Get(ref)
         └─ ref unknown → 404 /not-found
         └─ found → Provider.UnwrapKey(wrapped, key_id) → Open → plaintext
            └─ unwrap fails → 503 /unavailable
            └─ ok → audit secret.read (actor_kind=workload, object=ref) → 200 {ref, value_b64}, no-store
---
[cron] P3 master-key rotation (api leader, daily or `api secrets rotate` CLI)
└─ Rotator.Run: page through ListByKeyID(old) 
   └─ each envelope: UnwrapKey(old) → WrapKey(new) → Update(wrapped, key_id, rotated_at)
      └─ failure → stop, report count, retry next run (idempotent)
   └─ done → audit secrets.rotated {count, from_key_id, to_key_id}
```

### 6.2 Transport
- `httpserver.TLSFactory` builds `*tls.Config` (TLS 1.2+, modern suites) from `TLSConfig`; public listener
  TLS optional (usually terminated at ingress), internal listener requires client certs in non-local envs (P2).
- Workers/preview use `internal/platform/controlplane.Client` (owned here) with workload credential + CA
  pinning; route-runtime and live-preview use it — they do not build their own HTTP clients.
- Kafka TLS/SASL is owned by connections (`pkg/kafka`).

## 7. Audit write (`internal/audit`)

```go
type Action string      // enum, "<object>.<verb>": connection.create, route.deploy, auth.login, secret.read, …
type ObjectType string  // enum: connection, route, route_version, deployment, transform, lookup_table, user, team, role_binding, alert_rule, secret, session, api_token
type ActorKind string   // enum: user, api_token, workload, system
type Entry struct { Actor Principal; Action Action; Object ObjectType; ObjectID string; TeamID *uuid.UUID; Diff json.RawMessage; IP netip.Addr; RequestID string }
type Recorder interface { Record(ctx context.Context, e Entry) error }  // PostgresRecorder writes via current tx if present
type Scope struct { /* per request; Record(action, obj, id, before, after any) computes JSON merge-patch diff with secret fields stripped */ }
type Middleware struct { recorder Recorder }  // for RouteSpec.Mutating
type DiffBuilder struct { redactor logging.Redactor }   // drops fields tagged `audit:"redact"`
```

```
HTTP mutating request (RouteSpec.Mutating = true)
└─ audit.Middleware: put Scope{actor, ip, request_id} in ctx
   └─ handler → service → TxManager.WithinTx
      └─ service calls scope.Record(action, object, id, before, after)
         └─ PostgresRecorder inserts audit_log row using the same tx → commit together
   └─ response 2xx and scope recorded ≥1 entry → done
   └─ response 2xx and scope empty → Recorder.Record(fallback entry action=<RouteSpec.AuditAction>, no diff) + log warn audit.missing_scope
   └─ response 4xx/5xx → nothing recorded (tx rolled back) except auth failures recorded explicitly by auth
```
In tests `audit.StrictMode=true` turns the fallback into a panic → handler test fails (FR-31).

## 8. Bootstrap wiring

```
Process start: cmd/api main → app.Runner.Run
└─ config.Loader.Load → validate env-restricted modes
   └─ invalid → exit 1 with config error
   └─ ok → logging.Factory → store.Open(pgx pool) + redis.Open
      └─ ROUTER_MIGRATE_ON_START=true → Migrator.Up (advisory lock 7001 so only one api migrates)
         └─ migration fails → exit 1
      └─ secrets.Registry.Build(provider) → auth.Registry.Build(modes) → audit.PostgresRecorder
      └─ api.Mounter.Mount(public router, feature modules…) ; Mount(internal router, internal modules…)
      └─ start public + internal listeners → Health.MarkReady when Postgres+Redis ping ok [async, every 5 s]
---
CLI: api migrate up|down N|status
└─ Migrator (golang-migrate with iofs source over store.Migrations) → advisory lock → apply → print versions
```

## 9. Deploy (`deploy/`)

| File | Content |
| --- | --- |
| `deploy/docker/Dockerfile.go` | multi-stage: `golang:1.27` build `-trimpath` all cmds → `gcr.io/distroless/static:nonroot`; `ARG CMD` selects entrypoint |
| `deploy/docker/Dockerfile.web` | node build of `web/` → `nginx:alpine` with SPA fallback + `/api` proxy |
| `deploy/docker/nginx.conf` | SPA routing, proxy `/api/`, `/auth/` → api:8080, WS upgrade |
| `deploy/docker-compose.yml` | existing infra + `migrate` (one-shot), `api`, `worker`, `preview`, `web`; profiles `app` and `oidc` (Dex with static user); `.local/master.key` volume |
| `deploy/dex/config.yaml` | Dex static client + user for OIDC testing |
| `deploy/helm/event-router/` | Chart: `values.yaml` (image, replicas: api 2, worker 2, preview 1; auth; secrets provider; tls), templates for Deployments, Services, Ingress, migrate Job (pre-install/upgrade hook), ServiceAccounts (worker/preview projected tokens with audience `event-router-internal`), PDBs, HPA, NetworkPolicies (worker egress allow-list values), ConfigMap/Secret for env |
| `Makefile` | `up`, `down`, `migrate-up`, `docker-build`, `helm-lint`, `openapi-gen` |

## 10. Generated API client (web)

```
api/openapi/openapi.yaml          root: info, servers /api/v1, securitySchemes (cookie, bearer), $refs to paths/*
api/openapi/components/*.yaml     Problem, FieldError, Page (items/next_cursor), ETag/If-Match params, CursorParams, IdParam
api/openapi/paths/<feature>.yaml  owned by each feature folder (auth.yaml owned here)
web/src/api/generated/schema.ts   openapi-typescript output (committed; `npm run api:gen`, CI checks drift)
web/src/api/http/HttpClient.ts    fetch wrapper: baseUrl, credentials:'include', X-CSRF-Token, If-Match, JSON, ApiError from problem
web/src/api/http/ApiError.ts      class with status, type, detail, path, errors[], requestId
web/src/api/http/CursorIterator.ts async iteration over next_cursor
web/src/api/http/HttpServicesFactory.ts  builds `Services`; each feature contributes Http<Feature>Service in web/src/api/http/services/
web/src/api/http/HttpSessionService.ts   me(), authConfig(), devLogin(), logout()
web/src/pages/login/LoginPage.tsx  SSO button + "Dev login" when /auth/config lists dev
web/src/app/SessionGate.tsx        loads /auth/me; 401 → /login?return_to=
```
`ServiceRegistry.register(DataSource.Http, () => HttpServicesFactory.create())` — screens do not change.
`ApiError.fieldErrors()` gives `{path → message}` so forms highlight fields (UI §12 pattern).
Members `current()` maps to `/auth/me` (fields match `CurrentUser`).

```
User action: open app (VITE_DATA_SOURCE=http)
└─ SessionGate → HttpSessionService.me() GET /api/v1/auth/me
   └─ 200 → store current user + csrf token → render app
   └─ 401 → navigate /login?return_to=<path>
      └─ LoginPage → GET /auth/config
         └─ "dev" listed → show Dev login → POST /auth/dev/login → reload → me() 200
         └─ "oidc" listed → window.location = /auth/oidc/login?return_to=…
---
User action: save a form (any feature) via HttpClient
└─ request with X-CSRF-Token and If-Match (if entity has etag)
   └─ 2xx → parse JSON, keep new ETag on entity
   └─ 412 → ApiError(precondition-failed) → toast "Someone else changed this. Reload to see their version."
   └─ 422 → ApiError.fieldErrors() → form highlights paths
   └─ 401 → SessionGate redirect to login (draft kept by feature in SafeStorage)
   └─ network error → ApiError(kind network) → toast "Can't reach the server. Check your connection and retry."
```

## 11. Error cases summary
| Case | Result |
| --- | --- |
| Postgres down | `/readyz` 503; requests → 503 `/unavailable`; workers unaffected (SysDesign §9.5) |
| Redis down | authenticated requests 503 (sessions fail closed); `/auth/config` still works |
| Vault/KMS down | secret Put/Get → 503; connections create fails atomically; running workers keep in-memory secrets |
| Migration mismatch (DB newer than binary) | startup error `store: schema version N newer than binary M` |
| Concurrent migrators | advisory lock serializes |
| Duplicate unique key | 409 `/conflict` with `path` of the unique field (mapping from constraint name in `store.ConstraintMap`) |
| Body > limit | 413 |

## 12. Decisions
- There is one workspace per deployment. The `workspace` name comes from config and there is no multi-tenancy.
- Roles are assigned by hand. The first login becomes admin through the `LoginHook` default, which
  access-control's Bootstrapper replaces. OIDC group/claim → role mapping comes later (P3, PC-79).
- Auth is generic OIDC (auth code + PKCE) plus a dev login (local only).
- Secrets: Vault Transit is the production provider in MVP and the local key-file provider is for dev.
  AWS KMS and GCP KMS adapters come in P2.
- Problem/issue `path` is a JSON Pointer everywhere.
- MVP migration numbers are fixed (0002–0008, §3). P2/P3 migrations start at 0010.
- `users.disabled_at` belongs to access-control (0005). The platform-core P2 migration only adds `last_login_at`.
- Integration tests use `internal/testkit`: a `Provider` per kind (`external` env addresses, `container` via the
  docker CLI, no Docker SDK dependency) behind `testkit.Manager`, selected by `ROUTER_TEST_PROVIDER`; tests
  skip when a service is not configured. Postgres tests get a per-test schema sandbox:

```
go test (integration)
  └─ Manager.Postgres(t)
      └─ Provider.Provide(postgres) → endpoint DSN | not configured → t.Skip
      └─ CREATE SCHEMA testkit_<ulid> → DSN with search_path=<schema> only (public never visible)
      └─ sandbox.Migrate → production Migrator (lock 7001) applies embedded migrations into the schema
      └─ t.Cleanup → DROP SCHEMA … CASCADE; container provider also `docker rm -f`
```

## 13. P2 additions

Unchanged: sections 1–12 above. P2 migrations use the orchestrator range 1000–1009 (`1000_api_tokens`,
`1001_users_last_login`); the "≥ 0010, next free" rule above is superseded for this phase.

| Concern | Types / files | Config |
| --- | --- | --- |
| API tokens | `internal/auth/apitoken` (`Manager`, `Authenticator`, `PostgresRepository`, `UsageBatcher`), `httpapi` handlers, `auth.ScopeCatalog/ScopeGuard`, `api.ScopeStage` | mode `api_token` in `ROUTER_AUTH_MODES` (default on) |
| Rate limit | `internal/platform/ratelimit` (`Limiter`, `RedisLimiter` GCRA, `Unlimited`, `Registry`, `Guard`), `api.RateLimitStage`, `problem.Retryable` | `ROUTER_RATE_LIMITER=redis\|none`, `_PER_MINUTE` 1200, `_BURST` 300 |
| TLS / mTLS | `httpserver.TLSFactory`, `CertificateSource` (hot reload), `Server.WithTLS`, `workload.MTLSAuthenticator`, `controlplane.TransportFactory.ClientFor` | `ROUTER_TLS_*`, `ROUTER_WORKLOAD_AUTH=mtls`, `ROUTER_WORKLOAD_MTLS_ALLOWED`, `ROUTER_CONTROL_PLANE_CLIENT_CERT_FILE/_KEY_FILE` |
| KMS | `internal/secrets/kms` (`AWSProvider`, `GCPProvider`) | `ROUTER_SECRETS_PROVIDER=aws_kms\|gcp_kms`, `ROUTER_AWS_KMS_*`, `ROUTER_GCP_KMS_*` |
| Tracing | `internal/platform/tracing` (`Registry`, `Provider`, `Middleware`, `LogHandler`) | `ROUTER_TRACING_EXPORTER=none\|stdout\|otlp`, `_OTLP_ENDPOINT`, `_OTLP_INSECURE`, `_SAMPLE_RATIO` |
| WebSocket | `api.WSHandler`, `WSUpgrader` (`Wiring.WebSockets`), `WSConn`, `WSOriginPolicy` | `ROUTER_WS_ALLOWED_ORIGINS` (default public URL), `ROUTER_WS_PING_INTERVAL` 30 s |
| Contract tests | `internal/platform/api/contract` (`Spec`, `Validator`, `Recorder`) | — |
| Helm | `deploy/helm/event-router`, `.github/workflows/helm.yml` | `values.yaml` |

Public chain (design §4.1, P2 order): `RequestID → Recover → Metrics → Logging → NoStore` (global, inside
`Tracing` from `app.Runner`) then per route `Authenticate → RateLimit → CSRF → Authorize → Scope → Audit`.
RateLimit runs after Authenticate because the key is the principal.

```
HTTP POST /api/v1/auth/tokens {name, scopes, expires_in_days}   (session cookie + CSRF)
└─ Authenticate(session) → RateLimit → CSRF → Scope (users pass) → Audit scope
   └─ principal kind ≠ user → 403 "only from a signed-in browser session"
   └─ invalid name/scope/expiry → 422 errors[] (/name, /scopes/0, /expires_in_days)
   └─ ok → WithinTx: count active (≥ 50 → 422) → insert sha256(ert_…) → audit api_token.created → 201 {…, token} (shown once)
---
HTTP any /api/v1 route with Authorization: Bearer ert_…
└─ apitoken.Authenticator: hash lookup → missing / revoked / expired / owner disabled → 401
   └─ ok → Principal{kind api_token, owner's teams, scopes} → UsageBatcher.Used [async, flushed every 60 s]
      └─ RateLimit key token:<id> → over rate → 429 + Retry-After
      └─ Authorize (owner's RBAC) → ScopeGuard: route access (permission Access, else GET=read / other=write) ∉ scopes → 403
---
HTTP request on the internal listener (TLS, ROUTER_TLS_CLIENT_CA_FILE set)
└─ TLS handshake: no client cert / unknown CA → connection refused (no HTTP response)
   └─ verified chain → MTLSAuthenticator: name = URI SAN | DNS SAN | CN
      └─ not in ROUTER_WORKLOAD_MTLS_ALLOWED → 401
      └─ ok → Principal{workload, name} → handler (secrets read audited as before)
---
HTTP GET (Upgrade: websocket) on a WSHandler route
└─ normal chain (authenticate, rate limit, authorize, scopes)
   └─ WSOriginPolicy: Origin not allowed (or cookie session without Origin) → 403
   └─ upgrade → ping every interval [async] → missed pong / client close → stream ctx cancelled → close 1000
```

### Decisions (P2)
- API tokens are **user-owned** (open question resolved): a token acts as its owner and is narrowed by scopes
  `read ⊂ write ⊂ admin`; tokens cannot create or revoke tokens; default 90 days, max 365, 50 active per user.
- Rate limiting fails open when Redis errors (sessions already fail closed); staging/local keep defaults.
- The internal listener without client certificates is allowed only in `local` and `staging`.
- TLS uses one certificate per pod for both listeners; workers present the same certificate as client cert in Helm.

## 14. P3 additions

```
[cron] RotationJob (api, ROUTER_SECRETS_ROTATION_INTERVAL, first run 2 min after start) or CLI `api secrets rotate`
└─ PgLeaderElector lock 7120 (job only) → not leader → skip
└─ Rotator.Run: ListStale(provider kind, key_id ≠ Provider.KeyID(), ref > after, page 100)
   └─ each envelope: UnwrapKey(wrapped, old key_id) → WrapKey → Rewrap CAS (ref, old key_id, old wrapped)
      └─ concurrent Replace won → counted as raced, skipped
      └─ unwrap/wrap fails → stop, report failed ref; next run resumes (idempotent)
   └─ rotated > 0 → audit secrets.rotated {count, fromKeyIds, toKeyId} (actor system)
---
OIDC callback (login tx)
└─ Flow.verify reads ROUTER_OIDC_ROLE_CLAIM (dot path) → Identity.Groups
└─ ClaimRoleHook.OnLogin: access-control Bootstrapper first
   └─ mode ≠ oidc or no mappings → done
   └─ mapped grants → strongest role per team → BindingGranter
      └─ team missing → warn, skip (mappings never create teams)
      └─ no binding → BindingService.Set → audit role_binding.set
      └─ weaker binding → Set with If-Match = current row_version (raise)
      └─ equal/stronger → keep (manual assignments survive)
---
PostgresRecorder.Record (any audited write)
└─ INSERT … RETURNING id, at → store.AfterCommit
   └─ tx rolled back → nothing streamed
   └─ committed → Stream.Publish → per-sink bounded queue [async]
      └─ queue full → drop + router_audit_stream_dropped_total
      └─ Sink.Deliver fails → warn + router_audit_stream_failed_total
```

### Decisions (P3)
- Local master keys rotate by keyring: the new file is current, `ROUTER_SECRETS_LOCAL_PREVIOUS_KEY_FILES` are
  decrypt-only. Vault/KMS rotate by switching to a new key name (stored key id changes); Transit key versions inside
  one key are handled by Vault itself. Envelopes of another provider kind are not touched.
- Claim mapping is additive and upgrade-only; workspace-admin is not claim-mapped (Bootstrapper rules stay).
- The audit stream is at-most-once real-time fan-out (`audit.Sink`, file `internal/audit/stream.go` so it does not
  collide with access-control's checkpointed `ExportSink` in AC-204); the checkpointed export is the reliable path.
- Migration `2000_secrets_rotation`: index `(provider, key_id, ref)` for the stale-envelope scan.
