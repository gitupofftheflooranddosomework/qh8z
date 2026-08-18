#!/usr/bin/env bash
set -euo pipefail

ORIGIN=http://localhost:3000
USER_JAR=/tmp/qh8z-product-user-cookies.txt

cleanup() {
  docker compose --profile production logs --no-color > /tmp/qh8z-product-compose.log 2>&1 || true
  docker compose --profile production down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

json_field() {
  python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"
}

docker compose --profile production up -d --build --remove-orphans

ready=0
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:3000/readyz >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[[ "$ready" == "1" ]]

# Product assets and dashboard shell are actually served.
curl -fsS http://localhost:3000/app | grep -q 'Link library'
curl -fsS http://localhost:3000/assets/product.css | grep -q 'qh-link-card'
curl -fsS http://localhost:3000/assets/app.js | grep -q 'apiTokenForm'

register=$(curl -fsS -c "$USER_JAR" -H 'content-type: application/json' \
  -d '{"name":"Product User","email":"product@example.com","password":"correct-horse-battery","acceptTerms":true}' \
  http://localhost:3000/api/auth/register)
printf '%s' "$register" >/tmp/qh8z-product-register.json
verify_token=$(python3 -c 'import json; d=json.load(open("/tmp/qh8z-product-register.json")); print(d["debugVerificationToken"])')
curl -fsS -H 'content-type: application/json' -d "{\"token\":\"$verify_token\"}" http://localhost:3000/api/auth/verify-email >/dev/null

# Free is useful, but advanced automation controls are genuinely gated.
free_status=$(curl -sS -o /tmp/qh8z-free-advanced.json -w '%{http_code}' -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/free-advanced","customSlug":"free-advanced","maxVisits":10}' \
  http://localhost:3000/api/links)
[[ "$free_status" == "402" ]]
grep -q 'feature_requires_pro' /tmp/qh8z-free-advanced.json

# CI promotes the test account directly so the product suite can exercise Pro
# without coupling acceptance tests to Stripe's external network.
docker compose exec -T db psql -U postgres -d qh8z -v ON_ERROR_STOP=1 \
  -c "UPDATE users SET plan='pro' WHERE email='product@example.com';" >/dev/null

expires_at=$(date -u -d '+30 days' '+%Y-%m-%dT%H:%M:%S.000Z')
advanced=$(curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' \
  -d "{\"longUrl\":\"https://example.com/advanced\",\"customSlug\":\"product-advanced\",\"title\":\"Product advanced\",\"tags\":[\"launch\",\"pro\"],\"notes\":\"Private product acceptance note\",\"expiresAt\":\"$expires_at\",\"maxVisits\":25}" \
  http://localhost:3000/api/links)
printf '%s' "$advanced" >/tmp/qh8z-product-advanced.json
advanced_id=$(python3 -c 'import json; d=json.load(open("/tmp/qh8z-product-advanced.json")); print(d["link"]["id"])')
python3 - <<'PY'
import json
with open('/tmp/qh8z-product-advanced.json') as f: link=json.load(f)['link']
assert link['short_code']=='product-advanced'
assert set(link['tags'])=={'launch','pro'}
assert link['notes']=='Private product acceptance note'
assert link['max_visits']==25
assert link['expires_at']
PY
redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced)
[[ "$redirect" == "https://example.com/advanced" ]]

# Search, tag filters, state filters, and pagination metadata.
curl -fsS -b "$USER_JAR" 'http://localhost:3000/api/links?q=Product%20advanced&tag=launch&status=active&limit=1&offset=0' >/tmp/qh8z-product-list.json
python3 - <<'PY'
import json
with open('/tmp/qh8z-product-list.json') as f: d=json.load(f)
assert d['total']==1
assert d['limit']==1
assert d['offset']==0
assert d['hasMore'] is False
assert d['links'][0]['short_code']=='product-advanced'
PY

# Archive is organizational only: redirect stays live. Unarchive returns it to active view.
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X POST "http://localhost:3000/api/links/${advanced_id}/archive" >/tmp/qh8z-product-archive.json
curl -fsS -b "$USER_JAR" 'http://localhost:3000/api/links?status=archived&q=product-advanced' | grep -q 'product-advanced'
redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced)
[[ "$redirect" == "https://example.com/advanced" ]]
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X POST "http://localhost:3000/api/links/${advanced_id}/unarchive" >/dev/null

# Disable/restore lifecycle preserves the short code and advanced controls.
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X DELETE "http://localhost:3000/api/links/${advanced_id}" >/dev/null
curl -fsS -b "$USER_JAR" 'http://localhost:3000/api/links?status=disabled&q=product-advanced' | grep -q 'product-advanced'
disabled_redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced || true)
[[ "$disabled_redirect" != "https://example.com/advanced" ]]
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X POST "http://localhost:3000/api/links/${advanced_id}/restore" >/tmp/qh8z-product-restored.json
redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced)
[[ "$redirect" == "https://example.com/advanced" ]]

