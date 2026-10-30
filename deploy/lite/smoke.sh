#!/bin/sh
# Smoke test of a running lite stack (run on the docker host from any directory); prints PASS/FAIL/SKIP lines
# and exits non-zero on any failure. Reads ports, URL and admin credentials from the lite .env.
# Usage: ./smoke.sh [--env-file PATH] [--url BASE_URL] [--email E] [--password P] [--new-password P] [--insecure]
#        [--metrics]   (--new-password performs the forced first password change; --insecure: curl -k on BASE_URL)
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ENV_FILE="${LITE_ENV_FILE:-$SCRIPT_DIR/.env}"
LOOPBACK="127.0.0.1"
CURL_TIMEOUT=10
HTTP_OK=200
HTTP_NO_CONTENT=204
PASSWORD_MODE="password"
PASSWORD_LOGIN_PATH="/auth/password/login"
CSRF_COOKIE="er_csrf"
CSRF_HEADER="X-CSRF-Token"
HEALTHY="healthy"
LONG_RUNNING_SERVICES="postgres redis api worker preview web"
ONE_SHOT_SERVICES="certs-init migrate"
METRICS_PROFILE="metrics"
METRICS_SERVICE="prometheus"
PROMETHEUS_JOBS="router-api router-worker router-preview"

base_url=""
email=""
password=""
new_password=""
insecure=""
metrics=""
failures=0

usage() {
  sed -n '2,5p' "$0" | sed 's/^# \{0,1\}//'
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --env-file) ENV_FILE="$2"; shift 2 ;;
      --url) base_url="$2"; shift 2 ;;
      --email) email="$2"; shift 2 ;;
      --password) password="$2"; shift 2 ;;
      --new-password) new_password="$2"; shift 2 ;;
      --insecure) insecure="-k"; shift ;;
      --metrics) metrics="on"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) echo "smoke.sh: unknown argument $1" >&2; exit 2 ;;
    esac
  done
}

env_value() {
  [ -f "$ENV_FILE" ] || return 0
  grep "^$1=" "$ENV_FILE" | tail -n 1 | cut -d= -f2- | sed -e 's/^"\(.*\)"$/\1/' -e "s/^'\(.*\)'\$/\1/"
}

pass() { echo "PASS $*"; }
skip() { echo "SKIP $*"; }
fail() {
  echo "FAIL $*"
  failures=$((failures + 1))
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

json_field() {
  printf '%s' "$1" | tr -d '\n' | grep -oE "\"$2\"[[:space:]]*:[[:space:]]*(\"[^\"]*\"|\[[^]]*\]|[a-z0-9]+)" | head -n 1 |
    sed "s/^\"$2\"[[:space:]]*:[[:space:]]*//"
}

compose() {
  (cd "$SCRIPT_DIR" && docker compose --env-file "$ENV_FILE" "$@")
}

http_status() {
  curl -s -o "$work/body" -w '%{http_code}' --max-time "$CURL_TIMEOUT" "$@" || true
}

check_web() {
  bind=$(env_value WEB_BIND)
  case "$bind" in ""|0.0.0.0) bind="$LOOPBACK" ;; esac
  url="http://$bind:$(env_value WEB_PORT)/nginx-health"
  status=$(http_status "$url")
  [ "$status" = "$HTTP_OK" ] && pass "web $url" || fail "web $url -> $status"
}

check_api_ready() {
  ca_opt="-k"
  if command -v docker >/dev/null 2>&1 && compose cp api:/certs/ca.crt "$work/ca.crt" >/dev/null 2>&1; then
    ca_opt="--cacert $work/ca.crt"
  fi
  url="https://$LOOPBACK:$(env_value API_HOST_PORT)/readyz"
  # shellcheck disable=SC2086
  status=$(http_status $ca_opt "$url")
  [ "$status" = "$HTTP_OK" ] && pass "api readiness $url ($ca_opt)" || fail "api readiness $url -> $status"
}

check_auth_config() {
  status=$(http_status $insecure "$base_url/auth/config")
  body=$(cat "$work/body")
  modes=$(json_field "$body" modes)
  login_url=$(json_field "$body" password_login_url)
  case "$modes" in
    *"\"$PASSWORD_MODE\""*) pass "auth config modes $modes" ;;
    *) fail "auth config ($status): modes=$modes" ;;
  esac
  [ "$login_url" = "\"$PASSWORD_LOGIN_PATH\"" ] && pass "password_login_url $login_url" ||
    fail "password_login_url=$login_url (want \"$PASSWORD_LOGIN_PATH\")"
}

