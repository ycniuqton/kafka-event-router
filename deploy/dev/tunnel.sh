#!/usr/bin/env bash
# SSH tunnel to the staging backing services for local development (idempotent).
# Usage: deploy/dev/tunnel.sh start|stop|status|restart
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

TUNNEL_PID_FILE="$DEV_RUN_DIR/tunnel.pid"
TUNNEL_LOG_FILE="$DEV_LOG_DIR/tunnel.log"
TUNNEL_READY_SECONDS=20

# Prints "local:remote" pairs of every forwarded port.
tunnel_forwards() {
  printf '%s\n' \
    "$DEV_PG_PORT:$DEV_REMOTE_PG_PORT" \
    "$DEV_REDIS_PORT:$DEV_REMOTE_REDIS_PORT" \
    "$DEV_KAFKA_PORT:$DEV_REMOTE_KAFKA_PORT" \
    "$DEV_KAFKA_B_PORT:$DEV_REMOTE_KAFKA_B_PORT" \
    "$DEV_PROM_PORT:$DEV_REMOTE_PROM_PORT"
}

tunnel_ready() {
  local pair
  dev_pid_alive "$TUNNEL_PID_FILE" || return 1
  for pair in $(tunnel_forwards); do dev_port_open "${pair%%:*}" || return 1; done
}

# Starts one background ssh -N process carrying all forwards; fails if any port cannot be bound.
tunnel_start() {
  local args=() pair i
  dev_prepare_dirs
  tunnel_ready && { echo "tunnel: already up (pid $(cat "$TUNNEL_PID_FILE"))"; return 0; }
  dev_pid_alive "$TUNNEL_PID_FILE" && tunnel_stop
  dev_ssh_resolve
  for pair in $(tunnel_forwards); do args+=(-L "127.0.0.1:${pair%%:*}:127.0.0.1:${pair##*:}"); done
  setsid ssh -N -o BatchMode=yes -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
    -o ServerAliveCountMax=3 -o ConnectTimeout=15 -p "$DEV_SSH_PORT" "${args[@]}" "$DEV_SSH_TARGET" \
    >>"$TUNNEL_LOG_FILE" 2>&1 < /dev/null &
  echo $! > "$TUNNEL_PID_FILE"
  for ((i = 0; i < TUNNEL_READY_SECONDS; i++)); do
    tunnel_ready && { echo "tunnel: up (pid $(cat "$TUNNEL_PID_FILE"))"; return 0; }
    dev_pid_alive "$TUNNEL_PID_FILE" || break
    sleep 1
  done
  echo "tunnel: failed to start, see $TUNNEL_LOG_FILE" >&2
  return 1
}

tunnel_stop() {
  if dev_pid_alive "$TUNNEL_PID_FILE"; then
    kill "$(cat "$TUNNEL_PID_FILE")" 2>/dev/null || true
    echo "tunnel: stopped"
  else
    echo "tunnel: not running"
  fi
  rm -f "$TUNNEL_PID_FILE"
}

tunnel_status() {
  local pair state
  dev_pid_alive "$TUNNEL_PID_FILE" && echo "tunnel: pid $(cat "$TUNNEL_PID_FILE")" || echo "tunnel: not running"
  for pair in $(tunnel_forwards); do
    dev_port_open "${pair%%:*}" && state=open || state=closed
    echo "  127.0.0.1:${pair%%:*} -> staging 127.0.0.1:${pair##*:}  $state"
  done
  tunnel_ready
}

case "${1:-status}" in
  start) tunnel_start ;;
  stop) tunnel_stop ;;
  restart) tunnel_stop; tunnel_start ;;
  status) tunnel_status ;;
  *) echo "usage: $0 start|stop|status|restart" >&2; exit 2 ;;
esac
