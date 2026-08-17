#!/usr/bin/env bash
set -euo pipefail
BASE="${1:-https://qh8z.com}"
BASE="${BASE%/}"

curl -fsS "$BASE/healthz" | grep -q '"ok":true'
curl -fsS "$BASE/readyz" | grep -q '"ready":true\|"ok":true'
curl -fsS "$BASE/" | grep -q 'QH8Z'
curl -fsS "$BASE/security" | grep -q 'Security'
curl -fsS "$BASE/.well-known/security.txt" | grep -q 'mailto:security@qh8z.com'
headers=$(mktemp)
trap 'rm -f "$headers"' EXIT
curl -fsS -D "$headers" -o /dev/null "$BASE/"
grep -qi '^strict-transport-security:' "$headers"
grep -qi '^x-content-type-options: nosniff' "$headers"
code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/links")
[[ "$code" == "401" ]]

echo "QH8Z post-deploy public checks passed for $BASE"
