#!/usr/bin/env bash
set -euo pipefail
BASE="${1:-https://qh8z.com}"
BASE="${BASE%/}"
CURL=(curl --connect-timeout 5 --max-time 15)

"${CURL[@]}" -fsS "$BASE/healthz" | grep -q '"ok":true'
"${CURL[@]}" -fsS "$BASE/readyz" | grep -q '"ready":true\|"ok":true'
"${CURL[@]}" -fsS "$BASE/" | grep -q 'QH8Z'
"${CURL[@]}" -fsS "$BASE/security" | grep -q 'Security'
"${CURL[@]}" -fsS "$BASE/.well-known/security.txt" | grep -q 'mailto:security@qh8z.com'
headers=$(mktemp)
trap 'rm -f "$headers"' EXIT
"${CURL[@]}" -fsS -D "$headers" -o /dev/null "$BASE/"
grep -qi '^strict-transport-security:' "$headers"
grep -qi '^x-content-type-options: nosniff' "$headers"
code=$("${CURL[@]}" -sS -o /dev/null -w '%{http_code}' "$BASE/api/links")
[[ "$code" == "401" ]]
upstream_code=$("${CURL[@]}" -sS -o /dev/null -w '%{http_code}' "$BASE/rest/health")
[[ "$upstream_code" == "404" ]]
upstream_root_code=$("${CURL[@]}" -sS -o /dev/null -w '%{http_code}' "$BASE/rest")
[[ "$upstream_root_code" == "404" ]]

echo "QH8Z post-deploy public checks passed for $BASE"
