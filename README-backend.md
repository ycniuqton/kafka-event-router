# Kafka Event Router — Backend

Go monorepo (module `github.com/eventrouter/router`) for the control plane and data plane.
The frontend lives in `web/` and is documented separately.

## Layout

| Path | Responsibility |
| --- | --- |
| `cmd/api` | Control-plane REST/WS API under `/api/v1` (RFC 7807 errors) |
| `cmd/worker` | Data-plane worker: route runners, offsets, DLQ, reconciliation; `/metrics` |
| `cmd/preview` | Sample sessions and traced preview runs; `/metrics` |
| `pkg/config` | Route config document types and validators |
| `pkg/engine` | Kafka-free pipeline engine shared by worker and preview |
| `pkg/sandbox` | QuickJS/WASM (wazero) JavaScript sandbox |
| `pkg/kafka` | franz-go client factory, admin ops, sampling helpers |
| `internal/store` | pgx pool, `TxManager`, cursor/keyset paging, versioned updates, `Migrator`, `Notifier`; SQL migrations in `internal/store/migrations`; `identity/` repos for auth |
| `internal/secrets` | Envelope encryption (`EnvelopeManager`), `local` key-file and `vault` Transit providers, internal `/internal/v1/secrets/{ref}` |
| `internal/auth` | Principal, session cookies (Redis), CSRF, OIDC + dev login, workload auth, `Authorizer` port |
| `internal/audit` | Audit write side: `Recorder`, `Scope`, merge-patch `DiffBuilder`, `Middleware` |
| `internal/platform` | Bootstrap (`app.Runner`, `Deps`, internal listener), env config, slog JSON + redaction, HTTP server, RFC 7807 `problem` catalog, `api` conventions (handlers, decoder, paging, If-Match), metrics, `controlplane` client |
| `deploy/` | Docker Compose (infra + profile `app` + profile `oidc` with Dex), Dockerfiles, nginx; Helm chart `deploy/helm/event-router` |

Every service exposes `/healthz` (liveness) and `/readyz` (readiness) and shuts down
gracefully on SIGINT/SIGTERM.

## Running locally

```sh
make compose-up              # start Postgres, Redis, Kafka, Prometheus
cp deploy/.env.example .env  # optional; defaults work for local dev
make run-api                 # :8080
make run-worker              # :8081
make run-preview             # :8082
```

Migrations are embedded in the api binary: `go run ./cmd/api migrate up|down N|status`
(or `make migrate-up`). `ROUTER_MIGRATE_ON_START=true` applies them on api start (local only;
advisory lock 7001 serialises replicas).

Full stack in containers: `make up` (infra + migrate job + api/worker/preview/web, web on :3000),
`make down`. OIDC testing: `docker compose -f deploy/docker-compose.yml --profile app --profile oidc up -d`
and add `127.0.0.1 dex` to `/etc/hosts`; Dex user `admin@example.com` / `password`.
Without OIDC, `POST /auth/dev/login` (local only) seeds the default team and an admin.

## Configuration

All settings are `ROUTER_*` env variables; see `deploy/.env.example`. Outside `ROUTER_ENV=local`,
the `dev` auth mode and the `local` secrets provider are rejected at startup, and `ROUTER_CURSOR_SECRET` /
`ROUTER_COOKIE_SECRET` are required. `static_token` workload auth and `ROUTER_COOKIE_SECURE=false`
(plain-HTTP cookies; default is secure outside local) are allowed in `local` and `staging` only.
Single-host staging with Docker Compose: `deploy/staging/README.md`.

The preview service samples Kafka (`ROUTER_PREVIEW_KAFKA_SAMPLER=franz`, default) with connection specs it
reads from the API internal listener (`ROUTER_CONTROL_PLANE_INTERNAL_URL` + `ROUTER_WORKLOAD_AUTH` credential);
`ROUTER_PREVIEW_SALTS` picks `process` (local default) or `control_plane` team hash salts. JS sandbox caps
come from `ROUTER_SANDBOX_*` (engine, max timeout/memory/output, reset policy, pool idle, cache dir).

## Security & operations (P2)

- **API tokens:** Settings → API tokens, or `POST /api/v1/auth/tokens {name, scopes:[read|write|admin], expires_in_days}`
  (secret `ert_…` shown once); send `Authorization: Bearer ert_…`. Tokens act as their owner, narrowed by scope.
- **Rate limiting:** per user / token / IP in Redis (`ROUTER_RATE_LIMITER`, `ROUTER_RATE_LIMIT_PER_MINUTE`, `_BURST`) → 429 + `Retry-After`.
- **TLS:** `ROUTER_TLS_CERT_FILE` / `_KEY_FILE` on every listener; with `ROUTER_TLS_CLIENT_CA_FILE` the internal listener
  requires client certificates (`ROUTER_WORKLOAD_AUTH=mtls`; workers set `ROUTER_CONTROL_PLANE_CLIENT_CERT_FILE` / `_KEY_FILE`).
