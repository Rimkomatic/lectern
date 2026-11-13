#!/usr/bin/env bash
# Browser smoke test. Builds the web app and the server, starts the server on
# the TEST database, generates a small sample library and drives the UI with
# headless Chrome (setup, login, playback and resume, PDF, notes, tables,
# images, links, downloads, settings, admin, a second user, a course dropped
# into the library with HTML pages and subtitles).
#
# WARNING: this EMPTIES the database named by LECTERN_TEST_DB_NAME (default
# "lectern_test", the one `go test` uses). Do not point it at real data.
#
# Needs: Go, Node 22+, ffmpeg, Python 3 with `pip install playwright pillow
# reportlab`, and a browser: set E2E_CHROME=/path/to/chrome, or run
# `playwright install chromium`. MySQL must be running with the test database
# (`make db db-test`).
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${E2E_PORT:-18080}"
WORK="$(mktemp -d)"
SERVER_PID=""
cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# the server reads the same database as `go test`
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

echo "== sample library"
python3 e2e/make_library.py "$WORK/library"

# A nightly scan 12 hours from now: the schedule is on (and shown in the admin
# page) but cannot fire while the test runs.
export E2E_SCAN_AT="$(python3 -c 'import datetime as d; print((d.datetime.now() + d.timedelta(hours=12)).strftime("%H:%M"))')"
export E2E_LIBRARY="$WORK/library"

echo "== starting the server on :$PORT"
(
  cd "$WORK" # so no .env from the project is picked up
  LECTERN_LISTEN="127.0.0.1:$PORT" LECTERN_LIBRARIES="$WORK/library" \
    LECTERN_COOKIE_SECURE=false LECTERN_SCAN_AT="$E2E_SCAN_AT" \
    exec ./lectern >server.log 2>&1
) &
SERVER_PID=$!
for _ in $(seq 1 60); do
  curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null && break
  sleep 0.5
done
curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null || { echo "server did not start"; cat "$WORK/server.log"; exit 1; }

echo "== browser run"
if E2E_BASE="http://127.0.0.1:$PORT" python3 e2e/smoke.py; then
  echo "== smoke test passed"
else
  echo "== smoke test FAILED; server log tail:"
  tail -n 20 "$WORK/server.log"
  exit 1
fi
