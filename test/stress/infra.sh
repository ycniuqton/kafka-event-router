#!/usr/bin/env bash
# Local single-node KRaft Kafka dedicated to stress tests (127.0.0.1:39092, controller :39093).
# Pinned with taskset to STRESS_KAFKA_CPUS so it does not compete with the router under test.
# Usage: test/stress/infra.sh start|stop|status|reset    (reset wipes only the stress data dir)
set -euo pipefail

STRESS_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STRESS_HOME="${STRESS_HOME:-$HOME/.local/stress}"
STRESS_JAVA_HOME="$STRESS_HOME/jdk"
STRESS_KAFKA_HOME="$STRESS_HOME/kafka"
STRESS_DATA_DIR="$STRESS_HOME/kafka-data"
STRESS_CONF_TEMPLATE="$STRESS_REPO_ROOT/test/stress/kafka/server.properties"
STRESS_CONF_FILE="$STRESS_HOME/server.properties"
STRESS_LOG_DIR="$STRESS_REPO_ROOT/.local/logs"
STRESS_LOG_FILE="$STRESS_LOG_DIR/stress-kafka.log"
STRESS_RUN_DIR="$STRESS_REPO_ROOT/.local/run"
STRESS_PID_FILE="$STRESS_RUN_DIR/stress-kafka.pid"
STRESS_BOOTSTRAP="127.0.0.1:39092"
STRESS_KAFKA_CPUS="${STRESS_KAFKA_CPUS:-6-9}"
STRESS_KAFKA_HEAP="${STRESS_KAFKA_HEAP:--Xms4g -Xmx4g}"
STRESS_CLUSTER_ID_FILE="$STRESS_HOME/cluster.id"
STRESS_READY_SECONDS=60
STRESS_STOP_SECONDS=30

export JAVA_HOME="$STRESS_JAVA_HOME"
export PATH="$STRESS_JAVA_HOME/bin:$PATH"

# True when the recorded broker pid is alive.
kafka_alive() {
  local pid
  [ -f "$STRESS_PID_FILE" ] || return 1
  pid="$(cat "$STRESS_PID_FILE" 2>/dev/null)"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

# Renders the config and formats the KRaft log dir once (cluster id is kept across restarts).
prepare_storage() {
  mkdir -p "$STRESS_DATA_DIR" "$STRESS_LOG_DIR" "$STRESS_RUN_DIR"
  sed "s#@STRESS_LOG_DIR@#$STRESS_DATA_DIR#" "$STRESS_CONF_TEMPLATE" > "$STRESS_CONF_FILE"
  [ -s "$STRESS_CLUSTER_ID_FILE" ] || "$STRESS_KAFKA_HOME/bin/kafka-storage.sh" random-uuid > "$STRESS_CLUSTER_ID_FILE"
  [ -f "$STRESS_DATA_DIR/meta.properties" ] && return 0
  "$STRESS_KAFKA_HOME/bin/kafka-storage.sh" format --standalone \
    -t "$(cat "$STRESS_CLUSTER_ID_FILE")" -c "$STRESS_CONF_FILE" >>"$STRESS_LOG_FILE" 2>&1
}

# Polls the broker API until it answers or the timeout elapses.
wait_ready() {
  local i
  for ((i = 0; i < STRESS_READY_SECONDS; i++)); do
    kafka_alive || { echo "stress-kafka: exited during startup, see $STRESS_LOG_FILE" >&2; tail -n 20 "$STRESS_LOG_FILE" >&2; return 1; }
    "$STRESS_KAFKA_HOME/bin/kafka-broker-api-versions.sh" --bootstrap-server "$STRESS_BOOTSTRAP" >/dev/null 2>&1 && { echo "stress-kafka: ready on $STRESS_BOOTSTRAP"; return 0; }
    sleep 1
  done
  echo "stress-kafka: not ready after ${STRESS_READY_SECONDS}s, see $STRESS_LOG_FILE" >&2
  return 1
}

cmd_start() {
  kafka_alive && { echo "stress-kafka: already running (pid $(cat "$STRESS_PID_FILE"))"; return 0; }
  prepare_storage
  printf '\n===== stress-kafka start %s cpus=%s =====\n' "$(date -Is)" "$STRESS_KAFKA_CPUS" >>"$STRESS_LOG_FILE"
  KAFKA_HEAP_OPTS="$STRESS_KAFKA_HEAP" LOG_DIR="$STRESS_HOME/kafka-logs" \
    setsid taskset -c "$STRESS_KAFKA_CPUS" "$STRESS_KAFKA_HOME/bin/kafka-server-start.sh" "$STRESS_CONF_FILE" \
    >>"$STRESS_LOG_FILE" 2>&1 </dev/null &
  echo $! > "$STRESS_PID_FILE"
  echo "stress-kafka: started (pid $(cat "$STRESS_PID_FILE"), cpus $STRESS_KAFKA_CPUS, log $STRESS_LOG_FILE)"
  wait_ready
}

cmd_stop() {
  local pid i
  kafka_alive || { echo "stress-kafka: not running"; rm -f "$STRESS_PID_FILE"; return 0; }
  pid="$(cat "$STRESS_PID_FILE")"
  kill -TERM "$pid" 2>/dev/null || true
  for ((i = 0; i < STRESS_STOP_SECONDS; i++)); do
    kill -0 "$pid" 2>/dev/null || { rm -f "$STRESS_PID_FILE"; echo "stress-kafka: stopped"; return 0; }
    sleep 1
  done
  kill -KILL "$pid" 2>/dev/null || true
  rm -f "$STRESS_PID_FILE"
  echo "stress-kafka: killed after ${STRESS_STOP_SECONDS}s"
}

cmd_status() {
  kafka_alive || { echo "stress-kafka: stopped"; return 1; }
  echo "stress-kafka: running pid $(cat "$STRESS_PID_FILE") on $STRESS_BOOTSTRAP, affinity $(taskset -cp "$(cat "$STRESS_PID_FILE")" | sed 's/.*: //')"
  "$STRESS_KAFKA_HOME/bin/kafka-topics.sh" --bootstrap-server "$STRESS_BOOTSTRAP" --list
}

# Wipes only the stress data dir; the broker must be stopped first.
cmd_reset() {
  kafka_alive && { echo "stress-kafka: stop it before reset" >&2; return 1; }
  case "$STRESS_DATA_DIR" in "$STRESS_HOME"/kafka-data) ;; *) echo "stress-kafka: refusing to wipe $STRESS_DATA_DIR" >&2; return 1 ;; esac
  rm -rf "$STRESS_DATA_DIR"
  echo "stress-kafka: wiped $STRESS_DATA_DIR"
}

case "${1:-}" in
  start) cmd_start ;; stop) cmd_stop ;; status) cmd_status ;; reset) cmd_reset ;;
  *) echo "usage: $0 start|stop|status|reset" >&2; exit 2 ;;
esac
