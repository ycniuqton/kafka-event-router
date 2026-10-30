# Event Router — staging (single host, Docker Compose)

Self-contained stack (compose project `event-router-staging`) for one Ubuntu host:
postgres 16, redis 7, kafka 3.9 (KRaft, single node), kafka-b (second independent cluster, demos), prometheus,
vault (server mode, file storage), vault-init (init/unseal/provision sidecar), dex (OIDC), migrate, api, worker,
preview, web (nginx), demo-producer (low-rate demo traffic).
`ROUTER_ENV=staging`.

## Deploy / redeploy

```sh
deploy/staging/deploy.sh          # from the repo root on your machine; idempotent
```

It reads `STAGING_HOST`, `STAGING_USER`, `STAGING_SSH_PORT` from `~/.secrets/event-router-staging.env`
(override with `CREDS_FILE=...`), then:

1. rsyncs the repo to `/opt/event-router/src` (no `node_modules`, `web/dist`, `.git`, `.claude`, `.devlens/html`);
2. runs `server/bootstrap.sh`: ufw (22/tcp + 80/tcp only, ssh allowed before enabling), `/opt/event-router`
   (0700), and generates missing secrets into `/opt/event-router/.env` (0600) — existing keys are never regenerated;
3. `docker compose -f deploy/staging/docker-compose.staging.yml --env-file /opt/event-router/.env up -d --build`;
4. appends the human-needed values (URL, login, Postgres, Vault unseal key/root token, workload token,
   server paths) to the local credentials file — only keys that are not there yet; nothing is printed;
5. waits until every service is healthy and `migrate` exited 0.

## Network exposure

Only `web` publishes a public port (`80 -> nginx:8080`). nginx serves the SPA and proxies `/api` (incl.
WebSocket), `/auth` and `/dex`. Postgres (`127.0.0.1:5432`), Redis (`127.0.0.1:6379`), Kafka's `DEV`
listener (`127.0.0.1:29092`, advertised as `127.0.0.1:29092`; `kafka-b`: `127.0.0.1:29093`), Prometheus (`127.0.0.1:9090`) and the api
(`127.0.0.1:8080`) bind to loopback only, for local development over an ssh tunnel (`deploy/dev/`);
everything else is reachable only on the `event-router-staging-backend` network. In-stack services keep
using `kafka:19092`. Docker bypasses ufw for published ports, so never publish anything on a non-loopback
address besides `web`.

## Login

Dex issuer `http://<host>/dex`, client `event-router`, redirect `http://<host>/auth/oidc/callback`.
Two static users from the credentials file:
- `STAGING_LOGIN_EMAIL` / `STAGING_LOGIN_PASSWORD`: also in `ROUTER_BOOTSTRAP_ADMIN_EMAILS` (workspace admin).
- `STAGING_APPROVER_EMAIL` / `STAGING_APPROVER_PASSWORD` (`staging-approver@example.com`): role `approver` in team
  `default`, the second person for strict 4-eyes production deploys (nobody approves their own request).

Dev login is refused outside `local`.

## Staging-specific settings

- `ROUTER_AUTH_MODES=oidc,api_token` (override in `/opt/event-router/.env`): OIDC for people, bearer API tokens
  (Settings → API tokens) for scripts; with `oidc` alone tokens could be created but every call with them got 401.
- `ROUTER_WORKLOAD_AUTH=static_token` on the internal listener (allowed in `local` and `staging` only).
- `ROUTER_COOKIE_SECURE=false` because staging is plain HTTP (allowed in `local` and `staging` only);
  set it to `true` in `/opt/event-router/.env` once TLS terminates in front of nginx.
- Feature settings (non-secret, appended to `/opt/event-router/.env` once by `bootstrap.sh`, compose falls back
  to the same values): `ROUTER_ASSIST_PROVIDER=mock`, `ROUTER_LIVETAIL_TAP=redis`, `ROUTER_EVENTS_BUS=redis`,
  `ROUTER_ROUTE_METRICS_READER=prometheus`, `ROUTER_ALERTS_CHANNELS=slack,teams,webhook` (email off: no SMTP host).