login() {
  body="{\"email\":\"$(json_escape "$email")\",\"password\":\"$(json_escape "$password")\"}"
  status=$(http_status $insecure -c "$work/jar" -b "$work/jar" -H 'Content-Type: application/json' -X POST \
    --data "$body" "$base_url$PASSWORD_LOGIN_PATH")
  [ "$status" = "$HTTP_NO_CONTENT" ] && pass "password login as $email" || fail "password login -> $status $(cat "$work/body")"
}

check_me() {
  status=$(http_status $insecure -c "$work/jar" -b "$work/jar" "$base_url/api/v1/auth/me")
  must_change=$(json_field "$(cat "$work/body")" must_change_password)
  [ "$status" = "$HTTP_OK" ] && pass "GET /api/v1/auth/me must_change_password=${must_change:-absent}" ||
    fail "GET /api/v1/auth/me -> $status"
}

csrf_token() {
  awk -v name="$CSRF_COOKIE" '$6 == name { print $7 }' "$work/jar" | tail -n 1
}

change_password() {
  token=$(csrf_token)
  body="{\"current_password\":\"$(json_escape "$password")\",\"new_password\":\"$(json_escape "$new_password")\"}"
  status=$(http_status $insecure -c "$work/jar" -b "$work/jar" -H 'Content-Type: application/json' \
    -H "$CSRF_HEADER: $token" -X POST --data "$body" "$base_url/api/v1/auth/password")
  case "$status" in
    2??) pass "password changed (now set ROUTER_BOOTSTRAP_ADMIN_PASSWORD= empty in .env)" ;;
    *) fail "POST /api/v1/auth/password -> $status $(cat "$work/body")" ;;
  esac
}

check_auth_flow() {
  check_auth_config
  if [ -z "$password" ]; then
    skip "password login (no --password and ROUTER_BOOTSTRAP_ADMIN_PASSWORD is empty)"
    return 0
  fi
  login
  check_me
  if [ -n "$new_password" ]; then change_password; else skip "password change (no --new-password)"; fi
}

container_field() {
  id=$(compose ps -a -q "$1" 2>/dev/null | head -n 1)
  [ -n "$id" ] && docker inspect -f "$2" "$id" 2>/dev/null || true
}

check_services() {
  if ! command -v docker >/dev/null 2>&1; then
    skip "container states (docker not found)"
    return 0
  fi
  for svc in $LONG_RUNNING_SERVICES; do
    state=$(container_field "$svc" '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}')
    [ "$state" = "$HEALTHY" ] && pass "container $svc $state" || fail "container $svc ${state:-missing}"
  done
  for svc in $ONE_SHOT_SERVICES; do
    code=$(container_field "$svc" '{{.State.ExitCode}}')
    [ "$code" = "0" ] && pass "one-shot $svc exited 0" || fail "one-shot $svc exit ${code:-missing}"
  done
}

check_metrics() {
  case "$(env_value COMPOSE_PROFILES)" in *"$METRICS_PROFILE"*) metrics="on" ;; esac
  if [ "$metrics" != "on" ]; then
    skip "prometheus (profile $METRICS_PROFILE off)"
    return 0
  fi
  prom="http://$LOOPBACK:$(env_value PROMETHEUS_HOST_PORT)"
  status=$(http_status "$prom/-/ready")
  [ "$status" = "$HTTP_OK" ] && pass "$METRICS_SERVICE ready" || fail "$METRICS_SERVICE ready -> $status"
  http_status "$prom/api/v1/targets?state=active" >/dev/null
  targets=$(tr -d '\n' <"$work/body" | sed 's/},{/}\n{/g')
  for job in $PROMETHEUS_JOBS; do
    printf '%s\n' "$targets" | grep "\"job\":\"$job\"" | grep -q '"health":"up"' && pass "scrape target $job up" ||
      fail "scrape target $job not up"
  done
}

main() {
  parse_args "$@"
  [ -f "$ENV_FILE" ] || { echo "smoke.sh: $ENV_FILE not found (run ./init.sh)" >&2; exit 2; }
  base_url="${base_url:-$(env_value ROUTER_PUBLIC_URL)}"
  email="${email:-$(env_value ROUTER_BOOTSTRAP_ADMIN_EMAIL)}"
  password="${password:-$(env_value ROUTER_BOOTSTRAP_ADMIN_PASSWORD)}"
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT INT TERM
  echo "smoke: base URL $base_url"
  check_web
  check_api_ready
  check_auth_flow
  check_services
  check_metrics
  if [ "$failures" -gt 0 ]; then
    echo "smoke: $failures failure(s)"
    exit 1
  fi
  echo "smoke: all checks passed"
}

main "$@"
