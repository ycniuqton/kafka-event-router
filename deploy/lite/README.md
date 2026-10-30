# Event Router — lite (single host, Postgres + Redis only)

A small, self-contained Docker Compose install (project `event-router-lite`) for one host. Infrastructure is
only **Postgres 16** and **Redis 7**: no Vault (connection credentials are encrypted with a static master key),
no OIDC/Dex (email + password login), no Kafka of its own (routes use the Kafka clusters you add as
connections). Prometheus is optional (`metrics` profile).

| Service | Role | Host port |
|---|---|---|
| `postgres`, `redis` | metadata store, sessions/samples/live tail/events bus | `127.0.0.1:5432`, `127.0.0.1:6379` |
| `certs-init` (one-shot) | internal CA + mTLS certificates into the `certs` volume | — |
| `migrate` (one-shot) | `api migrate up` before the api starts | — |
| `api` | control plane: public `:8080`, internal mTLS `:8443` | `127.0.0.1:8080` (HTTPS, internal CA) |
| `worker`, `preview` | data plane, sample/preview runs | — |
| `web` | nginx: SPA + `/api` (incl. WebSocket) + `/auth` proxy | `WEB_BIND:WEB_PORT` (default `0.0.0.0:8088`) |
| `prometheus` (profile `metrics`) | scrapes api/worker/preview `/metrics` | `127.0.0.1:9090` |

All host ports except `web` bind to loopback (change them with the `*_HOST_PORT` keys). Docker bypasses host
firewalls for published ports, so do not publish anything else on a public address.

## Quick start

```sh
cd deploy/lite
./init.sh --admin-email you@example.com --public-url https://router.example.com   # writes .env (0600)
docker compose up -d --build
./smoke.sh            # after everything is healthy (docker compose ps)
```

`init.sh` options: `--env-file PATH` (or `LITE_ENV_FILE`), `--trial` (HTTP trial mode), `--metrics`,
`--yes` (no prompts). It generates the Postgres/Redis passwords, cookie/cursor/preview-directive secrets, the
webhook signing key, the master key (`openssl rand -base64 32`) and a 20-character admin password that it
prints **once**. It never changes a key that already exists in `.env`; rerunning only appends keys missing from
`.env.example` (so upgrades that add settings are picked up). Every key is documented in `.env.example`.
Keep values unquoted and without `$` (compose interpolates it; write `$$`).

## Modes

**Production (default, `ROUTER_ENV=production`).** The platform guards require secure cookies
(`ROUTER_COOKIE_SECURE=true`), so browsers must reach the UI over HTTPS. Pick one:

- *TLS terminator in front* (Caddy, Traefik, a cloud load balancer, host nginx): forward to
  `WEB_BIND:WEB_PORT`, send `X-Forwarded-Proto: https` (the lite nginx passes it through) and keep WebSocket
  upgrades. With a proxy on the same host set `WEB_BIND=127.0.0.1`.
- *TLS in the web nginx*: in `.env` set
  `COMPOSE_FILE=docker-compose.yml:docker-compose.tls.yml`, `WEB_TLS_CERT_FILE` / `WEB_TLS_KEY_FILE` (host paths,
  e.g. certbot `fullchain.pem` / `privkey.pem`) and optionally `WEB_TLS_PORT` (default 443). nginx then serves
  HTTPS on `WEB_TLS_PORT` and redirects plain HTTP (the redirect assumes 443). Reload after renewing the cert:
  `docker compose exec web nginx -s reload`.

`ROUTER_PUBLIC_URL` must be the exact HTTPS origin users open: it is the allowed WebSocket origin and the base
of links in alerts.

**HTTP trial (`./init.sh --trial`).** `ROUTER_ENV=staging` + `ROUTER_COOKIE_SECURE=false` +
`ROUTER_PUBLIC_URL=http://localhost:8088`: works over plain HTTP for a quick evaluation. Session cookies and
passwords travel unencrypted — **not for real data or credentials**. Everything else (mTLS, static key,
password login) is identical, so moving to production is: TLS in front, then `ROUTER_ENV=production`,
`ROUTER_COOKIE_SECURE=true`, the HTTPS `ROUTER_PUBLIC_URL`, and `docker compose up -d`.

