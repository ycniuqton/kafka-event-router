---
name: devlens
description: Show the Devlens dashboard URL and status for the current project, auto-starting if not running
---

Show the Devlens dashboard URLs for the current project by inspecting running processes (no AI reasoning needed — just print whatever the script returns):

```!
DIR="${CLAUDE_PROJECT_DIR:-$PWD}"
PROC=$(ps -eo pid,args 2>/dev/null | grep -E "node.*(devlens|dist/index\.js).*start.*--dir[= ]?${DIR}([ /]|$)" | grep -v grep | head -1)

if [ -z "$PROC" ]; then
  PORT=$(grep -oE '"port"[[:space:]]*:[[:space:]]*[0-9]+' "${DIR}/.devlens/runtime.json" 2>/dev/null | grep -oE "[0-9]+" | head -1)
  BIN=""
  command -v devlens &>/dev/null && BIN="devlens"
  [ -z "$BIN" ] && [ -f "${DIR}/dist/index.js" ] && BIN="node ${DIR}/dist/index.js"
  [ -z "$BIN" ] && [ -f "${DIR}/node_modules/.bin/devlens" ] && BIN="${DIR}/node_modules/.bin/devlens"
  if [ -n "$BIN" ] && [ -n "$PORT" ]; then
    echo "DEVLENS_NOT_RUNNING — starting..."
    nohup $BIN start --dir "$DIR" --port $PORT --no-open > /tmp/devlens-$PORT.log 2>&1 &
    for i in 1 2 3 4 5; do
      sleep 1
      PROC=$(ps -eo pid,args 2>/dev/null | grep -E "node.*(devlens|dist/index\.js).*start.*--dir[= ]?${DIR}([ /]|$)" | grep -v grep | head -1)
      [ -n "$PROC" ] && break
    done
  else
    echo "DEVLENS_NOT_RUNNING for ${DIR}"
    exit 0
  fi
fi

PID=$(echo "$PROC" | awk '{print $1}')
PORT=$(echo "$PROC" | grep -oE -- "--port[= ]?[0-9]+" | grep -oE "[0-9]+" | head -1)
[ -z "$PORT" ] && PORT=$(grep -oE '"port"[[:space:]]*:[[:space:]]*[0-9]+' "${DIR}/.devlens/runtime.json" 2>/dev/null | grep -oE "[0-9]+" | head -1)

if [ -z "$PORT" ]; then
  echo "DEVLENS_RUNNING but could not determine port (PID $PID)"
  exit 0
fi

echo "DEVLENS_RUNNING"
echo "  PID:     $PID"
echo "  Project: ${DIR}"
echo "  Local:   http://localhost:${PORT}"
for ip in $(hostname -I 2>/dev/null || true); do
  case "$ip" in
    127.*|::1|fe80*|*:*) continue ;;
    *) echo "  Network: http://${ip}:${PORT}" ;;
  esac
done
```

Print the script output verbatim. If it shows `DEVLENS_NOT_RUNNING — starting...` followed by a running status, tell the user it was just started. If it ends with `DEVLENS_NOT_RUNNING for ...` (no binary found), tell the user Devlens could not be started and suggest running `devlens init` first.
