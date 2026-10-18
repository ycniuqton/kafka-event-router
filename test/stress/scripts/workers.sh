#!/usr/bin/env bash
# Local worker control for stress scenarios (S6 scale-out, S7 kill/restart). Uses the binary and env file that
# deploy/dev/run.sh built (.local/bin/worker, .local/dev.env); never touches secrets beyond what run.sh prepared.
# Usage: workers.sh scale N            run N workers in total: the dev worker (#1, :8081) + stress-worker-2..N (:8091..)
#        workers.sh restart-main       start the dev worker again after a kill (no rebuild), pinned like the target
#        workers.sh status
# Pinning in scale mode: worker i gets cores STRESS_WORKER_CPUS_<i> (defaults 0-1, 2-3, 4-5).
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../../../deploy/dev/common.sh"

STRESS_EXTRA_PORT_BASE=8089
STRESS_MAX_WORKERS=3
STRESS_READY_SECONDS=60
STRESS_MAIN_CPUS="${STRESS_MAIN_CPUS:-0-5}"
declare -A STRESS_CPUS=([1]="${STRESS_WORKER_CPUS_1:-0-1}" [2]="${STRESS_WORKER_CPUS_2:-2-3}" [3]="${STRESS_WORKER_CPUS_3:-4-5}")

extra_pidfile() { echo "$DEV_RUN_DIR/stress-worker-$1.pid"; }
extra_port() { echo $((STRESS_EXTRA_PORT_BASE + $1)); }

load_env() {
  [ -f "$DEV_ENV_FILE" ] || { echo "workers: $DEV_ENV_FILE missing (run deploy/dev/run.sh start first)" >&2; exit 1; }
  set -a; source "$DEV_ENV_FILE"; set +a
}

wait_ready() {
  local port="$1" i
  for ((i = 0; i < STRESS_READY_SECONDS; i++)); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/readyz")" = 200 ] && return 0
    sleep 1
  done
  echo "workers: worker on :$port not ready after ${STRESS_READY_SECONDS}s" >&2; return 1
}

# Starts stress-worker-i with its own id, port and cache dir.
start_extra() {
  local i="$1" pidfile port
  pidfile="$(extra_pidfile "$i")"; port="$(extra_port "$i")"
  dev_pid_alive "$pidfile" && return 0
  (load_env
   cd "$DEV_REPO_ROOT"
   ROUTER_WORKER_ID="${ROUTER_WORKER_ID}-stress-$i" ROUTER_HTTP_ADDR="127.0.0.1:$port" \
     ROUTER_WORKER_CACHE_DIR="$DEV_LOCAL_DIR/worker-cache-stress-$i" \
     setsid taskset -c "${STRESS_CPUS[$i]}" "$DEV_BIN_DIR/worker" >>"$DEV_LOG_DIR/stress-worker-$i.log" 2>&1 </dev/null &
   echo $! >"$pidfile")
  wait_ready "$port"
  echo "workers: stress-worker-$i up (pid $(cat "$pidfile"), :$port, cpus ${STRESS_CPUS[$i]})"
}

stop_extra() {
  local pidfile; pidfile="$(extra_pidfile "$1")"
  if dev_pid_alive "$pidfile"; then kill -TERM "$(cat "$pidfile")"; echo "workers: stress-worker-$1 stopped"; fi
  rm -f "$pidfile"
}

cmd_scale() {
  local n="$1" i
  [ "$n" -ge 1 ] && [ "$n" -le "$STRESS_MAX_WORKERS" ] || { echo "workers: N must be 1..$STRESS_MAX_WORKERS" >&2; exit 2; }
  dev_pid_alive "$DEV_RUN_DIR/worker.pid" || { echo "workers: dev worker not running" >&2; exit 1; }
  local main_cpus="${STRESS_CPUS[1]}"; [ "$n" -eq 1 ] && main_cpus="$STRESS_MAIN_CPUS"
  taskset -acp "$main_cpus" "$(cat "$DEV_RUN_DIR/worker.pid")" >/dev/null
  for ((i = 2; i <= STRESS_MAX_WORKERS; i++)); do
    if [ "$i" -le "$n" ]; then start_extra "$i"; else stop_extra "$i"; fi
  done
}

cmd_restart_main() {
  local pidfile="$DEV_RUN_DIR/worker.pid"
  dev_pid_alive "$pidfile" && { echo "workers: dev worker already running"; return 0; }
  (load_env
   cd "$DEV_REPO_ROOT"
   ROUTER_HTTP_ADDR="127.0.0.1:$DEV_WORKER_PORT" setsid taskset -c "$STRESS_MAIN_CPUS" "$DEV_BIN_DIR/worker" \
     >>"$DEV_LOG_DIR/worker.log" 2>&1 </dev/null &
   echo $! >"$pidfile")
  wait_ready "$DEV_WORKER_PORT"
  echo "workers: dev worker restarted (pid $(cat "$pidfile"))"
}

cmd_status() {
  local i
  echo "worker-1 pid $(cat "$DEV_RUN_DIR/worker.pid" 2>/dev/null || echo -) :$DEV_WORKER_PORT"
  for ((i = 2; i <= STRESS_MAX_WORKERS; i++)); do
    echo "stress-worker-$i pid $(cat "$(extra_pidfile "$i")" 2>/dev/null || echo -) :$(extra_port "$i")"
  done
}

case "${1:-}" in
  scale) cmd_scale "${2:?N}" ;;
  restart-main) cmd_restart_main ;;
  status) cmd_status ;;
  *) echo "usage: $0 scale N | restart-main | status" >&2; exit 2 ;;
esac
