#!/usr/bin/env bash
# Compose smoke check: waits until api/worker/preview /readyz and the web root answer 200.
# Usage: deploy/docker/smoke.sh [timeout_seconds]   (after `make up`; used by the compose-smoke CI job)
set -euo pipefail

TIMEOUT="${1:-180}"
POLL_SECONDS=3
HOST="${SMOKE_HOST:-127.0.0.1}"
CHECKS=(
  "api|http://${HOST}:8080/readyz"
  "worker|http://${HOST}:8081/readyz"
  "preview|http://${HOST}:8082/readyz"
  "web|${SMOKE_WEB_URL:-http://${HOST}:3000/}"
)

status_of() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$1" || true
}

deadline=$(( $(date +%s) + TIMEOUT ))
pending=("${CHECKS[@]}")
while (( ${#pending[@]} > 0 )); do
  still=()
  for check in "${pending[@]}"; do
    name="${check%%|*}"; url="${check#*|}"
    code="$(status_of "$url")"
    if [[ "$code" == "200" ]]; then echo "ok    $name $url"; else still+=("$check"); fi
  done
  pending=("${still[@]+"${still[@]}"}")
  (( ${#pending[@]} == 0 )) && break
  if (( $(date +%s) >= deadline )); then
    for check in "${pending[@]}"; do echo "FAIL  ${check%%|*} ${check#*|} -> $(status_of "${check#*|}")"; done
    exit 1
  fi
  sleep "$POLL_SECONDS"
done
echo "stack healthy"
