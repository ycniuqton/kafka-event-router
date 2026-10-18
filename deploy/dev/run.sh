#!/usr/bin/env bash
# Builds and runs the Go services (migrate, api, worker, preview) and the vite dev server locally,
# against the staging backing services through the ssh tunnel (deploy/dev/tunnel.sh).
# Usage: deploy/dev/run.sh start|stop|restart|status [svc...]
#        deploy/dev/run.sh logs [svc] [-f]          svc: migrate api worker preview web tunnel
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

RUN_GO_SERVICES="api worker preview"
RUN_ALL_SERVICES="$RUN_GO_SERVICES web"
RUN_READY_SECONDS=60
RUN_STOP_SECONDS=15
RUN_LOG_LINES=200

# Maps a service to its local HTTP port.
svc_port() {
  case "$1" in
    api) echo "$DEV_API_PORT" ;; worker) echo "$DEV_WORKER_PORT" ;;
    preview) echo "$DEV_PREVIEW_PORT" ;; web) echo "$DEV_WEB_PORT" ;;
    *) echo "run: unknown service '$1'" >&2; return 1 ;;
  esac
}

svc_pidfile() { echo "$DEV_RUN_DIR/$1.pid"; }
svc_logfile() { echo "$DEV_LOG_DIR/$1.log"; }

# Ensures the tunnel is up and the env file is fresh, then loads it.
prepare_env() {
  dev_prepare_dirs
  "$DEV_REPO_ROOT/deploy/dev/tunnel.sh" start
  "$DEV_REPO_ROOT/deploy/dev/env.sh" >/dev/null
  set -a; source "$DEV_ENV_FILE"; set +a
}

build_go() {
  echo "run: building api worker preview -> $DEV_BIN_DIR"
  (cd "$DEV_REPO_ROOT" && for c in $RUN_GO_SERVICES; do go build -o "$DEV_BIN_DIR/$c" "./cmd/$c" || exit 1; done)
}

migrate_up() {
  echo "run: migrate up (log: $(svc_logfile migrate))"
  if ! (cd "$DEV_REPO_ROOT" && "$DEV_BIN_DIR/api" migrate up) >>"$(svc_logfile migrate)" 2>&1; then
    tail -n 20 "$(svc_logfile migrate)" >&2
    return 1
  fi
}

# Launches one service in its own session so stop can signal the whole process group.
launch() {
  local svc="$1" pidfile log
  pidfile="$(svc_pidfile "$svc")"; log="$(svc_logfile "$svc")"
  dev_pid_alive "$pidfile" && { echo "run: $svc already running (pid $(cat "$pidfile"))"; return 0; }
  printf '\n===== %s start %s =====\n' "$svc" "$(date -Is)" >>"$log"
  if [ "$svc" = web ]; then
    launch_web "$log"
  else
    (cd "$DEV_REPO_ROOT" || exit 1
     ROUTER_HTTP_ADDR="127.0.0.1:$(svc_port "$svc")" setsid "$DEV_BIN_DIR/$svc" >>"$log" 2>&1 < /dev/null &
     echo $! > "$pidfile")
  fi
  echo "run: $svc started (pid $(cat "$pidfile"), log $log)"
}

# Vite dev server: data source http, /api and /auth proxied to the local api.
launch_web() {
  set +u; source "$HOME/.nvm/nvm.sh" >/dev/null; set -u
  (cd "$DEV_REPO_ROOT/web" || exit 1
   VITE_DATA_SOURCE=http VITE_PROXY_TARGET="http://127.0.0.1:$DEV_API_PORT" \
     VITE_DEV_HOST="$DEV_WEB_HOST" VITE_DEV_PORT="$DEV_WEB_PORT" \
     setsid node node_modules/vite/bin/vite.js --strictPort >>"$1" 2>&1 < /dev/null &
   echo $! > "$(svc_pidfile web)")
}

# Polls the readiness endpoint (index page for web) until 200 or timeout.
wait_ready() {
  local svc="$1" url i
  url="http://127.0.0.1:$(svc_port "$svc")/readyz"
  [ "$svc" = web ] && url="http://127.0.0.1:$DEV_WEB_PORT/"
  for ((i = 0; i < RUN_READY_SECONDS; i++)); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "$url")" = 200 ] && { echo "run: $svc ready"; return 0; }
    dev_pid_alive "$(svc_pidfile "$svc")" || break
    sleep 1
  done
  echo "run: $svc not ready; last log lines:" >&2
  tail -n 30 "$(svc_logfile "$svc")" >&2
  return 1
}

stop_svc() {
  local svc="$1" pidfile pid i
  pidfile="$(svc_pidfile "$svc")"
  dev_pid_alive "$pidfile" || { rm -f "$pidfile"; echo "run: $svc not running"; return 0; }
  pid="$(cat "$pidfile")"
  kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  for ((i = 0; i < RUN_STOP_SECONDS; i++)); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
  kill -0 "$pid" 2>/dev/null && kill -KILL -- "-$pid" 2>/dev/null || true
  rm -f "$pidfile"
  echo "run: $svc stopped"
}

cmd_start() {
  local svcs="${*:-$RUN_ALL_SERVICES}" svc rc=0
  prepare_env
  case " $svcs " in *" api "*|*" worker "*|*" preview "*) build_go; migrate_up ;; esac
  for svc in $svcs; do launch "$svc"; wait_ready "$svc" || rc=1; done
  return "$rc"
}

cmd_stop() {
  local svc
  for svc in $(echo "${*:-$RUN_ALL_SERVICES}" | tr ' ' '\n' | tac); do stop_svc "$svc"; done
}

cmd_status() {
  local svc state code
  "$DEV_REPO_ROOT/deploy/dev/tunnel.sh" status || true
  for svc in ${*:-$RUN_ALL_SERVICES}; do
    dev_pid_alive "$(svc_pidfile "$svc")" && state="running pid $(cat "$(svc_pidfile "$svc")")" || state="stopped"
    code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$(svc_port "$svc")/$([ "$svc" = web ] || echo readyz)")" || true
    printf '%-8s %-22s :%s  http=%s  log=%s\n' "$svc" "$state" "$(svc_port "$svc")" "$code" "$(svc_logfile "$svc")"
  done
}

cmd_logs() {
  local svc="${1:-api}" log
  log="$(svc_logfile "$svc")"
  [ -f "$log" ] || { echo "run: no log at $log" >&2; return 1; }
  [ "${2:-}" = -f ] && exec tail -n "$RUN_LOG_LINES" -F "$log"
  tail -n "$RUN_LOG_LINES" "$log"
}

action="${1:-status}"; shift || true
case "$action" in
  start) cmd_start "$@" ;;
  stop) cmd_stop "$@" ;;
  restart) cmd_stop "$@"; cmd_start "$@" ;;
  status) cmd_status "$@" ;;
  logs) cmd_logs "$@" ;;
  *) echo "usage: $0 start|stop|restart|status [svc...] | logs [svc] [-f]" >&2; exit 2 ;;
esac
