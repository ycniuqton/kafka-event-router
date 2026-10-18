#!/usr/bin/env bash
# One-command entry point for local development (stands in for make, which is not installed).
# Usage: deploy/dev/dev.sh up|down|status|logs [svc] [-f]
set -euo pipefail
DEV_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "${1:-status}" in
  up) "$DEV_DIR/tunnel.sh" start && "$DEV_DIR/run.sh" start ;;
  down) "$DEV_DIR/run.sh" stop; "$DEV_DIR/tunnel.sh" stop ;;
  status) "$DEV_DIR/run.sh" status ;;
  logs) shift; "$DEV_DIR/run.sh" logs "$@" ;;
  *) echo "usage: $0 up|down|status|logs [svc] [-f]" >&2; exit 2 ;;
esac
