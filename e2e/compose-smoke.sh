#!/usr/bin/env bash
# Builds the image from this source tree, starts the real compose setup
# (docker-compose.yml + docker-compose.build.yml) with a throw-away .env, one
# sample course and a first administrator, and checks it over HTTP. Then it
# resets the administrator's password with the command the README gives for a
# forgotten password, once with a random password and once with a chosen one.
#
# CI runs this (.github/workflows/ci.yml). On your own machine it needs port
# 18081 free (SMOKE_PORT) and no other Lectern containers, because the
# containers have fixed names. It cleans up after itself, volume included.
set -euo pipefail
cd "$(dirname "$0")/.."

PORT="${SMOKE_PORT:-18081}"
ADMIN_PASSWORD="smoke-admin-password"
WORK="$(mktemp -d)"
ENVFILE="$WORK/env"

compose() { docker compose --env-file "$ENVFILE" -f docker-compose.yml -f docker-compose.build.yml "$@"; }
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "== FAILED; container state and log tail"
    compose ps -a || true
    compose logs --no-color --tail 80 || true
  fi
  compose down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$WORK"
  exit "$status"
}
trap cleanup EXIT

mkdir -p "$WORK/library/Sample course/01 - Basics"
printf '# Hello\n\nA note.\n' > "$WORK/library/Sample course/01 - Basics/01 - Note.md"

cat > "$ENVFILE" <<EOT
MYSQL_ROOT_PASSWORD=smoke-root-password
MYSQL_PASSWORD=smoke-db-password
LECTERN_LIBRARY=$WORK/library
LECTERN_PORT=$PORT
PUID=$(id -u)
PGID=$(id -g)
LECTERN_ADMIN_USER=admin
LECTERN_ADMIN_PASSWORD=$ADMIN_PASSWORD
EOT

echo "== docker compose config"
compose config --quiet

echo "== building and starting (waits until the app is healthy)"
compose up -d --build --wait --wait-timeout 300

e2e/http-smoke.sh "http://127.0.0.1:$PORT" admin "$ADMIN_PASSWORD" "Sample course"

echo "== resetting the password with the README's command (random password)"
out="$(compose exec -T app lectern -reset-password admin)"
echo "$out"
random_password="$(sed -n 's/^  New password: //p' <<<"$out")"
[ -n "$random_password" ] || { echo "FAILED: the command printed no new password"; exit 1; }
e2e/http-smoke.sh "http://127.0.0.1:$PORT" admin "$random_password" "Sample course"
if curl -fsS -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" \
    "http://127.0.0.1:$PORT/api/v1/auth/login" >/dev/null 2>&1; then
  echo "FAILED: the old password still works after the reset"
  exit 1
fi

echo "== resetting the password with the README's command (a chosen password)"
chosen_password="chosen-smoke-password"
printf '%s\n' "$chosen_password" | compose exec -T app lectern -reset-password admin -password-stdin
e2e/http-smoke.sh "http://127.0.0.1:$PORT" admin "$chosen_password" "Sample course"
