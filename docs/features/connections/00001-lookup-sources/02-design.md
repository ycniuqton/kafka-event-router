# Connections — Design (00001-lookup-sources)

Incremental. **Unchanged:** all of `00000-init/02-design.md` (Kafka connections). Lookup sources are a separate
resource in their own package that mirrors the connection patterns (service + repo + vault + audit + http module).

## 1. Packages

```
internal/lookups/                 domain + service (business entry point)
  model.go enums.go               Source, Draft, Health, ListQuery, EnvTag reuse, SourceKind (pkg/lookup)
  kinds.go                        KindSpec base {Kind, Validate, SecretFields, Target} + KindManager (http, redis, postgres, static)
  service.go                      Service: List/Get/Create/Update/Delete (usage guard), Resolve (spec + secrets)
  vault.go                        SecretVault over secrets.Manager (JSON lookup.SecretBundle)
  tester.go                       TestService: draft/saved test via lookup.Tester, health update, audit
  audit.go                        actions lookup_source.{created,updated,deleted,secret_replaced,tested}, AuditView
  errors.go ports.go
internal/lookups/pgstore/         Repository (lookup_sources) + UsageResolver (routes using `lookup.sourceId`)
internal/lookups/http/            public module /api/v1/lookup-sources, internal spec module
internal/store/migrations/3020_lookup_sources.{up,down}.sql
cmd/api/wire_lookups.go           composition (+ audit catalog actions, object labels)
```

## 2. Table `lookup_sources` (migration 3020, P4 range 3020..3029 of this folder)

| column | type | notes |
| --- | --- | --- |
| id | uuid pk default gen_random_uuid() | |
| team_id | uuid fk teams | owner team |
| name | text | unique (team_id, name) |
| description | text default '' | |
| kind | text check in (http, redis, postgres, static) | |
| env_tag | text check in (dev, staging, prod) | |
| sensitive | boolean default false | masks previews like a sensitive connection |
| config | jsonb | kind settings (never secrets) |
| limits | jsonb | concurrency / rate / breaker |
| secret_ref | text null | `internal/secrets` envelope ref |
| secret_fields | text[] default '{}' | which secrets are set |
| health_status | text default 'unknown' | unknown / healthy / unreachable / auth_failed / degraded |
| last_checked_at, last_check | timestamptz, jsonb | last test |
| row_version | bigint | If-Match (trigger `bump_row_version`, skipped for health-only writes) |
| created_at, updated_at | timestamptz | |

## 3. API

| Method + path | Permission | Notes |
| --- | --- | --- |
| `GET /api/v1/lookup-sources?kind=&env=&q=&cursor=&limit=` | `connection.read` | page of `LookupSourceDTO` |
| `GET /api/v1/lookup-sources/{id}` | `connection.read` | ETag = row_version |
| `POST /api/v1/lookup-sources` | `connection.write` (owner team) | body `LookupSourceInput` → 201 |
| `PATCH /api/v1/lookup-sources/{id}` | `connection.write` | JSON merge patch, `If-Match` required |
| `DELETE /api/v1/lookup-sources/{id}` | `connection.write` | 409 `lookup-source-in-use` with `routes[]` |
| `POST /api/v1/lookup-sources/test` | `connection.write` | draft test, body `LookupSourceInput` |
| `POST /api/v1/lookup-sources/{id}/test` | `connection.write` | saved test, updates health |
| `GET /internal/v1/lookup-sources/{id}/spec` | workload | `lookup.SourceSpec` incl. secrets |

```jsonc
// LookupSourceDTO
{ "id": "uuid", "name": "crm-api", "description": "", "kind": "http", "envTag": "prod", "sensitive": false,
  "owner": { "teamId": "uuid", "teamName": "Payments" },
  "http":     { "baseUrl": "https://crm.internal/api", "auth": { "mode": "bearer", "username": "", "headerName": "" },
                "tls": { "insecureSkipVerify": false, "caPem": "" }, "headers": { "Accept": "application/json" } },
  "redis":    null,   // { "addr": "redis:6379", "db": 0, "username": "", "tls": false }
  "postgres": null,   // { "host": "", "port": 5432, "database": "", "user": "", "sslMode": "require", "statementTimeoutMs": 2000 }
  "static":   null,   // { "tableRef": "price-list", "version": 3 }
  "limits": { "maxConcurrency": 16, "ratePerSec": 0, "burst": 0, "breakerFailures": 5, "breakerOpenMs": 10000 },
  "secretFields": ["token"],                                    // token | password | headerValue
  "health": { "status": "healthy", "checkedAt": "…", "lastFailure": null },  // {step, message, hint}
  "usedBy": { "count": 1, "routes": [{ "id": "uuid", "name": "orders-enrich" }] },
  "version": 3, "createdAt": "…", "updatedAt": "…" }
// LookupSourceInput = DTO members name, description, kind, envTag, sensitive, owner{teamId}, http|redis|postgres|static,
//                     limits, plus write-only "secrets": { "token"?, "password"?, "headerValue"? }
// TestResultDTO
{ "ok": false, "health": "auth_failed", "checkedAt": "…",
  "steps": [ { "step": "reachable", "state": "passed", "durationMs": 12, "message": "", "hint": "" },
             { "step": "authenticated", "state": "failed", "durationMs": 30, "message": "401 Unauthorized",
               "hint": "Check the bearer token." }, { "step": "sample", "state": "skipped" } ] }
```

