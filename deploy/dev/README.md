# Local development against staging backing services

Code runs **here**: `migrate`, `api`, `worker`, `preview` (Go) and the web dev server (vite). Postgres,
Redis, Kafka and Prometheus come from the **staging host**, through one ssh tunnel. You do not redeploy
staging for each change. All state, logs and generated env stay in `./.local/` (gitignored).

```
local machine                                   staging host (127.0.0.1-only ports)
  vite :5173 (0.0.0.0) --/api,/auth--> api :8080
  api :8080 / :8443, worker :8081, preview :8082
  127.0.0.1:15432 ──ssh -L──>  postgres :5432   database router_dev (role router_dev)
  127.0.0.1:16379 ──ssh -L──>  redis :6379      DB index 1 (staging app uses 0)
  127.0.0.1:29092 ──ssh -L──>  kafka :29092     DEV listener, advertised 127.0.0.1:29092
  127.0.0.1:29093 ──ssh -L──>  kafka-b :29093   second demo cluster, advertised 127.0.0.1:29093
  127.0.0.1:19090 ──ssh -L──>  prometheus :9090
```

## Prerequisites

- ssh key access to the staging host (`ssh -o BatchMode=yes root@<host>` works).
- `~/.secrets/event-router-staging.env` (0600) with `STAGING_HOST`, `REDIS_PASSWORD`,
  `ROUTER_DEV_DB_USER`, `ROUTER_DEV_DB_NAME`, `ROUTER_DEV_DB_PASSWORD`. The scripts read single keys from
  it (they never `source` it: some values contain shell syntax) and never copy them into the repo.
- Go in `~/.local/go/bin` (added to PATH by the scripts), Node via `~/.nvm/nvm.sh`, `web/node_modules`
  installed (`cd web && npm ci`).

## Commands (no `make` needed)

```sh
deploy/dev/dev.sh up              # tunnel + build + migrate + api/worker/preview + vite
deploy/dev/dev.sh status          # tunnel ports, pids, /readyz codes, log paths
deploy/dev/dev.sh down            # stop everything incl. tunnel

deploy/dev/tunnel.sh start|stop|status|restart
deploy/dev/run.sh start|stop|restart|status [api worker preview web]
deploy/dev/run.sh restart api     # rebuild + migrate + restart just the api after a code change
deploy/dev/run.sh logs worker     # last 200 lines;  add -f to follow
source deploy/dev/env.sh          # export the env into your shell, e.g. for `go run ./cmd/api` or tests
```

Logs: `.local/logs/{migrate,api,worker,preview,web,tunnel}.log` (JSON lines for Go services).
Pidfiles: `.local/run/*.pid`. Binaries: `.local/bin/`. Generated env: `.local/dev.env` (0600).

Web: http://localhost:5173 (also `http://<this-machine>:5173`), `VITE_DATA_SOURCE=http`, `/api` and
`/auth` proxied to the local api (`VITE_PROXY_TARGET`, see `web/vite.config.ts`). Sign in with
**dev login** (`ROUTER_ENV=local`, `ROUTER_AUTH_MODES=dev`, user `admin@local.dev`). Secrets use the local
key-file provider (`.local/master.key`); no Vault is needed.

## Isolation and sharing

- **Database `router_dev`** on the staging Postgres server, owned by role `router_dev` (which cannot connect
  to the staging app database). Local workers/scheduler therefore never see or lease staging routes.
- **Redis DB 1** (sessions, leases, preview); staging uses DB 0.
- **Kafka is shared** with staging. Topics are namespaced by route name (`router.<route>` consumer groups,
  `router.dlq.<route>`), so pick route names that do not collide with staging routes (e.g. a `dev-` prefix).
- **Connections created in local dev must use bootstrap `127.0.0.1:29092`** (the tunnel); `kafka:19092`
  only resolves inside the staging network. Such connections are only valid in local dev.
- Prometheus is staging's: it scrapes staging services, not your local processes, so route metrics in the
  local UI show staging-side data or nothing.

## Producing / consuming test events

Use the kafka container on the server (inside the network the address is `kafka:19092`):

```sh
ssh root@<host>
cd /opt/event-router/src
K="docker compose -f deploy/staging/docker-compose.staging.yml --env-file /opt/event-router/.env exec -T kafka /opt/kafka/bin"
echo '{"id":1}' | $K/kafka-console-producer.sh --bootstrap-server kafka:19092 --topic dev-in
$K/kafka-console-consumer.sh --bootstrap-server kafka:19092 --topic dev-out --from-beginning --timeout-ms 10000
```

## Staging-side setup (done once; see deploy/staging/README.md)

- `docker-compose.staging.yml` publishes redis `127.0.0.1:6379`, prometheus `127.0.0.1:9090` and the kafka
  `DEV` listener `127.0.0.1:29092` (loopback only; ufw still allows only 22/80).
- `router_dev` role + database created on the staging Postgres; its password is only in the credentials file.

## Troubleshooting

- `tunnel: failed to start` → see `.local/logs/tunnel.log`; a local port (15432/16379/29092/19090) may be
  taken. Override with `DEV_PG_PORT`, `DEV_REDIS_PORT`, `DEV_PROM_PORT` (29092/29093 are fixed: they are advertised).
- A service is not ready → `deploy/dev/run.sh logs <svc>`; after a laptop sleep run `tunnel.sh restart`.
