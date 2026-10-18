#!/usr/bin/env bash
# Server-side bootstrap for the staging stack (run as root by deploy.sh, idempotent):
# firewall (22/tcp + 80/tcp only), state directory, and secrets generated once into $ER_HOME/.env.
set -euo pipefail

ER_HOME="${ER_HOME:-/opt/event-router}"
ENV_FILE="$ER_HOME/.env"
VAULT_INIT_FILE="$ER_HOME/vault-init.json"
PUBLIC_HOST="${1:?usage: bootstrap.sh <public-host>}"
LOGIN_EMAIL="${STAGING_LOGIN_EMAIL_DEFAULT:-staging-admin@example.com}"
APPROVER_EMAIL="${STAGING_APPROVER_EMAIL_DEFAULT:-staging-approver@example.com}"
HTPASSWD_IMAGE="httpd:2.4-alpine"

# Allows only ssh and http; ssh is allowed before the firewall is enabled.
configure_firewall() {
  command -v ufw > /dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw > /dev/null
  ufw allow 22/tcp > /dev/null
  ufw allow 80/tcp > /dev/null
  ufw default deny incoming > /dev/null
  ufw default allow outgoing > /dev/null
  ufw --force enable > /dev/null
}

# Creates the state directory and the secret files with owner-only permissions.
prepare_state() {
  install -d -m 700 "$ER_HOME"
  touch "$ENV_FILE" "$VAULT_INIT_FILE"
  chmod 600 "$ENV_FILE" "$VAULT_INIT_FILE"
}

# Appends KEY=value only when KEY is not yet present, so re-deploys never regenerate secrets.
ensure_key() {
  local key="$1" value="$2"
  grep -q "^${key}=" "$ENV_FILE" || printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
}

# Prints the current value of KEY from the env file (single quotes stripped).
read_key() {
  sed -n "s/^$1=//p" "$ENV_FILE" | head -n1 | sed "s/^'\(.*\)'$/\1/"
}

# Generates a random hex secret of the given byte length.
random_hex() {
  openssl rand -hex "$1"
}

# Writes every generated secret and the non-secret staging settings.
generate_secrets() {
  ensure_key STAGING_URL "http://${PUBLIC_HOST}"
  ensure_key ER_HOME "$ER_HOME"
  ensure_key POSTGRES_USER router
  ensure_key POSTGRES_DB router
  ensure_key POSTGRES_PASSWORD "$(random_hex 24)"
  ensure_key REDIS_PASSWORD "$(random_hex 24)"
  ensure_key ROUTER_CURSOR_SECRET "$(random_hex 32)"
  ensure_key ROUTER_COOKIE_SECRET "$(random_hex 32)"
  ensure_key ROUTER_WORKLOAD_TOKEN "$(random_hex 32)"
  ensure_key ROUTER_PREVIEW_DIRECTIVE_SECRET "$(random_hex 32)"
  ensure_key ROUTER_VAULT_TOKEN "$(random_hex 24)"
  ensure_key DEX_CLIENT_SECRET "$(random_hex 32)"
  ensure_key STAGING_LOGIN_EMAIL "$LOGIN_EMAIL"
  ensure_key STAGING_LOGIN_PASSWORD "$(random_hex 12)"
  ensure_key STAGING_APPROVER_EMAIL "$APPROVER_EMAIL"
  ensure_key STAGING_APPROVER_PASSWORD "$(random_hex 12)"
}

# Writes the non-secret feature settings (assist, live tail, alerts, route metrics); existing values win.
generate_feature_settings() {
  ensure_key ROUTER_ROUTE_METRICS_READER prometheus
  ensure_key ROUTER_EVENTS_BUS redis
  ensure_key ROUTER_LIVETAIL_TAP redis
  ensure_key ROUTER_ASSIST_PROVIDER mock
  ensure_key ROUTER_ALERTS_ENABLED true
  ensure_key ROUTER_ALERTS_CHANNELS slack,teams,webhook
}

# Stores the bcrypt hash of a Dex static user's password as HASH_KEY (single-quoted: it contains '$').
generate_dex_hash() {
  local hash_key="$1" password_key="$2" hash
  grep -q "^${hash_key}=" "$ENV_FILE" && return 0
  hash="$(read_key "$password_key" | docker run --rm -i "$HTPASSWD_IMAGE" htpasswd -niBC 10 staging | cut -d: -f2 | tr -d '\r\n')"
  [ -n "$hash" ] || { echo "bootstrap: bcrypt hash generation failed ($hash_key)" >&2; exit 1; }
  printf "%s='%s'\n" "$hash_key" "${hash/#\$2y\$/\$2a\$}" >> "$ENV_FILE"
}

configure_firewall
prepare_state
generate_secrets
generate_feature_settings
generate_dex_hash DEX_STAGING_USER_HASH STAGING_LOGIN_PASSWORD
generate_dex_hash DEX_STAGING_APPROVER_HASH STAGING_APPROVER_PASSWORD
echo "bootstrap: ok ($ENV_FILE, $VAULT_INIT_FILE)"
