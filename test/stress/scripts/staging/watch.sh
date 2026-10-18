#!/bin/sh
# Staging guardrail sampler: every 10 s logs host memory/disk, Kafka volume, PG conns, container CPU/mem/health.
# Any container not healthy, memory > MEM_LIMIT % or disk > 90 % -> SIGINT to stressrun (graceful cleanup) + ABORT file.
D=/opt/event-router/stress; LOG=$D/results/watch.csv; MEM_LIMIT=${MEM_LIMIT:-85}
KVOL=$(docker volume inspect event-router-staging_kafkadata --format '{{.Mountpoint}}')
[ -f $LOG ] || echo "ts,mem_pct,swap_used_kb,disk_pct,kafka_mb,pg_conns,unhealthy,worker_cpu,worker_mem,kafka_cpu,kafka_mem,api_cpu,api_mem,pg_cpu,load1" > $LOG
while [ ! -f $D/STOP_WATCH ]; do
  TS=$(date +%s)
  MEM=$(awk '/MemTotal/{t=$2}/MemAvailable/{a=$2}END{printf "%.1f",(t-a)*100/t}' /proc/meminfo)
  SWAP=$(awk '/SwapTotal/{t=$2}/SwapFree/{f=$2}END{print t-f}' /proc/meminfo)
  DISK=$(df --output=pcent / | tail -1 | tr -dc 0-9)
  KMB=$(du -sm $KVOL | cut -f1)
  PG=$(docker exec event-router-staging-postgres-1 sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "select count(*) from pg_stat_activity"' 2>/dev/null)
  BAD=$(docker ps -a --filter label=com.docker.compose.project=event-router-staging --format '{{.Names}}={{.State}}/{{.Status}}' | grep -v 'running/.*(healthy)' | grep -v 'migrate' | tr '\n' ';')
  ST=$(docker stats --no-stream --format '{{.Name}} {{.CPUPerc}} {{.MemUsage}}' | awk '{gsub("%","",$2); n[$1]=$2","$3}END{printf "%s,%s,%s,%s", n["event-router-staging-worker-1"], n["event-router-staging-kafka-1"], n["event-router-staging-api-1"], n["event-router-staging-postgres-1"]}')
  L1=$(cut -d' ' -f1 /proc/loadavg)
  echo "$TS,$MEM,$SWAP,$DISK,$KMB,$PG,$BAD,$ST,$L1" >> $LOG
  REASON=""
  [ -n "$BAD" ] && REASON="unhealthy:$BAD"
  [ "${MEM%.*}" -ge "$MEM_LIMIT" ] && REASON="$REASON mem:$MEM%"
  [ "$SWAP" -gt 0 ] && REASON="$REASON swap:${SWAP}kB"
  [ "$DISK" -ge 90 ] && REASON="$REASON disk:$DISK%"
  if [ -n "$REASON" ]; then
    echo "$TS $REASON" >> $D/results/ABORT
    [ -f $D/results/ABORT_SENT ] || { pkill -INT -x stressrun; touch $D/results/ABORT_SENT; }
  fi
  sleep 10
done
