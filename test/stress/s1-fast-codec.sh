#!/usr/bin/env bash
# S1 fast-codec comparison: runs the S1 engine + runner matrices once with ROUTER_ENGINE_CODEC=tree and once with
# fast (same pinned core, same settings) and merges both into test/stress/results/s1-fast-codec.json.
# Env: as s1-bench.sh (BENCHTIME, COUNT, CPU, SKIP_PROFILES); RESULTS (default test/stress/results).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="${RESULTS:-$ROOT/test/stress/results}"
WORK="$RESULTS/s1-fast-codec"

for codec in tree fast; do
  ENGINE_CODEC="$codec" SKIP_SANDBOX=1 RESULTS="$WORK/$codec" OUT_JSON="$WORK/$codec/s1-engine.json" \
    "$ROOT/test/stress/s1-bench.sh"
done

python3 -I "$ROOT/test/stress/s1-compare.py" --tree "$WORK/tree/s1-engine.json" --fast "$WORK/fast/s1-engine.json" \
  --out "$RESULTS/s1-fast-codec.json"
echo ">> wrote $RESULTS/s1-fast-codec.json" >&2
