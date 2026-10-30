# platform-core — Requirements (00002-lite-profile)

Incremental version on top of `00001-hardening`. **Unchanged:** every requirement of `00000-init` and
`00001-hardening` still applies (API conventions, migrations, secrets envelope encryption, Vault / KMS /
local providers, auth modes, deploy via Helm and the staging compose). This version adds a **lite deployment
profile** that needs only Postgres + Redis. Password login for it is specified in
`docs/features/access-control/00001-password-auth`.
Code: `internal/secrets/statickey`, `internal/secrets/keyring`, `internal/platform/config/secrets_static.go`,
`cmd/api/health_cmd.go`, `deploy/lite/`.

## Business intent
Small teams and evaluations should run the router on one host without operating Vault, an OIDC provider,
Kafka for the platform itself or Prometheus — yet safely enough for production data: saved connection
credentials stay encrypted at rest, internal traffic stays authenticated, and the operator knows exactly
what to back up.

## Functional requirements
### `static_key` master-key provider
- LT-01 `ROUTER_SECRETS_PROVIDER=static_key` wraps data keys with AES-256-GCM under an operator key; allowed in
  every environment, including production. `local` (key file generated on first run) stays local-only.
- LT-02 Key from `ROUTER_SECRETS_MASTER_KEY` (base64, padded or not) or `ROUTER_SECRETS_MASTER_KEY_FILE`
  (file holding the base64); exactly one must be set (api only — worker / preview never build a provider).
  The api refuses to start on missing, both-set, invalid base64 or not-32-byte keys; errors never echo key bytes.
- LT-03 Key id = `static:` + first 16 hex chars of SHA-256(key) (same scheme as `local:`).
- LT-04 Rotation: `ROUTER_SECRETS_PREVIOUS_MASTER_KEYS` (comma base64) / `_FILES` are decrypt-only;
  `api secrets rotate` re-wraps every envelope to the current key; afterwards the previous keys can be dropped.
- LT-05 Local and static providers share one key-ring implementation; existing `local:` envelopes keep decrypting.

### `deploy/lite` compose profile
- LT-06 Services: postgres 16, redis 7, `certs-init` (one-shot), `migrate` (one-shot), api, worker, preview,
  web (nginx); Prometheus only with `--profile metrics` (`COMPOSE_PROFILES=metrics`). No Vault, Dex, Kafka.
- LT-07 Production-capable without weakening the environment guards: `ROUTER_ENV` configurable (default
  `production`); the internal listener uses **mTLS** (`ROUTER_WORKLOAD_AUTH=mtls`, certificates from
  `certs-init`), since `static_token` and an internal listener without client CAs are non-production only.
- LT-08 Two modes: production behind HTTPS (`ROUTER_COOKIE_SECURE=true`; external TLS terminator or the
  optional nginx TLS override) and an HTTP-only trial (`ROUTER_ENV=staging`, insecure cookies, not for real data).
- LT-09 Metrics off must degrade gracefully: no fake numbers, no UI errors (route list shows "—", dashboard
  "Unavailable", overview its retry state); alerts off and metrics-based attention detectors skipped.
- LT-10 Patterns from staging: resource limits, healthchecks, loopback-only host ports except web, worker
  cache volume, Postgres 1536M / `max_connections=150`.
- LT-11 `init.sh` generates `.env` (random DB/Redis/cookie/cursor/preview secrets, admin password printed once,
  master key), chmod 600, idempotent: never regenerates an existing value, only appends missing keys.
- LT-12 `README.md` documents modes, the mTLS choice, metrics switch, first login, upgrades, sizing, DR
  (Postgres dump + `.env`; losing `ROUTER_SECRETS_MASTER_KEY` makes saved credentials unrecoverable) and rotation.
- LT-13 `smoke.sh` checks a running stack (web, readiness, password login, `/auth/me`, optional forced change,
  container states, Prometheus when on); `validate.py` checks the compose statically (no docker needed).
- LT-14 `api health <url>` probes readiness from inside the distroless image (HTTPS verified against the
  internal CA) so healthchecks do not depend on busybox TLS support.

## Non-functional requirements
- NFR-LT-1 Every secret value comes from `.env` (mode 600) or files; nothing secret is committed.
- NFR-LT-2 Static validation and unit tests only in this change; the live compose run is done by the user.