Validation errors use RFC 7807 `errors[]` with JSON Pointers (`/http/baseUrl`, `/secrets/token`, `/limits/maxConcurrency`).

## 4. Flows

```
[HTTP] POST /api/v1/lookup-sources
└─ CreateHandler → OwnerTeamPolicy.Resolve → TeamGuard(connection.write, team)
   └─ Service.Create(draft)
      └─ KindManager.Get(kind).Validate(draft) → errors → 422 with pointers
      └─ tx: SecretVault.Store(secrets only of the kind/auth mode) → Repository.Create
         → audit lookup_source.created (AuditView: no secret values)
---
[HTTP] POST /api/v1/lookup-sources/{id}/test
└─ TestService.Saved(id)
   └─ Service.Resolve(id) → spec + decrypted secrets
   └─ lookup.Tester.Run(spec) → reachable → authenticated → sample (each ≤ 10 s; first failure skips the rest)
   └─ Repository.UpdateHealth (no row_version bump) → audit lookup_source.tested
---
[worker/preview] SourceLookup step first call for source S
└─ lookup.Gateway.client(S) → RemoteSpecProvider GET /internal/v1/lookup-sources/S/spec [async boundary: HTTP]
   └─ FactoryRegistry.New(spec) → GuardedClient (breaker, limiter) cached for SpecTTL (60 s), then refreshed
```

## 5. Web (Connections screen + Studio)
- `api/contracts.ts`: `LookupSourcesService` (`list, get, create, update, remove, test(id), testDraft(input)`),
  mock + http implementations; domain `lookupSource.ts` (kinds, auth modes, defaults, labels, validation rules).
- Connections page: tabs "Kafka clusters" / "Lookup sources"; `LookupSourcesTable`, `LookupSourceFormDrawer`
  (Basics → Kind settings → Limits; secrets via the existing `SecretInput`), `LookupSourceTestPanel`.
- Studio lookup form: third mode "External source" → source picker, request template inputs per request kind,
  response mapping rows (from → to), on-miss / on-error, timeout / retries, cache, mock response (JSON text).
  Builder rail/step summary shows the source name + kind. Preview step logs with `lookup: mode=live` show a
  "live lookup" badge, `mode=mock` a "mock" badge.

## 6. Decisions
- Separate `lookup_sources` table/package instead of new `connections.kind` values: Kafka connections carry
  brokers/security/topics/probe semantics that do not apply; shared behaviour (env tag, sensitive, team, secrets,
  audit, RBAC) is reused through the same platform pieces.
- RBAC reuses `connection.read` / `connection.write` (no new catalog permissions, no OPA policy change).
- Static sources reference a versioned library lookup table (`tableRef` + `version`); upload/versioning stays in
  the library (JSON/CSV importers exist).
- Test is synchronous (≤ 3 steps), not streamed like the Kafka checklist.
- Owner team only (no grants yet): routes validation rule `lookup_source` (CN-LS-18) reports unknown sources
  (`connection_not_found`), other teams' sources (`connection_forbidden`) and request/source kind mismatches
  (`schema_invalid`) at `/…/lookup/sourceId`; the runtime gateway enforces the same team rule.

```
[HTTP] PUT draft / POST deploy of a route
└─ ValidationManager.Run → SourceRefValidator (contributed via assembly.Ports.Validators)
   └─ SourceRefs.Of(config) (nested if/else included) → Repository.Get(id) per reference
      ├─ missing / not a uuid      → error connection_not_found
      ├─ owner team ≠ route team   → error connection_forbidden
      ├─ request kind ≠ source kind → error schema_invalid
      └─ store failure             → check_skipped warning (manager convention)
```
