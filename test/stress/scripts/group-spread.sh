#!/usr/bin/env bash
# Snapshot of partition ownership for the stress consumer groups on the local stress Kafka (S6 spread, S7 recovery).
# Usage: group-spread.sh <group-prefix> <out-file>
#   Appends a timestamped `kafka-consumer-groups --describe --members --verbose` of every group starting with the
#   prefix (which must start with stress-) to out-file
#   (one line per member with its partition count and assignment).
set -euo pipefail

SPREAD_HOME="${STRESS_HOME:-$HOME/.local/stress}"
SPREAD_BOOTSTRAP="${STRESS_BOOTSTRAP:-127.0.0.1:39092}"
SPREAD_PREFIX="${1:?group prefix}"
SPREAD_OUT="${2:?out file}"

case "$SPREAD_PREFIX" in stress-*) ;; *) echo "group-spread: prefix must start with stress-" >&2; exit 2 ;; esac
export JAVA_HOME="$SPREAD_HOME/jdk" PATH="$SPREAD_HOME/jdk/bin:$PATH"
mkdir -p "$(dirname "$SPREAD_OUT")"

groups="$("$SPREAD_HOME/kafka/bin/kafka-consumer-groups.sh" --bootstrap-server "$SPREAD_BOOTSTRAP" --list 2>/dev/null | grep "^$SPREAD_PREFIX" || true)"
{
  echo "===== $(date -Is)"
  for g in $groups; do
    "$SPREAD_HOME/kafka/bin/kafka-consumer-groups.sh" --bootstrap-server "$SPREAD_BOOTSTRAP" --describe --group "$g" \
      --members --verbose 2>/dev/null || true
  done
} >>"$SPREAD_OUT"
