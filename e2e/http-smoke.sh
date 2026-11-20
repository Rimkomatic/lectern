#!/usr/bin/env bash
# Checks a running Lectern over HTTP: it is healthy, the page can be added to a
# home screen and tells search engines to stay away, the administrator can log
# in, and the library was scanned (the named course is listed).
#
#   e2e/http-smoke.sh http://127.0.0.1:8080 admin 'password' 'Sample course'
#
# e2e/compose-smoke.sh uses it against the Docker setup.
set -euo pipefail

base="${1:?usage: http-smoke.sh BASE_URL USER PASSWORD COURSE_TITLE}"
user="${2:?}"
pass="${3:?}"
course="${4:?}"
jar="$(mktemp)"
trap 'rm -f "$jar"' EXIT

echo "== waiting for $base/healthz"
for _ in $(seq 1 60); do
  curl -fsS "$base/healthz" >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS "$base/healthz" >/dev/null || { echo "FAILED: the server did not become healthy"; exit 1; }

echo "== the home-screen manifest, its icon, and the keep-out-of-search-engines signals"
manifest_type="$(curl -fsS -o /dev/null -w '%{content_type}' "$base/manifest.webmanifest")"
case "$manifest_type" in
  application/manifest+json*) ;;
  *) echo "FAILED: /manifest.webmanifest was served as \"$manifest_type\""; exit 1 ;;
esac
curl -fsS "$base/manifest.webmanifest" \
  | python3 -c 'import json, sys; m = json.load(sys.stdin); assert m["name"] and m["start_url"] and m["icons"]' \
  || { echo "FAILED: /manifest.webmanifest is not a usable manifest"; exit 1; }
icon_type="$(curl -fsS -o /dev/null -w '%{content_type}' "$base/icons/icon-192.png")"
[ "$icon_type" = "image/png" ] || { echo "FAILED: the icon was served as \"$icon_type\""; exit 1; }
curl -fsS "$base/robots.txt" | grep -qi '^disallow: /' || { echo "FAILED: /robots.txt does not disallow everything"; exit 1; }
curl -fsS -D - -o /dev/null "$base/healthz" | grep -qi '^x-robots-tag: noindex' \
  || { echo "FAILED: responses carry no X-Robots-Tag: noindex header"; exit 1; }

echo "== logging in as $user"
body="$(python3 -c 'import json, sys; print(json.dumps({"username": sys.argv[1], "password": sys.argv[2]}))' "$user" "$pass")"
curl -fsS -c "$jar" -H 'Content-Type: application/json' -d "$body" "$base/api/v1/auth/login" >/dev/null \
  || { echo "FAILED: login"; exit 1; }

echo "== waiting for the course \"$course\" to be scanned"
titles=""
for _ in $(seq 1 30); do
  titles="$(curl -fsS -b "$jar" "$base/api/v1/courses" \
    | python3 -c 'import json, sys; print("\n".join(c["title"] for c in json.load(sys.stdin)))')"
  if grep -qxF -- "$course" <<<"$titles"; then
    echo "== ok: logged in, library scanned, course listed"
    exit 0
  fi
  sleep 2
done
echo "FAILED: the course was never listed. Courses seen: ${titles:-none}"
exit 1
