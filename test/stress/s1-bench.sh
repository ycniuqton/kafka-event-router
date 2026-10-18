#!/usr/bin/env bash
# S1 engine-only benchmark matrix (no Kafka): step type x payload size on one pinned core.
# Writes test/stress/results/s1-engine.json, raw go test output and CPU profiles with pprof top-15.
# Env: BENCHTIME (default 3s), COUNT (3), PROFILE_BENCHTIME (10s), CPU (0), RESULTS (output dir), SKIP_PROFILES.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PATH="$HOME/.local/go/bin:$PATH"
export GOMAXPROCS=1

BENCHTIME="${BENCHTIME:-3s}"
COUNT="${COUNT:-3}"
PROFILE_BENCHTIME="${PROFILE_BENCHTIME:-10s}"
CPU="${CPU:-0}"
RESULTS="${RESULTS:-$ROOT/test/stress/results}"
RAW="$RESULTS/s1-raw"
PROFILES="$RESULTS/s1-profiles"
OUT_JSON="$RESULTS/s1-engine.json"
PIN=(taskset -c "$CPU")

mkdir -p "$RAW" "$PROFILES"
cd "$ROOT"

run_matrix() {
  local name="$1" pkg="$2" bench="$3"
  echo ">> $name: $pkg -bench '$bench' -benchtime $BENCHTIME -count $COUNT (cpu $CPU)" >&2
  "${PIN[@]}" go test -run '^$' -bench "$bench" -benchmem -benchtime "$BENCHTIME" -count "$COUNT" -timeout 0 "$pkg" \
    | tee "$RAW/$name.txt" >&2
}

profile_cell() {
  local name="$1" pkg="$2" bench="$3"
  local bin="$PROFILES/$name.test" prof="$PROFILES/$name.cpu.pprof"
  echo ">> profile $name: -bench '$bench' -benchtime $PROFILE_BENCHTIME" >&2
  "${PIN[@]}" go test -run '^$' -bench "$bench" -benchmem -benchtime "$PROFILE_BENCHTIME" -count 1 -timeout 0 \
    -cpuprofile "$prof" -o "$bin" "$pkg" >"$PROFILES/$name.bench.txt"
  go tool pprof -top -nodecount=15 "$bin" "$prof" >"$PROFILES/$name.top15.txt" 2>/dev/null
  go tool pprof -top -cum -nodecount=15 "$bin" "$prof" >"$PROFILES/$name.top15-cum.txt" 2>/dev/null
}

run_matrix engine ./pkg/engine/ 'BenchmarkS1Engine'
run_matrix sandbox ./pkg/sandbox/ 'BenchmarkS1SandboxCall'
run_matrix runner ./internal/runtime/runner/ 'BenchmarkS1RecordProcessor'

if [[ -z "${SKIP_PROFILES:-}" ]]; then
  profile_cell recipe-5-1KB ./pkg/engine/ 'BenchmarkS1Engine/shape=recipe-5/payload=1KB$'
  profile_cell js-1KB ./pkg/engine/ 'BenchmarkS1Engine/shape=js/payload=1KB$'
fi

python3 -I "$ROOT/test/stress/s1-report.py" \
  --raw "$RAW" --profiles "$PROFILES" --out "$OUT_JSON" \
  --benchtime "$BENCHTIME" --count "$COUNT" --cpu "$CPU" --go "$(go version)"
echo ">> wrote $OUT_JSON" >&2
