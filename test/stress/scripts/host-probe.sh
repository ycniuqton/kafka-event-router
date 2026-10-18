#!/usr/bin/env bash
# Host I/O probe for stress cases: every PROBE_INTERVAL seconds appends one line with the dirty / writeback page cache,
# the share of CPU time in iowait and steal since the previous line, and the number of tasks blocked on I/O.
# Usage: host-probe.sh <out-file> <seconds>     (meant to be fired as a scenario action at 0s, runs in background)
set -euo pipefail

PROBE_OUT="${1:?out file}"
PROBE_SECONDS="${2:?seconds}"
PROBE_INTERVAL="${PROBE_INTERVAL:-5}"

mkdir -p "$(dirname "$PROBE_OUT")"
read -r _ u n s i w q sq st _ < /proc/stat
prev_total=$((u + n + s + i + w + q + sq + st)); prev_wait=$w; prev_steal=$st
echo "time dirtyMB writebackMB iowaitPct stealPct blocked" >>"$PROBE_OUT"
end=$((SECONDS + PROBE_SECONDS))
while [ "$SECONDS" -lt "$end" ]; do
  sleep "$PROBE_INTERVAL"
  read -r _ u n s i w q sq st _ < /proc/stat
  total=$((u + n + s + i + w + q + sq + st)); dt=$((total - prev_total)); [ "$dt" -gt 0 ] || dt=1
  dirty=$(awk '/^Dirty:/{print int($2/1024)}' /proc/meminfo)
  wb=$(awk '/^Writeback:/{print int($2/1024)}' /proc/meminfo)
  blocked=$(awk '/^procs_blocked/{print $2}' /proc/stat)
  echo "$(date +%T) $dirty $wb $(((w - prev_wait) * 100 / dt)) $(((st - prev_steal) * 100 / dt)) $blocked" >>"$PROBE_OUT"
  prev_total=$total; prev_wait=$w; prev_steal=$st
done
