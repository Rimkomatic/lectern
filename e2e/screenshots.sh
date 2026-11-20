#!/usr/bin/env bash
# Takes the screenshots for the README (docs/screenshots/*.png): builds Lectern,
# starts it on the TEST database with a made-up demo library (make_demo_library.py)
# and drives it with headless Chrome.
#
# WARNING: like run.sh this EMPTIES the database named by LECTERN_TEST_DB_NAME
# (default "lectern_test"). Do not point it at real data.
#
# Needs what run.sh needs (Go, Node 22+, ffmpeg, Python 3 with `pip install
# playwright pillow reportlab`, a browser via E2E_CHROME or `playwright install
# chromium`, MySQL with the test database), and takes a few minutes: the demo
# "videos" are encoded first.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${E2E_PORT:-18081}"
WORK="$(mktemp -d)"
SERVER_PID=""
cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

export MYSQL_HOST="${LECTERN_TEST_DB_HOST:-127.0.0.1}"
export MYSQL_PORT="${LECTERN_TEST_DB_PORT:-3306}"
export MYSQL_DATABASE="${LECTERN_TEST_DB_NAME:-lectern_test}"
export MYSQL_USER="${LECTERN_TEST_DB_USER:-lectern_test}"
export MYSQL_PASSWORD="${LECTERN_TEST_DB_PASSWORD:-lectern_test}"

echo "== building"
[ -d web/node_modules ] || (cd web && npm ci)
(cd web && npm run build >/dev/null)
go build -o "$WORK/lectern" ./cmd/lectern

echo "== emptying the test database"
go run ./e2e/resetdb

echo "== demo library (a few minutes)"
python3 e2e/make_demo_library.py "$WORK/library"

echo "== starting the server on :$PORT"
(
  cd "$WORK" # so no .env from the project is picked up
  LECTERN_LISTEN="127.0.0.1:$PORT" LECTERN_LIBRARIES="$WORK/library" \
    LECTERN_COOKIE_SECURE=false LECTERN_SCAN_AT=off \
    exec ./lectern >server.log 2>&1
) &
SERVER_PID=$!
for _ in $(seq 1 60); do
  curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null && break
  sleep 0.5
done
curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null || { echo "server did not start"; cat "$WORK/server.log"; exit 1; }

echo "== screenshots"
E2E_BASE="http://127.0.0.1:$PORT" SHOTS_OUT="docs/screenshots" python3 e2e/screenshots.py