## Internal listener: why mTLS and `certs-init`

Worker and preview fetch secrets, assignments and salts from the api's internal listener. Outside `local`/
`staging` the guards (`internal/platform/config/guard.go`) reject `ROUTER_WORKLOAD_AUTH=static_token` and an
internal listener without client certificates, and `k8s_token` needs Kubernetes. So lite uses `mtls`, in both
modes:

- `certs-init` (alpine + openssl) creates a private CA once; its key lives only in the `certsca` volume.
  Into the `certs` volume it writes `ca.crt`, `api.crt/key` (SAN `api`, `localhost`, `127.0.0.1`, server auth)
  and `worker.crt/key`, `preview.crt/key` (CN = workload name, client auth). Keys are `0400`, owned by the
  distroless `nonroot` user (65532). Valid files are never regenerated; a leaf is re-issued only when missing,
  within 30 days of expiry (leaves last 825 days, the CA 10 years) or not signed by the current CA.
- The api verifies client certificates against `ca.crt` and only accepts the names in
  `ROUTER_WORKLOAD_MTLS_ALLOWED=worker,preview`.
- `ROUTER_TLS_CERT_FILE` / `_KEY_FILE` apply to **both** api listeners, so the public `:8080` listener is HTTPS
  too (with the internal certificate). nginx and Prometheus therefore talk `https://api:8080` and verify it
  against `ca.crt`; the loopback port `127.0.0.1:8080` is HTTPS as well (`curl --cacert` or `-k`).

Certificate renewal happens on the next `docker compose up -d` when a leaf nears expiry; the api reloads its
certificate by itself, restart the workers afterwards: `docker compose restart worker preview`. To rebuild all
internal certificates (e.g. a suspected key leak): `docker compose down`, `docker volume rm
event-router-lite_certs event-router-lite_certsca`, `docker compose up -d`.

## Metrics on or off

| `.env` | metrics off (default) | metrics on |
|---|---|---|
| `COMPOSE_PROFILES` | *(empty)* | `metrics` |
| `ROUTER_ROUTE_METRICS_READER` | `null` | `prometheus` |
| `ROUTER_ALERTS_ENABLED` | `false` | `true` |
| `ROUTER_MONITORING_ATTENTION_DETECTORS` | `route_error,connection_unhealthy,alert_firing` | *(empty = all)* |

`ROUTER_METRICS_BACKEND=prometheus` / `ROUTER_METRICS_URL=http://prometheus:9090` stay as they are (the only
non-mock backend). Without Prometheus the dashboard reports *Unavailable* metric cards, the flow map draws
without throughput, the route Overview tab shows its "metrics unavailable" retry state and the routes list shows
"—". Alert rules are metric expressions, so the evaluator is switched off (rules can still be edited). After
changing these keys run `docker compose up -d` (with metrics on it starts Prometheus and recreates the api).

## First login

Open `ROUTER_PUBLIC_URL`, sign in with `ROUTER_BOOTSTRAP_ADMIN_EMAIL` and the password `init.sh` printed (also in
`.env`). The first start creates that admin (workspace admin) with a forced password change. After changing it,
set `ROUTER_BOOTSTRAP_ADMIN_PASSWORD=` (empty, keep the key so `init.sh` does not generate a new one). Failed
logins lock the account for `ROUTER_LOGIN_LOCKOUT` (15m) after `ROUTER_LOGIN_MAX_FAILURES` (5). Scripts use API
tokens (`ROUTER_AUTH_MODES=password,api_token`).

## Upgrades

```sh
git pull                       # or copy the new source tree
cd deploy/lite && ./init.sh    # appends settings new releases added; existing values untouched
docker compose up -d --build   # migrate runs first; api starts only if it succeeded
docker compose logs migrate    # on failure
```

