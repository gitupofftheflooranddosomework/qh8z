#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  docker compose logs --no-color > /tmp/qh8z-compose.log 2>&1 || true
  docker compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker compose up -d --build db shlink app

ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/healthz >/tmp/qh8z-health.json 2>/dev/null; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "$ready" != "1" ]]; then
  echo "QH8Z did not become healthy"
  docker compose logs --no-color
  exit 1
fi

curl -fsS -c /tmp/qh8z-cookies.txt \
  -H 'content-type: application/json' \
  -d '{"name":"CI Admin","email":"admin@example.com","password":"correct-horse-battery"}' \
  http://localhost:3000/api/auth/register >/tmp/qh8z-user.json

create_json=$(curl -fsS -b /tmp/qh8z-cookies.txt \
  -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/one","customSlug":"ci-link","title":"CI link"}' \
  http://localhost:3000/api/links)
printf '%s' "$create_json" >/tmp/qh8z-link.json
link_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["link"]["id"])' </tmp/qh8z-link.json)

curl -sS -D /tmp/qh8z-redirect-1.headers -o /dev/null http://localhost:8080/ci-link
grep -Eiq '^location: https://example\.com/one\r?$' /tmp/qh8z-redirect-1.headers

curl -fsS -b /tmp/qh8z-cookies.txt -X PATCH \
  -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/two","title":"Updated CI link"}' \
  "http://localhost:3000/api/links/${link_id}" >/tmp/qh8z-edited.json

curl -sS -D /tmp/qh8z-redirect-2.headers -o /dev/null http://localhost:8080/ci-link
grep -Eiq '^location: https://example\.com/two\r?$' /tmp/qh8z-redirect-2.headers

curl -fsS -b /tmp/qh8z-cookies.txt "http://localhost:3000/api/links/${link_id}/stats" >/tmp/qh8z-stats.json
curl -fsS -H 'content-type: application/json' \
  -d '{"shortCode":"ci-link","email":"reporter@example.com","reason":"Integration test report"}' \
  http://localhost:3000/api/report >/tmp/qh8z-report.json
curl -fsS -b /tmp/qh8z-cookies.txt http://localhost:3000/api/admin/reports >/tmp/qh8z-reports.json
grep -q 'Integration test report' /tmp/qh8z-reports.json

status=$(curl -sS -o /dev/null -w '%{http_code}' -b /tmp/qh8z-cookies.txt -X DELETE "http://localhost:3000/api/links/${link_id}")
[[ "$status" == "204" ]]

curl -sS -D /tmp/qh8z-disabled.headers -o /dev/null http://localhost:8080/ci-link || true
if grep -Eiq '^location: https://example\.com/two\r?$' /tmp/qh8z-disabled.headers; then
  echo 'Disabled link still redirects to its former destination'
  exit 1
fi

echo 'QH8Z integration smoke test passed.'