- `ROUTER_DATABASE_URL` (api/worker/preview; `migrate` keeps the plain DSN because golang-migrate forwards unknown
  parameters to Postgres) carries `pool_max_conns` (`ROUTER_DB_POOL_MAX_CONNS`, default 16). Historical workaround:
  the api used to park pooled connections in LISTEN, which starved the pool on a small host (readiness ping timed
  out). Since platform-core 00001-hardening all LISTENs share one dedicated session, readiness pings on its own
  session, held leader locks leave the pool, and the pool defaults to max(10, 2 x CPUs) (`ROUTER_DB_POOL_MAX_CONNS`
  read by the process when the DSN has no `pool_max_conns`), so the DSN parameter is optional. Per api process:
  pool + 2 + up to 3 lock sessions; 3 services x 16 + extras + router_dev stay under Postgres' 100.
- Postgres sizing: on 2026-10-18 a 48-connection load probe pushed the postgres container past its old 512M cgroup
  limit and the OOM killer took down the checkpointer (crash recovery, ~2 s outage). The container now gets 1536M with
  `max_connections=150`, `shared_buffers=128MB`, `work_mem=4MB`; local dev processes cap their pools at 8
  (`deploy/dev/env.sh`).
- Secrets provider `vault` (Transit key `event-router`, mount `transit`). `vault-init` keeps
  `/opt/event-router/vault-init.json` (unseal key + root token, 0600), unseals after every Vault restart and
  creates the app token with the fixed id `ROUTER_VAULT_TOKEN` (policy `event-router-transit`).

## Demo: two Kafka clusters

| | cluster A (`kafka`) | cluster B (`kafka-b`) |
|---|---|---|
| In-stack bootstrap (connections) | `kafka:19092` | `kafka-b:19092` |
| Host / ssh tunnel (`DEV` listener) | `127.0.0.1:29092` | `127.0.0.1:29093` |
| Cluster id, volume | `eventrouter-stage-0001`, `kafkadata` | `eventrouter-stage-000b`, `kafkabdata` |

`kafka-b` is a single KRaft node like `kafka` (heap 384m, limit 768M); the api/worker/preview reach it only through
connections (`kafka-b`, `kafka-b-prod`), nothing depends on it at start-up. `demo-producer` (Go, `cmd/demoproducer`,
limit 48M) creates the demo topics with 3 partitions and writes ~1 ev/s each to `demo-orders` (A) and
`demo-payments`, `demo-raw` (~10% `bad:true`), `demo-b-in` (B). The demo routes and how to use them:
[`test/acceptance/DEMO-STAGING.md`](../../test/acceptance/DEMO-STAGING.md).

```sh
# C="docker compose -f deploy/staging/docker-compose.staging.yml --env-file /opt/event-router/.env" (see Operations)
$C stop demo-producer            # pause the demo traffic (until the next deploy.sh / `up -d`)
$C start demo-producer           # resume
# keep it off across redeploys: DEMO_PRODUCER_REPLICAS=0 in /opt/event-router/.env, then deploy.sh
# rate: DEMO_PRODUCER_RATE=<ev/s per stream> in /opt/event-router/.env
$C exec kafka-b /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-b:19092 --list
```

## Operations

```sh
ssh root@<host>
cd /opt/event-router/src
C="docker compose -f deploy/staging/docker-compose.staging.yml --env-file /opt/event-router/.env"
$C ps                    # status
$C logs -f api worker    # logs
$C exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:19092 --list
$C down                  # stop (volumes kept); `down -v` wipes data AND makes vault-init.json stale
```

If you wipe the `vaultdata` volume, also empty `/opt/event-router/vault-init.json` (`: > file`) so the
next start re-initialises Vault; existing connection secrets then become undecryptable.
