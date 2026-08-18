#!/usr/bin/env bash
set -euo pipefail

ORIGIN=http://localhost:3000
USER_JAR=/tmp/qh8z-product-user-cookies.txt

cleanup() {
  docker compose --profile production logs --no-color > /tmp/qh8z-product-compose.log 2>&1 || true
  docker compose --profile production down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

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

# Malformed pagination is normalized instead of surfacing a PostgreSQL OFFSET error.
curl -fsS -b "$USER_JAR" 'http://localhost:3000/api/links?limit=Infinity&offset=1.5' >/tmp/qh8z-product-pagination.json
python3 - <<'PY'
import json
with open('/tmp/qh8z-product-pagination.json') as f: d=json.load(f)
assert d['limit']==25
assert d['offset']==0
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
# Details remains usable while the redirect is intentionally absent from Shlink.
curl -fsS -b "$USER_JAR" "http://localhost:3000/api/links/${advanced_id}/stats" >/tmp/qh8z-product-disabled-stats.json
curl -fsS -b "$USER_JAR" "http://localhost:3000/api/links/${advanced_id}/visits?itemsPerPage=50" >/tmp/qh8z-product-disabled-visits.json
python3 - <<'PY'
import json
with open('/tmp/qh8z-product-disabled-stats.json') as f: stats=json.load(f)
with open('/tmp/qh8z-product-disabled-visits.json') as f: visits=json.load(f)
assert stats['unavailable'] is True
assert stats['visits']['total']==0
assert visits['visits']['data']==[]
assert visits['visits']['pagination']['totalItems']==0
PY
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X POST "http://localhost:3000/api/links/${advanced_id}/restore" >/tmp/qh8z-product-restored.json
redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced)
[[ "$redirect" == "https://example.com/advanced" ]]

# Repeat the cycle immediately. Successful restore must finalize its create
# intent transactionally and never require a janitor/background delay.
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X DELETE "http://localhost:3000/api/links/${advanced_id}" >/dev/null
curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X POST "http://localhost:3000/api/links/${advanced_id}/restore" >/dev/null
redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced)
[[ "$redirect" == "https://example.com/advanced" ]]

# QH8Z owns the entire redirect contract, not only the destination. If Shlink
# drifts on title/tags/expiry/max-visits, reconciliation restores those controls.
curl -fsS -X PATCH http://localhost:8080/rest/v3/short-urls/product-advanced \
  -H "X-Api-Key: ${SHLINK_API_KEY}" -H 'content-type: application/json' \
  -d '{"title":"Tampered","tags":["tampered"],"validUntil":null,"maxVisits":null}' >/tmp/qh8z-product-tampered.json
docker compose exec -T app node --input-type=module -e "import { reconcileDueLinks } from './src/consistency.mjs'; import { pool } from './src/db.mjs'; const r=await reconcileDueLinks({confirmAfterMs:0,batch:100}); await pool.end(); if(r.repaired<1) process.exit(1);"
curl -fsS http://localhost:8080/rest/v3/short-urls/product-advanced -H "X-Api-Key: ${SHLINK_API_KEY}" >/tmp/qh8z-product-reconciled.json
python3 - "$expires_at" <<'PY'
import json, sys
from datetime import datetime
with open('/tmp/qh8z-product-reconciled.json') as f: link=json.load(f)
expected=datetime.fromisoformat(sys.argv[1].replace('Z','+00:00'))
actual=datetime.fromisoformat(link['validUntil'].replace('Z','+00:00'))
assert link['longUrl']=='https://example.com/advanced'
assert link['title']=='Product advanced'
assert set(link['tags'])=={'launch','pro'}
assert link['maxVisits']==25
assert actual==expected
PY

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

# Developer API token lifecycle: create -> bearer read/write -> downgrade denied
# -> Pro restored -> token works again -> revoke -> denied.
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

# Billing downgrade keeps user data and tokens stored but removes paid API entitlement.
docker compose exec -T db psql -U postgres -d qh8z -v ON_ERROR_STOP=1 \
  -c "UPDATE users SET plan='free' WHERE email='product@example.com';" >/dev/null
downgraded_status=$(curl -sS -o /tmp/qh8z-product-downgraded-api.json -w '%{http_code}' -H "Authorization: Bearer $api_token" http://localhost:3000/api/v1/links)
[[ "$downgraded_status" == "402" ]]
grep -q 'feature_requires_pro' /tmp/qh8z-product-downgraded-api.json
# Existing advanced link and its redirect remain intact through downgrade.
redirect=$(curl -ksS -o /dev/null -w '%{redirect_url}' https://localhost/product-advanced)
[[ "$redirect" == "https://example.com/advanced" ]]
curl -fsS -b "$USER_JAR" 'http://localhost:3000/api/links?q=product-advanced' | grep -q 'product-advanced'

# Returning to Pro re-enables the same still-stored token.
docker compose exec -T db psql -U postgres -d qh8z -v ON_ERROR_STOP=1 \
  -c "UPDATE users SET plan='pro' WHERE email='product@example.com';" >/dev/null
curl -fsS -H "Authorization: Bearer $api_token" 'http://localhost:3000/api/v1/links?q=product-advanced' | grep -q 'product-advanced'

# Unknown scopes are explicit client errors rather than silently changing intent.
invalid_scope_status=$(curl -sS -o /tmp/qh8z-product-invalid-scope.json -w '%{http_code}' -b "$USER_JAR" -H "Origin: $ORIGIN" -H 'content-type: application/json' \
  -d '{"name":"bad scope","scopes":["links:admin"]}' http://localhost:3000/api/account/api-tokens)
[[ "$invalid_scope_status" == "400" ]]
grep -q 'invalid_api_scope' /tmp/qh8z-product-invalid-scope.json

curl -fsS -b "$USER_JAR" -H "Origin: $ORIGIN" -X DELETE "http://localhost:3000/api/account/api-tokens/${token_id}" >/dev/null
revoked_status=$(curl -sS -o /tmp/qh8z-product-revoked.json -w '%{http_code}' -H "Authorization: Bearer $api_token" http://localhost:3000/api/v1/links)
[[ "$revoked_status" == "401" ]]
grep -q 'invalid_api_token' /tmp/qh8z-product-revoked.json

echo 'QH8Z product acceptance suite passed.'