# Bulk workflow supports partial results and creates multiple usable links.
bulk=$(curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' \
  -d '{"links":[{"longUrl":"https://example.com/bulk-one","customSlug":"product-bulk-one","title":"Bulk one"},{"longUrl":"https://example.com/bulk-two","customSlug":"product-bulk-two","title":"Bulk two","tags":["bulk"]}]}' \
  http://localhost:3000/api/links/bulk)
printf '%s' "$bulk" >/tmp/qh8z-product-bulk.json
python3 - <<'PY'
import json
with open('/tmp/qh8z-product-bulk.json') as f: d=json.load(f)
assert d['created']==2
assert d['failed']==0
assert all(x['ok'] for x in d['results'])
PY
for slug in product-bulk-one product-bulk-two; do
  target=$(curl -ksS -o /dev/null -w '%{redirect_url}' "https://localhost/$slug")
  [[ "$target" == https://example.com/* ]]
done

# CSV export is a real downloadable inventory, not just a dashboard feature.
curl -fsS -b "$USER_JAR" http://localhost:3000/api/links/export.csv >/tmp/qh8z-product-links.csv
grep -q 'product-advanced' /tmp/qh8z-product-links.csv
grep -q 'product-bulk-one' /tmp/qh8z-product-links.csv

# Developer API token lifecycle: create -> bearer read/write -> revoke -> denied.
token_json=$(curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' \
  -d '{"name":"CI product token","scopes":["links:read","links:write"],"expiresInDays":30}' \
  http://localhost:3000/api/account/api-tokens)
printf '%s' "$token_json" >/tmp/qh8z-product-token.json
api_token=$(python3 -c 'import json; print(json.load(open("/tmp/qh8z-product-token.json"))["token"])')
token_id=$(python3 -c 'import json; print(json.load(open("/tmp/qh8z-product-token.json"))["record"]["id"])')
[[ "$api_token" == qh8z_live_* ]]

curl -fsS -H "Authorization: Bearer $api_token" 'http://localhost:3000/api/v1/links?q=product-advanced' >/tmp/qh8z-product-api-list.json
grep -q 'product-advanced' /tmp/qh8z-product-api-list.json
api_created=$(curl -fsS -H "Authorization: Bearer $api_token" -H 'content-type: application/json' \
  -d '{"longUrl":"https://example.com/api-created","customSlug":"product-api-created","title":"Created through API","tags":["api"]}' \
  http://localhost:3000/api/v1/links)
printf '%s' "$api_created" >/tmp/qh8z-product-api-created.json
grep -q 'product-api-created' /tmp/qh8z-product-api-created.json
api_redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-api-created)
[[ "$api_redirect" == "https://example.com/api-created" ]]

curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X DELETE "http://localhost:3000/api/account/api-tokens/${token_id}" >/dev/null
revoked_status=$(curl -sS -o /tmp/qh8z-product-revoked.json -w '%{http_code}' -H "Authorization: Bearer $api_token" http://localhost:3000/api/v1/links)
[[ "$revoked_status" == "401" ]]
grep -q 'invalid_api_token' /tmp/qh8z-product-revoked.json

echo 'QH8Z product acceptance suite passed.'