Back up first (below). Migrations are forward-only in practice; `docker compose run --rm migrate migrate status`
shows the schema version.

## Resource sizing

Limits: postgres 1 CPU / 1536M (`max_connections=150`, `shared_buffers=128MB`), redis 0.5 / 256M
(`maxmemory 200mb`, AOF on), api, worker, preview 1 CPU / 512M each, web 0.5 / 128M, prometheus 0.5 / 384M
(7 days, `PROMETHEUS_RETENTION`). A 4 vCPU / 8 GB host is comfortable; 2 vCPU / 4 GB works for evaluation.
The api/preview Postgres pools use `pool_max_conns=ROUTER_DB_POOL_MAX_CONNS` (16).

## Backup and disaster recovery

Back up together, and keep them apart from the host:

1. **Postgres** — all configuration, users, routes, encrypted connection credentials:
   `docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > router-$(date +%F).dump`
2. **`.env`** — above all `ROUTER_SECRETS_MASTER_KEY` (and any `ROUTER_SECRETS_PREVIOUS_MASTER_KEYS`). Without
   the key that encrypted them, saved connection credentials in the dump are **unrecoverable** (you would have
   to re-enter every Kafka/registry password). A dump taken before a key rotation needs the old key.
   Losing the cookie/cursor secrets only logs users out.
3. *Optional:* the `certs`/`certsca` volumes — they can always be regenerated (see above).

Redis holds only short-lived data (sessions, samples) and Prometheus only metrics; neither needs a backup.

Restore on a new host: copy the source tree and `.env` to `deploy/lite`, `docker compose up -d postgres`, then
`docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < router.dump`
and `docker compose up -d --build`.

## Master key rotation

1. `openssl rand -base64 32` → new key.
2. In `.env`: move the current value to `ROUTER_SECRETS_PREVIOUS_MASTER_KEYS` (comma-separated, decrypt-only)
   and put the new one in `ROUTER_SECRETS_MASTER_KEY`. Back up `.env`.
3. `docker compose up -d` (recreates migrate/api with the new keys).
4. `docker compose exec api /app secrets rotate` — re-wraps every data key to the new master key and prints
   `rotated=N`, then the per-key usage; `docker compose exec api /app secrets status` must show only the new
   key id. (The api leader also re-wraps in the background every `ROUTER_SECRETS_ROTATION_INTERVAL`, 24h.)
5. Remove the old key from `ROUTER_SECRETS_PREVIOUS_MASTER_KEYS`, `docker compose up -d`. Keep the old key in
   your offline backup for as long as you keep dumps taken before the rotation.

## Checks

- `python3 validate.py [--env-file .env]` — static checks, no docker: YAML, `depends_on`, every `${VAR}`
  documented and required ones set, every `ROUTER_*` key read by the Go binaries, volumes/networks, profiles,
  healthcheck binaries, loopback ports, master key shape, `sh -n` of the scripts.
- `./smoke.sh [--new-password P] [--metrics]` — against the running stack: web health, api readiness, password
  login, `/api/v1/auth/me`, optional forced password change, container states, Prometheus targets. It logs in
  through `ROUTER_PUBLIC_URL` (production: HTTPS, since secure cookies are not sent over HTTP).

## Known limits

- The api healthcheck runs the binary's own probe, `/app health https://127.0.0.1:8080/readyz` (the public
  listener is HTTPS, see above). It verifies the certificate against `ROUTER_TLS_CLIENT_CA_FILE` (the internal
  CA, which signed `api.crt` with SAN `127.0.0.1`), so it does not depend on busybox `wget` having TLS. If the api
  never turns healthy, run `docker compose exec api /app health https://127.0.0.1:8080/readyz` to see why.
- nginx sends the address it sees as `X-Real-IP`. Behind a TLS terminator that is the proxy, so the per-IP
  login limit (`ROUTER_LOGIN_IP_PER_MINUTE`) counts per proxy; add `set_real_ip_from <proxy address>;` and
  `real_ip_header X-Forwarded-For;` to the `server` block of `nginx/default.conf` to use the client address.
