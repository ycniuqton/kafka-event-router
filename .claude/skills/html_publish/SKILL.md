---
name: html_publish
description: Serve the .devlens/html folder over HTTP on a random port so HTML reports can be read in a browser
---

Serve the project's HTML reports folder and give the user the link. Run the script and print whatever it returns (no AI reasoning needed):

```!
DIR="${CLAUDE_PROJECT_DIR:-$PWD}"
BIN=""
command -v npx &>/dev/null && BIN="npx --yes @ycniuqton/devlens"
[ -z "$BIN" ] && command -v devlens &>/dev/null && BIN="devlens"
[ -z "$BIN" ] && [ -f "${DIR}/dist/index.js" ] && BIN="node ${DIR}/dist/index.js"
[ -z "$BIN" ] && [ -f "${DIR}/node_modules/.bin/devlens" ] && BIN="${DIR}/node_modules/.bin/devlens"

if [ -z "$BIN" ]; then
  echo "DEVLENS_BIN_NOT_FOUND — run 'devlens init' first"
  exit 0
fi

LOG="/tmp/devlens-html-serve.log"
nohup $BIN serve-html --dir "$DIR" > "$LOG" 2>&1 &
for i in 1 2 3 4 5; do
  sleep 1
  grep -qE 'http://' "$LOG" 2>/dev/null && break
done
cat "$LOG"
```

Print the URL(s) from the script output and tell the user to open the link in a browser to read the HTML reports. The server keeps running in the background until they stop it.
