# Shared settings of the local-development scripts (sourced by tunnel.sh, env.sh, run.sh, dev.sh).
# Local processes run here; backing services (Postgres, Redis, Kafka, Prometheus) live on staging and
# are reached through one ssh tunnel. Nothing secret is stored in the repo: values come from CREDS_FILE.

DEV_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEV_LOCAL_DIR="$DEV_REPO_ROOT/.local"
DEV_RUN_DIR="$DEV_LOCAL_DIR/run"
DEV_LOG_DIR="$DEV_LOCAL_DIR/logs"
DEV_BIN_DIR="$DEV_LOCAL_DIR/bin"
DEV_ENV_FILE="$DEV_LOCAL_DIR/dev.env"
DEV_SECRETS_FILE="$DEV_LOCAL_DIR/dev-secrets.env"
CREDS_FILE="${CREDS_FILE:-$HOME/.secrets/event-router-staging.env}"

# Tunnel layout: local port -> port bound to 127.0.0.1 on the staging host.
DEV_PG_PORT="${DEV_PG_PORT:-15432}"
DEV_REDIS_PORT="${DEV_REDIS_PORT:-16379}"
DEV_KAFKA_PORT=29092
DEV_KAFKA_B_PORT=29093
DEV_PROM_PORT="${DEV_PROM_PORT:-19090}"
DEV_REMOTE_PG_PORT=5432
DEV_REMOTE_REDIS_PORT=6379
DEV_REMOTE_KAFKA_PORT=29092
DEV_REMOTE_KAFKA_B_PORT=29093
DEV_REMOTE_PROM_PORT=9090

# Local service ports.
DEV_API_PORT=8080
DEV_API_INTERNAL_PORT=8443
DEV_WORKER_PORT=8081
DEV_PREVIEW_PORT=8082
DEV_WEB_PORT="${DEV_WEB_PORT:-5173}"
DEV_WEB_HOST="${DEV_WEB_HOST:-0.0.0.0}"

# Isolation from the staging app: own database/role and Redis DB index.
DEV_DB_NAME_DEFAULT=router_dev
DEV_REDIS_DB=1

export PATH="$HOME/.local/go/bin:$PATH"

# Reads one KEY from the credentials file without sourcing it (some values contain shell syntax).
dev_cred() {
  [ -r "$CREDS_FILE" ] || { echo "dev: credentials file $CREDS_FILE not readable" >&2; return 1; }
  sed -n "s/^$1=//p" "$CREDS_FILE" | tail -n1
}

# Resolves DEV_SSH_TARGET (user@host) and DEV_SSH_PORT from the credentials file.
dev_ssh_resolve() {
  local host user
  host="${STAGING_HOST:-$(dev_cred STAGING_HOST)}"
  user="${STAGING_USER:-$(dev_cred STAGING_USER)}"
  DEV_SSH_PORT="${STAGING_SSH_PORT:-$(dev_cred STAGING_SSH_PORT)}"
  DEV_SSH_PORT="${DEV_SSH_PORT:-22}"
  [ -n "$host" ] || { echo "dev: STAGING_HOST missing in $CREDS_FILE" >&2; return 1; }
  DEV_SSH_TARGET="${user:-root}@${host}"
}

# True when the pid stored in the given pidfile belongs to a live process.
dev_pid_alive() {
  local pid
  [ -f "$1" ] || return 1
  pid="$(cat "$1" 2>/dev/null)"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

# True when something listens on 127.0.0.1:<port>.
dev_port_open() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

dev_prepare_dirs() {
  mkdir -p "$DEV_RUN_DIR" "$DEV_LOG_DIR" "$DEV_BIN_DIR" "$DEV_LOCAL_DIR/worker-cache"
  chmod 700 "$DEV_LOCAL_DIR"
}