- **Secrets providers:** `local`, `vault`, `aws_kms` (`ROUTER_AWS_KMS_KEY_ID`, `_REGION`), `gcp_kms` (`ROUTER_GCP_KMS_KEY_NAME`).
- **Tracing:** `ROUTER_TRACING_EXPORTER=otlp` + `ROUTER_TRACING_OTLP_ENDPOINT` (or `stdout`); logs carry `trace_id`.
- **WebSockets:** features mount `api.WSHandler`s through `Wiring.WebSockets` (origin check `ROUTER_WS_ALLOWED_ORIGINS`, pings).
- **Contract tests:** `internal/platform/api/contract` validates API test traffic against `api/openapi/openapi.yaml`.

## Security & operations (P3)

- **Master-key rotation:** point `ROUTER_SECRETS_LOCAL_KEY_FILE` (or the Vault transit key / KMS key) at the new key,
  keep old local keys in `ROUTER_SECRETS_LOCAL_PREVIOUS_KEY_FILES` (decrypt only), then `api secrets rotate`
  (or wait for the leader job, `ROUTER_SECRETS_ROTATION_INTERVAL`, default 24h, `0s` off). `api secrets status`
  lists envelopes per key id; once only the new key is listed, old keys can be removed.
- **OIDC group mapping:** `ROUTER_OIDC_ROLE_CLAIM` (dot path, default `groups`) and
  `ROUTER_OIDC_ROLE_MAPPINGS=group=Team:role,…` create/raise role bindings on login (never lower or remove; existing teams only).
- **Audit stream:** `ROUTER_AUDIT_STREAM_SINKS=log` emits every committed audit entry as an `audit.event` log line
  (buffer `ROUTER_AUDIT_STREAM_BUFFER`); features add sinks via `Wiring.PlatformP3.AuditStream.Add`.
- **SIEM audit export (access-control):** `ROUTER_AUDIT_EXPORT_SINK=webhook|syslog` + `ROUTER_AUDIT_EXPORT_TARGET`
  (`https://…` NDJSON POST, or `udp://host:514` / `tcp://host:601` RFC 5424) ships every audit entry at least once from the
  scheduler leader, checkpointed in `audit_export_checkpoints` (`_INTERVAL` 10s, `_BATCH` 500, backoff ≤ 5 min);
  status in Settings → Workspace / `GET /api/v1/audit-log/export/status`. File export: `GET /api/v1/audit-log/export?format=csv|jsonl`.
- **Policy engine:** `ROUTER_RBAC_ENGINE=opa` evaluates the embedded Rego policy (`internal/rbac/opa/policy.rego`) over the
  role matrix as OPA data; `ROUTER_RBAC_OPA_POLICY=<file.rego>` replaces the policy (same package and rules).
- **Workspace settings:** `GET/PATCH /api/v1/workspace-settings` (`aiAssistEnabled`, default off; `defaultRequireProdApproval`).

## Make targets

`build` (binaries into `bin/`), `test`, `lint` (`go vet`), `tidy`, `run-api`, `run-worker`, `run-preview`,
`compose-up`/`compose-down` (infra only), `up`/`down` (full stack), `migrate-up`, `migrate-down`,
`migrate-status`, `docker-build`, `openapi-lint`, `openapi-gen`, `test-sandbox` (JS sandbox release gate),
`smoke` (`deploy/docker/smoke.sh`: waits for `/readyz` 200 after `make up`), `test-integration`, `helm-lint`, `helm-template` (kubeconform).

## Integration tests (testkit)

`internal/testkit` provides backing services to tests through a provider chosen by `ROUTER_TEST_PROVIDER`:
`external` (default; `ROUTER_TEST_DATABASE_URL`, `ROUTER_TEST_REDIS_URL`, `ROUTER_TEST_KAFKA_BOOTSTRAP`,
`ROUTER_TEST_VAULT_ADDR`/`_TOKEN`) or `container` (throwaway containers via the docker CLI). Tests skip when
the service is not configured. `Manager.Postgres(t)` gives each test its own schema (search_path pinned to it,
dropped at cleanup), so it is safe against the shared dev database:
`ROUTER_TEST_DATABASE_URL=$ROUTER_DATABASE_URL go test ./internal/store/ ./internal/audit/` (after
`source deploy/dev/env.sh`). CI: `.github/workflows/integration.yml`, `.github/workflows/compose-smoke.yml`.
