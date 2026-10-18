#!/usr/bin/env bash
# Deploys the Event Router staging stack from this machine (idempotent; safe to re-run):
# rsync repo -> server, server bootstrap (firewall, secrets once), compose up --build, wait for health,
# then append the human-needed credentials to the local credentials file without printing them.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CREDS_FILE="${CREDS_FILE:-$HOME/.secrets/event-router-staging.env}"
ER_HOME="/opt/event-router"
REMOTE_SRC="$ER_HOME/src"
REMOTE_ENV="$ER_HOME/.env"
REMOTE_VAULT_INIT="$ER_HOME/vault-init.json"
COMPOSE_FILE="deploy/staging/docker-compose.staging.yml"
HEALTH_TIMEOUT_SECONDS="${HEALTH_TIMEOUT_SECONDS:-600}"
HEALTH_POLL_SECONDS=10
EXPECTED_HEALTHY="postgres redis kafka kafka-b prometheus vault vault-init dex api worker preview web"

# Reads one KEY from the local credentials file (never echoed to the terminal by callers).
creds_value() {
  sed -n "s/^$1=//p" "$CREDS_FILE" | tail -n1
}

# Resolves ssh target and options from the credentials file (host/user/port; the password is not used).
resolve_target() {
  STAGING_HOST="${STAGING_HOST:-$(creds_value STAGING_HOST)}"
  STAGING_USER="${STAGING_USER:-$(creds_value STAGING_USER)}"
  STAGING_SSH_PORT="${STAGING_SSH_PORT:-$(creds_value STAGING_SSH_PORT)}"
  STAGING_USER="${STAGING_USER:-root}"
  STAGING_SSH_PORT="${STAGING_SSH_PORT:-22}"
  [ -n "$STAGING_HOST" ] || { echo "deploy: STAGING_HOST missing in $CREDS_FILE" >&2; exit 1; }
  TARGET="${STAGING_USER}@${STAGING_HOST}"
  SSH=(ssh -o BatchMode=yes -o ConnectTimeout=15 -p "$STAGING_SSH_PORT" "$TARGET")
}

# Mirrors the repository to the server, excluding build output, dependencies and local tooling state.
sync_source() {
  "${SSH[@]}" "install -d -m 700 $ER_HOME && install -d $REMOTE_SRC"
  rsync -az --delete -e "ssh -o BatchMode=yes -p $STAGING_SSH_PORT" \
    --exclude node_modules --exclude web/node_modules --exclude web/dist --exclude .git \
    --exclude .devlens/html --exclude .claude --exclude '.env' --exclude 'deploy/.env' \
    --exclude .local --exclude bin \
    "$REPO_ROOT/" "$TARGET:$REMOTE_SRC/"
}

# Runs the server bootstrap: firewall, state dir, one-time secret generation.
bootstrap_server() {
  "${SSH[@]}" "bash $REMOTE_SRC/deploy/staging/server/bootstrap.sh $STAGING_HOST"
}

# Builds and (re)starts the whole stack on the server.
compose_up() {
  "${SSH[@]}" "cd $REMOTE_SRC && docker compose -f $COMPOSE_FILE --env-file $REMOTE_ENV up -d --build --remove-orphans"
}

# Lists services that are not yet healthy (migrate must have exited 0).
unhealthy_services() {
  "${SSH[@]}" "cd $REMOTE_SRC && docker compose -f $COMPOSE_FILE --env-file $REMOTE_ENV ps -a --format '{{.Service}} {{.State}} {{.Health}} {{.ExitCode}}'" |
    awk -v want="$EXPECTED_HEALTHY" '
      BEGIN { n = split(want, w, " "); for (i = 1; i <= n; i++) need[w[i]] = 1 }
      $1 == "migrate" { if ($2 != "exited" || $NF != 0) print "migrate"; next }
      ($1 in need) { if ($3 != "healthy") print $1; delete need[$1] }
      END { for (s in need) print s }'
}

# Polls container health until everything is healthy or the timeout elapses.
wait_for_health() {
  local deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS)) pending
  while :; do
    pending="$(unhealthy_services | tr '\n' ' ')"
    [ -z "${pending// /}" ] && { echo "deploy: all services healthy"; return 0; }
    [ "$SECONDS" -ge "$deadline" ] && { echo "deploy: still not healthy: $pending" >&2; return 1; }
    echo "deploy: waiting for: $pending"
    sleep "$HEALTH_POLL_SECONDS"
  done
}

# Fetches the server values the humans need, as KEY=value lines (stdout is captured, not shown).
remote_credentials() {
  "${SSH[@]}" "python3 - $REMOTE_ENV $REMOTE_VAULT_INIT" <<'PY'
import json, sys
env = {}
for line in open(sys.argv[1]):
    if '=' in line and not line.startswith('#'):
        k, v = line.rstrip('\n').split('=', 1)
        env[k] = v.strip("'")
raw = open(sys.argv[2]).read().strip()
init = json.loads(raw) if raw else {'unseal_keys_b64': [''], 'root_token': ''}
out = {k: env.get(k, '') for k in ('STAGING_URL', 'STAGING_LOGIN_EMAIL', 'STAGING_LOGIN_PASSWORD',
       'STAGING_APPROVER_EMAIL', 'STAGING_APPROVER_PASSWORD',
       'POSTGRES_USER', 'POSTGRES_PASSWORD', 'POSTGRES_DB', 'ROUTER_WORKLOAD_TOKEN', 'ROUTER_VAULT_TOKEN', 'REDIS_PASSWORD')}
out['VAULT_UNSEAL_KEY'] = init['unseal_keys_b64'][0]
out['VAULT_ROOT_TOKEN'] = init['root_token']
for k, v in out.items():
    print(f'{k}={v}')
PY
}

# Appends keys missing from the local credentials file; existing lines are kept untouched.
append_credentials() {
  local fetched line key added=0
  fetched="$(remote_credentials)"
  fetched+=$'\n'"STAGING_POSTGRES_DEBUG=ssh -L 5432:127.0.0.1:5432 ${TARGET} (then localhost:5432)"
  fetched+=$'\n'"STAGING_SERVER_ENV_FILE=$REMOTE_ENV"
  fetched+=$'\n'"STAGING_SERVER_VAULT_INIT_FILE=$REMOTE_VAULT_INIT"
  fetched+=$'\n'"STAGING_SERVER_SRC_DIR=$REMOTE_SRC"
  fetched+=$'\n'"STAGING_COMPOSE_CMD=cd $REMOTE_SRC && docker compose -f $COMPOSE_FILE --env-file $REMOTE_ENV"
  grep -q '^# --- staging deploy' "$CREDS_FILE" || printf '\n# --- staging deploy (appended by deploy/staging/deploy.sh) ---\n' >> "$CREDS_FILE"
  while IFS= read -r line; do
    key="${line%%=*}"
    [ -z "$key" ] || [ -z "${line#*=}" ] && continue
    grep -q "^${key}=" "$CREDS_FILE" && continue
    printf '%s\n' "$line" >> "$CREDS_FILE"
    added=$((added + 1))
  done <<< "$fetched"
  chmod 600 "$CREDS_FILE"
  echo "deploy: $added new keys appended to $CREDS_FILE"
}

resolve_target
sync_source
bootstrap_server
compose_rc=0
compose_up || compose_rc=$?
append_credentials
[ "$compose_rc" -eq 0 ] || { echo "deploy: docker compose up failed ($compose_rc)" >&2; exit "$compose_rc"; }
wait_for_health
echo "deploy: done -> $(creds_value STAGING_URL)"
